package data

import (
	"context"
	"fmt"
	"sync"
	"time"

	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	defaultDeviceConfigKey   = "device-config.yaml"
	deviceCatalogFirstWait   = 10 * time.Second
	deviceCatalogListTimeout = 30 * time.Second
)

// ListFunc records list outcomes because a forbidden or empty list never reaches the handlers.
type deviceCatalog struct {
	ref      devicecatalog.Ref
	log      *log.Helper
	informer cache.SharedIndexInformer
	now      func() time.Time

	mu          sync.Mutex
	listErr     error
	listed      bool
	content     string
	current     *devicecatalog.Snapshot
	subscribers []func(previous, current *devicecatalog.Snapshot)
	firstList   chan struct{}
}

// NewDeviceCatalog waits briefly for the first read so Pods decoded at startup see it.
func NewDeviceCatalog(data *Data, c *conf.Bootstrap, logger log.Logger) devicecatalog.Source {
	return newDeviceCatalog(data.k8sCl, c.GetDeviceConfig(), log.NewHelper(logger), deviceCatalogFirstWait)
}

func newDeviceCatalog(client kubernetes.Interface, config *conf.DeviceConfig, logger *log.Helper, firstWait time.Duration) devicecatalog.Source {
	ref := devicecatalog.Ref{Namespace: config.GetNamespace(), Name: config.GetName(), Key: config.GetKey()}
	if ref.Key == "" {
		ref.Key = defaultDeviceConfigKey
	}
	if !config.GetEnabled() || ref.Namespace == "" || ref.Name == "" {
		return devicecatalog.Static{Current: &devicecatalog.Snapshot{State: devicecatalog.StateDisabled, Ref: ref}}
	}
	r := &deviceCatalog{
		ref:       ref,
		log:       logger,
		now:       time.Now,
		current:   &devicecatalog.Snapshot{State: devicecatalog.StateLoading, Ref: ref},
		firstList: make(chan struct{}),
	}
	selector := fields.OneTermEqualSelector("metadata.name", ref.Name).String()
	listWatch := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.FieldSelector = selector
			ctx, cancel := context.WithTimeout(context.Background(), deviceCatalogListTimeout)
			defer cancel()
			list, err := client.CoreV1().ConfigMaps(ref.Namespace).List(ctx, options)
			r.recordList(err)
			return list, err
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.FieldSelector = selector
			return client.CoreV1().ConfigMaps(ref.Namespace).Watch(context.Background(), options)
		},
	}
	r.informer = cache.NewSharedIndexInformer(listWatch, &corev1.ConfigMap{}, 0, cache.Indexers{})
	refresh := func(interface{}) { r.refresh() }
	if _, err := r.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    refresh,
		UpdateFunc: func(_, obj interface{}) { refresh(obj) },
		DeleteFunc: refresh,
	}); err != nil {
		return devicecatalog.Static{Current: &devicecatalog.Snapshot{State: devicecatalog.StateError, Reason: err.Error(), Ref: ref}}
	}
	go r.informer.Run(make(chan struct{}))
	go func() {
		if cache.WaitForCacheSync(make(chan struct{}), r.informer.HasSynced) {
			r.refresh()
		}
	}()
	select {
	case <-r.firstList:
	case <-time.After(firstWait):
		r.log.Warnf("HAMi device configuration %s/%s was not read within %s", ref.Namespace, ref.Name, firstWait)
		return r
	}
	r.mu.Lock()
	listed := r.listErr == nil
	r.mu.Unlock()
	if listed && cache.WaitForCacheSync(timeoutChannel(firstWait), r.informer.HasSynced) {
		r.refresh()
	}
	return r
}

func timeoutChannel(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	time.AfterFunc(d, func() { close(ch) })
	return ch
}

func (r *deviceCatalog) Snapshot() *devicecatalog.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}

func (r *deviceCatalog) Subscribe(fn func(previous, current *devicecatalog.Snapshot)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers = append(r.subscribers, fn)
}

func (r *deviceCatalog) recordList(err error) {
	r.mu.Lock()
	r.listErr = err
	if err == nil {
		r.listed = true
	}
	select {
	case <-r.firstList:
	default:
		close(r.firstList)
	}
	r.mu.Unlock()
	r.refresh()
}

func (r *deviceCatalog) refresh() {
	r.mu.Lock()
	previous := r.current
	next := r.build(previous)
	if next == previous {
		r.mu.Unlock()
		return
	}
	r.current = next
	subscribers := append([]func(previous, current *devicecatalog.Snapshot){}, r.subscribers...)
	r.mu.Unlock()
	if next.State != previous.State || next.ResourceVersion != previous.ResourceVersion {
		r.log.Infof("HAMi device configuration %s/%s: %s %s", r.ref.Namespace, r.ref.Name, next.State, next.Reason)
	}
	for _, fn := range subscribers {
		fn(previous, next)
	}
}

// build runs with r.mu held and returns previous when nothing changed.
func (r *deviceCatalog) build(previous *devicecatalog.Snapshot) *devicecatalog.Snapshot {
	next := &devicecatalog.Snapshot{Ref: r.ref, ObservedAt: r.now()}
	content := ""
	switch {
	case r.listErr != nil && (apierrors.IsForbidden(r.listErr) || apierrors.IsUnauthorized(r.listErr)):
		next.State, next.Reason = devicecatalog.StateForbidden, r.listErr.Error()
	case r.listErr != nil && previous.Loaded():
		// A transient error keeps the last configuration that was read.
		return previous
	case r.listErr != nil:
		next.State, next.Reason = devicecatalog.StateError, r.listErr.Error()
	case !r.listed || !r.informer.HasSynced():
		next.State = devicecatalog.StateLoading
	default:
		item, exists, err := r.informer.GetStore().GetByKey(r.ref.Namespace + "/" + r.ref.Name)
		configMap, _ := item.(*corev1.ConfigMap)
		switch {
		case err != nil:
			next.State, next.Reason = devicecatalog.StateError, err.Error()
		case !exists || configMap == nil:
			next.State = devicecatalog.StateMissing
		default:
			next.ResourceVersion = configMap.ResourceVersion
			var ok bool
			content, ok = configMap.Data[r.ref.Key]
			if !ok {
				next.State, next.Reason = devicecatalog.StateInvalid, fmt.Sprintf("key %q not found", r.ref.Key)
				break
			}
			parsed, err := devicecatalog.Parse([]byte(content))
			if err != nil {
				next.State, next.Reason = devicecatalog.StateInvalid, err.Error()
				break
			}
			next.State, next.HamiVnpuCore, next.Ascend, next.Issues = devicecatalog.StateLoaded, parsed.HamiVnpuCore, parsed.Ascend, parsed.Issues
		}
	}
	if next.State == previous.State && next.Reason == previous.Reason && next.ResourceVersion == previous.ResourceVersion && content == r.content {
		return previous
	}
	r.content = content
	return next
}
