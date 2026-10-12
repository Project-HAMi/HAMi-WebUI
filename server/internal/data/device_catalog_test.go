package data

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var testDeviceConfig = &conf.DeviceConfig{Enabled: true, Namespace: "kube-system", Name: "hami-scheduler-device"}

func deviceConfigMap(t *testing.T, fixture string) *corev1.ConfigMap {
	t.Helper()
	content, err := os.ReadFile("../devicecatalog/testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "hami-scheduler-device", ResourceVersion: "1"},
		Data:       map[string]string{defaultDeviceConfigKey: string(content)},
	}
}

func startTestCatalog(t *testing.T, client *fake.Clientset, config *conf.DeviceConfig) devicecatalog.Source {
	t.Helper()
	return newDeviceCatalog(client, config, log.NewHelper(log.NewStdLogger(io.Discard)), 5*time.Second)
}

func waitForCatalog(t *testing.T, source devicecatalog.Source, want devicecatalog.State) *devicecatalog.Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if snapshot := source.Snapshot(); snapshot.State == want {
			return snapshot
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("catalog state = %s (%s), want %s", source.Snapshot().State, source.Snapshot().Reason, want)
	return nil
}

func TestDeviceCatalogDisabledWithoutAReference(t *testing.T) {
	for _, config := range []*conf.DeviceConfig{nil, {Namespace: "kube-system", Name: "x"}, {Enabled: true, Name: "x"}} {
		if state := startTestCatalog(t, fake.NewSimpleClientset(), config).Snapshot().State; state != devicecatalog.StateDisabled {
			t.Fatalf("%v gave %s", config, state)
		}
	}
}

