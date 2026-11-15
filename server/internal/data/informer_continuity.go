package data

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	typedcore "k8s.io/client-go/kubernetes/typed/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

// This observer is on the existing factory's client boundary. It retains only
// object identity/version, not a second object cache or another List/Watch.
type objectVersion struct {
	UID types.UID
	RV  string
}
type continuitySnapshot struct {
	sequence          uint64
	objects           map[string]objectVersion
	ready             bool
	reason, rv        string
	listedAt, watchAt time.Time
}
type informerContinuity struct {
	resource                  string
	mu                        sync.Mutex
	sequence, watchGeneration uint64
	objects, listing          map[string]objectVersion
	listRV, nextPage, rv      string
	ready, listReady          bool
	reason                    string
	listedAt, watchAt         time.Time
}
type sharedContinuity struct{ pods, nodes *informerContinuity }

func newInformerContinuity(resource ...string) *informerContinuity {
	s := &informerContinuity{reason: "initializing", objects: map[string]objectVersion{}}
	if len(resource) > 0 {
		s.resource = resource[0]
	}
	return s
}
func newTrackedInformerFactory(client kubernetes.Interface, resync time.Duration) (informers.SharedInformerFactory, *sharedContinuity) {
	state := &sharedContinuity{pods: newInformerContinuity("pods"), nodes: newInformerContinuity("nodes")}
	return informers.NewSharedInformerFactory(trackedInformerClient{Interface: client, state: state}, resync), state
}
func (s *informerContinuity) invalidate(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked(reason)
}
func (s *informerContinuity) invalidateLocked(reason string) {
	s.sequence++
	s.ready = false
	s.listReady = false
	s.reason = reason
}
func (s *informerContinuity) beginList(options metav1.ListOptions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.invalidateLocked("relisting")
	if options.Continue == "" {
		s.listing = map[string]objectVersion{}
		s.listRV = ""
		s.nextPage = ""
	}
}
func objectKey(obj metav1.Object) string { return obj.GetNamespace() + "/" + obj.GetName() }
func versionOf(obj metav1.Object) (objectVersion, bool) {
	return objectVersion{obj.GetUID(), obj.GetResourceVersion()}, obj.GetName() != "" && obj.GetUID() != "" && obj.GetResourceVersion() != ""
}
func (s *informerContinuity) listResult(options metav1.ListOptions, result runtime.Object, err error, fullScope bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.invalidateLocked(continuityFailure("list", err))
		s.listing = nil
		return
	}
	if !fullScope || options.LabelSelector != "" || options.FieldSelector != "" {
		s.invalidateLocked("filtered_list")
		s.listing = nil
		return
	}
	info, e := meta.ListAccessor(result)
	if e != nil || info.GetResourceVersion() == "" || s.listing == nil || options.Continue != s.nextPage {
		s.invalidateLocked("invalid_list")
		s.listing = nil
		return
	}
	if s.listRV != "" && s.listRV != info.GetResourceVersion() {
		s.invalidateLocked("inconsistent_list_rv")
		s.listing = nil
		return
	}
	s.listRV = info.GetResourceVersion()
	items, e := meta.ExtractList(result)
	if e != nil {
		s.invalidateLocked("invalid_list")
		s.listing = nil
		return
	}
	for _, item := range items {
		obj, e := meta.Accessor(item)
		if e != nil {
			s.invalidateLocked("invalid_list_object")
			s.listing = nil
			return
		}
		v, valid := versionOf(obj)
		key := objectKey(obj)
		_, duplicate := s.listing[key]
		if !valid || duplicate {
			s.invalidateLocked("invalid_list_identity")
			s.listing = nil
			return
		}
		s.listing[key] = v
	}
	next := info.GetContinue()
	if next != "" && next == options.Continue {
		s.invalidateLocked("repeated_list_page")
		s.listing = nil
		return
	}
	s.nextPage = next
	s.sequence++
	if next != "" {
		return
	}
	s.objects = s.listing
	s.listing = nil
	s.rv = s.listRV
	s.listReady = true
	s.listedAt = time.Now().UTC()
	s.reason = "awaiting_watch"
}
func (s *informerContinuity) allowWatch(options metav1.ListOptions, fullScope bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fullScope || options.LabelSelector != "" || options.FieldSelector != "" || (options.SendInitialEvents != nil && *options.SendInitialEvents) {
		s.invalidateLocked("unsupported_watch_scope")
		// Reflector explicitly falls back to LIST on a failed watch-list attempt.
		return apierrors.NewBadRequest("shared continuity requires full LIST then WATCH")
	}
	if !s.listReady || options.ResourceVersion == "" || options.ResourceVersion != s.rv {
		s.invalidateLocked("relist_required")
		// This non-retriable watch error returns from Reflector.watch and starts a
		// real LIST cycle. Returning an empty stream would retry WATCH indefinitely.
		return apierrors.NewResourceExpired("shared continuity requires a completed full LIST")
	}
	return nil
}
func (s *informerContinuity) wrapWatch(ctx context.Context, options metav1.ListOptions, w watch.Interface, err error) (watch.Interface, error) {
	if err != nil || w == nil {
		s.invalidate(continuityFailure("watch", err))
		if err == nil {
			err = fmt.Errorf("watch returned nil stream")
		}
		return nil, err
	}
	s.mu.Lock()
	s.watchGeneration++
	generation := s.watchGeneration
	s.sequence++
	s.ready = true
	s.listReady = false
	s.watchAt = time.Now().UTC()
	s.reason = ""
	s.mu.Unlock()
	lifetime := 10 * time.Minute
	if options.TimeoutSeconds != nil && *options.TimeoutSeconds > 0 {
		lifetime = time.Duration(*options.TimeoutSeconds)*time.Second + 30*time.Second
	}
	wrapped := &continuityWatch{upstream: w, results: make(chan watch.Event), done: make(chan struct{}), state: s, generation: generation}
	go wrapped.run(ctx, lifetime)
	return wrapped, nil
}
func (s *informerContinuity) watchEvent(generation uint64, event watch.Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.watchGeneration || !s.ready {
		return false
	}
	if event.Type == watch.Error {
		s.invalidateLocked(continuityFailure("watch", apierrors.FromObject(event.Object)))
		return false
	}
	obj, err := meta.Accessor(event.Object)
	if err != nil || obj.GetResourceVersion() == "" {
		s.invalidateLocked("invalid_watch_object")
		return false
	}
	if event.Type == watch.Bookmark {
		s.rv = obj.GetResourceVersion()
		s.sequence++
		return true
	}
	validType := true
	switch s.resource {
	case "pods":
		_, validType = event.Object.(*corev1.Pod)
	case "nodes":
		_, validType = event.Object.(*corev1.Node)
	}
	if !validType {
		s.invalidateLocked("invalid_watch_object_type")
		return false
	}
	v, valid := versionOf(obj)
	if !valid {
		s.invalidateLocked("invalid_watch_identity")
		return false
	}
	key := objectKey(obj)
	switch event.Type {
	case watch.Added, watch.Modified:
		s.objects[key] = v
	case watch.Deleted:
		if previous, exists := s.objects[key]; exists && previous.UID != v.UID {
			s.invalidateLocked("delete_uid_conflict")
			return false
		}
		delete(s.objects, key)
	default:
		s.invalidateLocked("invalid_watch_event")
		return false
	}
	s.rv = v.RV
	s.sequence++
	return true
}
func (s *informerContinuity) ended(generation uint64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation == s.watchGeneration && s.ready {
		s.invalidateLocked(reason)
	}
}
func (s *informerContinuity) snapshot() continuitySnapshot {
	if s == nil {
		return continuitySnapshot{reason: "not_tracked"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	objects := make(map[string]objectVersion, len(s.objects))
	for k, v := range s.objects {
		objects[k] = v
	}
	return continuitySnapshot{sequence: s.sequence, objects: objects, ready: s.ready, reason: s.reason, rv: s.rv, listedAt: s.listedAt, watchAt: s.watchAt}
}
func (s *informerContinuity) unchanged(v continuitySnapshot) bool {
	if s == nil {
		return true // not tracked before or after the read
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Readiness is checked separately by continuityCoverage. A stable outage
	// is not a concurrent change and must retain its original diagnostic.
	return s.sequence == v.sequence
}
func continuityCoverage(entity, source string, before continuitySnapshot, actual []metav1.Object, unchanged bool) *viewv1.Coverage {
	result := &viewv1.Coverage{Entity: entity, Observed: int32(len(actual))}
	state := &viewv1.SourceState{Source: source, State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"continuity": before.reason}}
	result.Sources = []*viewv1.SourceState{state}
	if strings.HasSuffix(before.reason, "_forbidden") || strings.HasSuffix(before.reason, "_unauthorized") {
		state.Reason = "no_permission"
	}
	if !before.listedAt.IsZero() {
		state.Params["listCompletedAt"] = before.listedAt.Format(time.RFC3339Nano)
	}
	if !before.watchAt.IsZero() {
		state.Params["watchStartedAt"] = before.watchAt.Format(time.RFC3339Nano)
	}
	if before.rv != "" {
		state.Params["resourceVersion"] = before.rv
	}
	valid := before.ready && unchanged && len(actual) == len(before.objects)
	seen := map[string]bool{}
	for _, obj := range actual {
		v, ok := versionOf(obj)
		key := objectKey(obj)
		expected, exists := before.objects[key]
		if !ok || !exists || expected != v || seen[key] {
			valid = false
		}
		seen[key] = true
	}
	if valid {
		total := int32(len(actual))
		result.Total = &total
		result.Complete = true
		state.State = resourceview.Known
		state.Reason = ""
		state.Params["continuity"] = "list_watch_and_consumer_matched"
	} else if before.ready {
		state.Params["continuity"] = "consumer_pending_or_changed"
	}
	return result
}

type continuityWatch struct {
	upstream   watch.Interface
	results    chan watch.Event
	done       chan struct{}
	once       sync.Once
	state      *informerContinuity
	generation uint64
}

func (w *continuityWatch) Stop() {
	w.once.Do(func() { w.state.ended(w.generation, "watch_stopped"); close(w.done); w.upstream.Stop() })
}
func (w *continuityWatch) ResultChan() <-chan watch.Event { return w.results }
func (w *continuityWatch) run(ctx context.Context, lifetime time.Duration) {
	defer close(w.results)
	defer w.Stop()
	timer := time.NewTimer(lifetime)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-timer.C:
			w.state.ended(w.generation, "watch_deadline_exceeded")
			return
		case event, ok := <-w.upstream.ResultChan():
			if !ok {
				w.state.ended(w.generation, "watch_closed")
				return
			}
			valid := w.state.watchEvent(w.generation, event)
			if !valid && event.Type != watch.Error {
				event = watch.Event{Type: watch.Error, Object: &metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonExpired, Code: 410, Message: "shared watch evidence invalid; relist required"}}
			}
			select {
			case w.results <- event:
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-timer.C:
				return
			}
			if !valid {
				return
			}
		}
	}
}

