package data

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"vgpu/internal/biz"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func TestHAMiComponentsCountsPodsByChartLabel(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	pod := func(name, component, node string, ready bool, phase corev1.PodPhase, restarts int32) *corev1.Pod {
		status := corev1.ConditionFalse
		if ready {
			status = corev1.ConditionTrue
		}
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "kube-system", Labels: map[string]string{hamiComponentLabel: component}},
			Spec:       corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				PodIPs:            []corev1.PodIP{{IP: "10.0.0." + name[len(name)-1:]}},
				Phase:             phase,
				Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
				ContainerStatuses: []corev1.ContainerStatus{{RestartCount: restarts}},
			},
		}
	}
	for _, item := range []*corev1.Pod{
		pod("scheduler-a", biz.HAMiScheduler, "cp-1", true, corev1.PodRunning, 0),
		pod("plugin-a", biz.HAMiDevicePlugin, "gpu-1", true, corev1.PodRunning, 2),
		pod("plugin-b", biz.HAMiDevicePlugin, "gpu-2", false, corev1.PodPending, 1),
		pod("plugin-old", biz.HAMiDevicePlugin, "gpu-3", false, corev1.PodFailed, 0),
		pod("other", "hami-webui", "gpu-1", true, corev1.PodRunning, 0),
	} {
		if err := indexer.Add(item); err != nil {
			t.Fatal(err)
		}
	}
	repo := &clusterRepo{pods: &podRepo{podLister: listerscorev1.NewPodLister(indexer)}}

	snapshot, err := repo.Components(context.Background())
	components := snapshot.HAMi
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != 2 {
		t.Fatalf("components = %+v", components)
	}
	plugin, scheduler := components[0], components[1]
	if plugin.Name != biz.HAMiDevicePlugin || plugin.Pods != 2 || plugin.Ready != 1 || plugin.Nodes != 2 || plugin.Restarts != 3 {
		t.Fatalf("device plugin = %+v", plugin)
	}
	if scheduler.Name != biz.HAMiScheduler || scheduler.Pods != 1 || scheduler.Ready != 1 || scheduler.Members[0].Name != "scheduler-a" || scheduler.Members[0].IPs[0] != "10.0.0.a" || scheduler.Members[0].Namespace != "kube-system" {
		t.Fatalf("scheduler = %+v", scheduler)
	}
}

func TestServerVersionStopsAtTheContextDeadline(t *testing.T) {
	newRepo := func(handler http.HandlerFunc) *clusterRepo {
		apiServer := httptest.NewServer(handler)
		t.Cleanup(apiServer.Close)
		client, err := kubernetes.NewForConfig(&rest.Config{Host: apiServer.URL})
		if err != nil {
			t.Fatal(err)
		}
		return &clusterRepo{data: &Data{k8sCl: client}}
	}

	fast := newRepo(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gitVersion":"v1.31.2"}`))
	})
	if version, err := fast.ServerVersion(context.Background()); err != nil || version != "v1.31.2" {
		t.Fatalf("ServerVersion = %q, %v", version, err)
	}

	slow := newRepo(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := slow.ServerVersion(ctx); err == nil || time.Since(start) > time.Second {
		t.Fatalf("slow ServerVersion returned %v after %s", err, time.Since(start))
	}
}

func TestVendorPluginNeedsTheDeploymentIdentity(t *testing.T) {
	controller := true
	for _, tc := range []struct{ name, role, owner, container, image, want string }{
		{"AMD plugin", "", "amd-gpu-device-plugin-daemonset", "amd-gpu-dp-cntr", "ghcr.io/project-hami/amd-device-plugin:0.0.4", "amd-hami"},
		{"AMD monitor sharing the plugin image", "", "amd-gpu-monitor-daemonset", "amd-gpu-monitor-cntr", "ghcr.io/project-hami/amd-device-plugin:0.0.4", ""},
		{"ROCm native plugin", "", "amdgpu-device-plugin-daemonset", "amdgpu-dp-cntr", "rocm/k8s-device-plugin", ""},
		{"ROCm node labeller", "", "amdgpu-labeller-daemonset", "amdgpu-lr-cntr", "rocm/k8s-device-plugin:labeller-latest", ""},
		{"HAMi Ascend", "hami-ascend-device-plugin", "hami-ascend-device-plugin", "device-plugin", "docker.io/projecthami/ascend-device-plugin:v1.3.0", "ascend-hami"},
		{"native Ascend is separate", "hami-ascend-device-plugin", "ascend", "device-plugin", "other/ascend-device-plugin:v1.3.0", ""},
		{"HCU", "", "hcu-device-plugin-hami", "hcu-dp-cntr", "harbor.sourcefind.cn:5443/hcu/admin/base/hcu-device-plugin:v3.0.2", "hcu-plugin"},
		{"legacy DCU", "", "dcu-device-plugin-daemonset", "dcu-dp-cntr", "projecthami/dcu-vgpu-device-plugin:v1.0.5", "legacy-dcu"},
		{"MetaX", "", "metax-gpu-device", "gpu-device", "cr.metax-tech.com/cloud/gpu-device@sha256:abc", "metax-native"},
		{"unrelated gpu-device", "", "metax-gpu-device", "gpu-device", "other/cloud/gpu-device:latest", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: tc.owner, Controller: &controller}}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: tc.container, Image: tc.image}}}}
			if tc.name == "HCU" {
				pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "RESOURCE_REGISTER_STRATEGY", Value: "hami"}}
			}
			if id, _ := devicePluginIdentity(pod, tc.role); id != tc.want {
				t.Fatalf("got %q, want %q", id, tc.want)
			}
			pod.OwnerReferences = nil
			if id, _ := devicePluginIdentity(pod, tc.role); id != "" {
				t.Fatalf("unmanaged Pod classified as %q", id)
			}
		})
	}
}

func TestHCURegistrationStrategy(t *testing.T) {
	for _, tc := range []struct {
		env  string
		args []string
		want bool
	}{
		{"", nil, false}, {"mixed", nil, false}, {"mig", nil, false}, {"hami", nil, true},
		{"hami", []string{"--strategy=mig"}, false}, {"mixed", []string{"--strategy", "hami"}, true},
	} {
		c := corev1.Container{Args: tc.args}
		if tc.env != "" {
			c.Env = []corev1.EnvVar{{Name: "RESOURCE_REGISTER_STRATEGY", Value: tc.env}}
		}
		if got := hcuHAMiRegistration(c); got != tc.want {
			t.Errorf("env=%q args=%v: got %v, want %v", tc.env, tc.args, got, tc.want)
		}
	}
}

func TestStandaloneCambriconDevicePlugin(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cambricon-device-plugin-static-pod"},
		Spec: corev1.PodSpec{NodeName: "mlu-a", Containers: []corev1.Container{{
			Name: "cambricon-device-plugin-ctr", Image: "cambricon-device-plugin:v2.5.0",
			Command: []string{"/usr/bin/k8s-device-plugin"},
		}}},
	}
	if id, _ := devicePluginIdentity(pod, ""); id != "mlu-plugin" {
		t.Fatalf("single-Pod manifest: got %q", id)
	}
	pod.Name += "-mlu-a"
	pod.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "hash"}
	if id, _ := devicePluginIdentity(pod, ""); id != "mlu-plugin" {
		t.Fatalf("mirror Pod: got %q", id)
	}
	pod.Spec.Containers[0].Command = []string{"sleep"}
	if id, _ := devicePluginIdentity(pod, ""); id != "" {
		t.Fatalf("same image with a different command: got %q", id)
	}
}