func TestDeviceCatalogLoadsUpdatesAndNotifies(t *testing.T) {
	client := fake.NewSimpleClientset(deviceConfigMap(t, "hami-v2.9.0.yaml"))
	source := startTestCatalog(t, client, testDeviceConfig)
	snapshot := source.Snapshot()
	if snapshot.State != devicecatalog.StateLoaded || len(snapshot.Ascend["Ascend910C"].Templates) != 0 || snapshot.ResourceVersion != "1" {
		t.Fatalf("first read = %+v", snapshot)
	}
	var mu sync.Mutex
	var changes [][2]*devicecatalog.Snapshot
	source.Subscribe(func(previous, current *devicecatalog.Snapshot) {
		mu.Lock()
		changes = append(changes, [2]*devicecatalog.Snapshot{previous, current})
		mu.Unlock()
	})
	updated := deviceConfigMap(t, "hami-v2.10.0.yaml")
	if _, err := client.CoreV1().ConfigMaps("kube-system").Update(context.Background(), updated, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	// Subscribers run after the snapshot is swapped, so wait for them rather than for the snapshot.
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		var last [2]*devicecatalog.Snapshot
		if len(changes) > 0 {
			last = changes[len(changes)-1]
		}
		mu.Unlock()
		if last[1] != nil && len(last[1].Ascend["Ascend910C"].Templates) == 2 {
			if last[1] != source.Snapshot() {
				t.Fatal("the last change is not the current snapshot")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("subscribers never saw the updated configuration; first read had %d templates", len(snapshot.Ascend["Ascend910C"].Templates))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDeviceCatalogReportsMissingInvalidAndDeleted(t *testing.T) {
	missing := startTestCatalog(t, fake.NewSimpleClientset(), testDeviceConfig)
	waitForCatalog(t, missing, devicecatalog.StateMissing)

	noKey := deviceConfigMap(t, "hami-v2.10.0.yaml")
	noKey.Data = map[string]string{"other.yaml": "vnpus: []"}
	if snapshot := waitForCatalog(t, startTestCatalog(t, fake.NewSimpleClientset(noKey), testDeviceConfig), devicecatalog.StateInvalid); snapshot.Reason != `key "device-config.yaml" not found` {
		t.Fatalf("reason = %q", snapshot.Reason)
	}
	broken := deviceConfigMap(t, "hami-v2.10.0.yaml")
	broken.Data[defaultDeviceConfigKey] = "vnpus: ["
	waitForCatalog(t, startTestCatalog(t, fake.NewSimpleClientset(broken), testDeviceConfig), devicecatalog.StateInvalid)

	client := fake.NewSimpleClientset(deviceConfigMap(t, "hami-v2.10.0.yaml"))
	source := startTestCatalog(t, client, testDeviceConfig)
	waitForCatalog(t, source, devicecatalog.StateLoaded)
	if err := client.CoreV1().ConfigMaps("kube-system").Delete(context.Background(), "hami-scheduler-device", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForCatalog(t, source, devicecatalog.StateMissing)
}

func TestDeviceCatalogReportsForbiddenWithoutWaitingAndRecovers(t *testing.T) {
	client := fake.NewSimpleClientset(deviceConfigMap(t, "hami-v2.10.0.yaml"))
	var forbidden atomic.Bool
	forbidden.Store(true)
	deny := func(action k8stesting.Action) (bool, runtime.Object, error) {
		if forbidden.Load() {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "hami-scheduler-device", errors.New("no RBAC"))
		}
		return false, nil, nil
	}
	client.PrependReactor("list", "configmaps", deny)
	started := time.Now()
	source := startTestCatalog(t, client, testDeviceConfig)
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("a forbidden read delayed startup by %s", elapsed)
	}
	if snapshot := source.Snapshot(); snapshot.State != devicecatalog.StateForbidden || snapshot.Loaded() {
		t.Fatalf("forbidden read = %+v", snapshot)
	}
	forbidden.Store(false)
	waitForCatalog(t, source, devicecatalog.StateLoaded)
}

func TestDeviceCatalogKeepsTheLastConfigurationOnTransientErrors(t *testing.T) {
	loaded := &devicecatalog.Snapshot{State: devicecatalog.StateLoaded, ResourceVersion: "7"}
	r := &deviceCatalog{now: time.Now, listErr: errors.New("connection reset"), listed: true}
	if got := r.build(loaded); got != loaded {
		t.Fatalf("transient error replaced the loaded configuration: %+v", got)
	}
	if got := r.build(&devicecatalog.Snapshot{State: devicecatalog.StateLoading}); got.State != devicecatalog.StateError {
		t.Fatalf("transient error before the first read = %+v", got)
	}
}

func TestSchedulingRequestsNameAscendResourcesFromTheDeviceConfiguration(t *testing.T) {
	content, err := os.ReadFile("../devicecatalog/testdata/hami-v2.10.0.yaml")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := devicecatalog.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &devicecatalog.Snapshot{State: devicecatalog.StateLoaded, Ascend: parsed.Ascend}
	tests := []struct {
		name, value string
		want        biz.SchedulingResource
	}{
		{"huawei.com/Ascend910B3", "1", biz.SchedulingResource{Kind: "count", Vendor: "Ascend"}},
		{"huawei.com/Ascend910B3-memory", "16384", biz.SchedulingResource{Kind: "memory", Unit: "MiB", Vendor: "Ascend"}},
		{"huawei.com/Ascend910B3-core", "40", biz.SchedulingResource{Kind: "core", Unit: "%", Vendor: "Ascend"}},
		{"nvidia.com/gpu", "2", biz.SchedulingResource{Kind: "count", Vendor: "NVIDIA"}},
		{"custom.io/card", "1", biz.SchedulingResource{Kind: "raw"}},
	}
	for _, tt := range tests {
		tt.want.Name, tt.want.Value = tt.name, tt.value
		if got := schedulingResource(loaded, corev1.ResourceName(tt.name), resource.MustParse(tt.value)); got != tt.want {
			t.Errorf("%s = %+v, want %+v", tt.name, got, tt.want)
		}
	}
	if got := schedulingResource(nil, "huawei.com/Ascend910B3", resource.MustParse("1")); got.Kind != "raw" || got.Vendor != "" {
		t.Fatalf("without a configuration = %+v", got)
	}
}