type trackedInformerClient struct {
	kubernetes.Interface
	state *sharedContinuity
}

func (c trackedInformerClient) CoreV1() typedcore.CoreV1Interface {
	return trackedCore{CoreV1Interface: c.Interface.CoreV1(), state: c.state}
}

type trackedCore struct {
	typedcore.CoreV1Interface
	state *sharedContinuity
}

func (c trackedCore) Pods(namespace string) typedcore.PodInterface {
	return trackedPods{PodInterface: c.CoreV1Interface.Pods(namespace), state: c.state.pods, namespace: namespace}
}
func (c trackedCore) Nodes() typedcore.NodeInterface {
	return trackedNodes{NodeInterface: c.CoreV1Interface.Nodes(), state: c.state.nodes}
}

type trackedPods struct {
	typedcore.PodInterface
	state     *informerContinuity
	namespace string
}

func (c trackedPods) List(ctx context.Context, o metav1.ListOptions) (*corev1.PodList, error) {
	c.state.beginList(o)
	v, e := c.PodInterface.List(ctx, o)
	c.state.listResult(o, v, e, c.namespace == metav1.NamespaceAll)
	return v, e
}
func (c trackedPods) Watch(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
	if e := c.state.allowWatch(o, c.namespace == metav1.NamespaceAll); e != nil {
		return nil, e
	}
	v, e := c.PodInterface.Watch(ctx, o)
	return c.state.wrapWatch(ctx, o, v, e)
}

type trackedNodes struct {
	typedcore.NodeInterface
	state *informerContinuity
}

func (c trackedNodes) List(ctx context.Context, o metav1.ListOptions) (*corev1.NodeList, error) {
	c.state.beginList(o)
	v, e := c.NodeInterface.List(ctx, o)
	c.state.listResult(o, v, e, true)
	return v, e
}
func (c trackedNodes) Watch(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
	if e := c.state.allowWatch(o, true); e != nil {
		return nil, e
	}
	v, e := c.NodeInterface.Watch(ctx, o)
	return c.state.wrapWatch(ctx, o, v, e)
}

func continuityFailure(operation string, err error) string {
	suffix := "failed"
	switch {
	case apierrors.IsForbidden(err):
		suffix = "forbidden"
	case apierrors.IsUnauthorized(err):
		suffix = "unauthorized"
	case apierrors.IsResourceExpired(err) || apierrors.IsGone(err):
		suffix = "expired"
	case apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err):
		suffix = "timeout"
	}
	return operation + "_" + suffix
}
