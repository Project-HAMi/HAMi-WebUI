package service

import (
	"errors"
	"slices"
	"testing"
	pb "vgpu/api/v1"

	"vgpu/internal/biz"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func deviceSources(components biz.ClusterComponents, podsChecked bool, targets []v1.ActiveTarget, targetsChecked bool) *pb.DeviceSources {
	sources, _ := readDeviceSources(components, podsChecked, func(visit func(v1.ActiveTarget)) error {
		for _, t := range targets {
			visit(t)
		}
		if !targetsChecked {
			return errors.New("targets unavailable")
		}
		return nil
	})
	return sources
}

func TestDeviceSourcesKeepPodReadinessSeparateFromExporterHealth(t *testing.T) {
	pods := []biz.DevicePluginPod{
		{ID: "nvidia-hami", Image: "registry:5000/hami:v2.11.0@sha256:123", Ready: true},
		{ID: "nvidia-hami", Image: "registry:5000/hami:v2.10.0"},
		{ID: "hami-device-plugin", Ready: true},
	}
	labels := map[string]string{"__meta_kubernetes_pod_container_image": "mirror/nvidia/k8s/dcgm-exporter:4.6.1-4.8.4-distroless",
		"__meta_kubernetes_pod_container_name": "nvidia-dcgm-exporter", "__meta_kubernetes_pod_label_app_kubernetes_io_component": "nvidia-dcgm-exporter",
		"__meta_kubernetes_pod_uid": "test-pod", "__meta_kubernetes_pod_controller_kind": "DaemonSet"}
	targets := []v1.ActiveTarget{{DiscoveredLabels: labels, Health: v1.HealthGood}, {DiscoveredLabels: labels, Health: v1.HealthBad}, {}}
	r := deviceSources(biz.ClusterComponents{Plugins: pods}, true, targets, true)
	if slices.Contains(r.CheckedKeys, "amd-metrics") {
		t.Fatal("a static target can hide an exporter in a mixed target list")
	}
	if len(r.Plugins) != 2 || r.Plugins[0].Pods != 2 || r.Plugins[0].Ready != 1 || r.Plugins[0].Version != "" || r.Plugins[1].Vendor != "" {
		t.Fatalf("plugins: %v", r.Plugins)
	}
	if len(r.Exporters) != 1 || r.Exporters[0].Targets != 2 || r.Exporters[0].Up != 1 || r.Exporters[0].Version != "4.8.4" {
		t.Fatalf("exporters: %v", r.Exporters)
	}
	// GPU Operator's DRA manifest uses the same component label and image, with a different container name.
	labels["__meta_kubernetes_pod_container_name"] = "nvidia-dcgm-exporter-dra"
	if id, _, _ := exporterIdentity(targets[0], nil); id != "dcgm" {
		t.Fatalf("DRA target classified as %q", id)
	}
	labels["__meta_kubernetes_pod_container_name"] = "exporter"
	labels["__meta_kubernetes_pod_label_app_kubernetes_io_name"] = "custom-chart-name"
	labels["__meta_kubernetes_pod_label_app_kubernetes_io_component"] = "custom-chart-name"
	if id, _, _ := exporterIdentity(targets[0], nil); id != "dcgm" {
		t.Fatalf("standalone Helm target classified as %q", id)
	}
	labels["__meta_kubernetes_pod_container_image"] = "other-vendor/dcgm-exporter:1.2.3"
	if len(deviceSources(biz.ClusterComponents{}, false, targets, true).Exporters) != 0 {
		t.Fatal("misattributed an unrelated image")
	}
	r = deviceSources(biz.ClusterComponents{Plugins: pods}, true, nil, false)
	if r.ExportersChecked || !slices.Contains(r.CheckedKeys, "amd-hami") || slices.Contains(r.CheckedKeys, "amd-metrics") || !r.PluginsChecked {
		t.Fatalf("source failure: %v", r)
	}
}

func TestDCGMOperatorAppLabel(t *testing.T) {
	// GPU Operator v26.3.3 selects the DaemonSet's pods by app alone.
	labels := map[string]string{
		"__meta_kubernetes_pod_uid":             "example-pod",
		"__meta_kubernetes_pod_controller_kind": "DaemonSet",
		"__meta_kubernetes_pod_controller_name": "nvidia-dcgm-exporter",
		"__meta_kubernetes_pod_label_app":       "nvidia-dcgm-exporter",
		"__meta_kubernetes_pod_container_name":  "nvidia-dcgm-exporter",
		"__meta_kubernetes_pod_container_image": "mirror/nvidia/k8s/dcgm-exporter:4.5.3-4.8.2-distroless",
	}
	target := v1.ActiveTarget{DiscoveredLabels: labels}
	if id, version, _ := exporterIdentity(target, nil); id != "dcgm" || version != "4.8.2" {
		t.Fatalf("GPU Operator target: got %q, %q", id, version)
	}
	labels["__meta_kubernetes_pod_container_name"] = "sidecar"
	if id, _, _ := exporterIdentity(target, nil); id != "" {
		t.Fatalf("sidecar classified as %q", id)
	}
}

func TestVendorExporterTargets(t *testing.T) {
	for _, tc := range []struct{ container, image, id, version string }{
		{"npu-exporter", "npu-exporter:v5.0.RC1", "npu", "v5.0.RC1"},
		{"hcu-exporter", "harbor.sourcefind.cn:5443/hcu/admin/base/hcu-exporter:v3.0.0", "hcu-exporter", "v3.0.0"},
		{"dcu-exporter", "dcu-exporter:v2.0.0.240718", "dcu-exporter", "v2.0.0.240718"},
		{"mx-exporter", "cr.metax-tech.com/cloud/mx-exporter:latest", "mx", ""},
		{"amdgpu-metrics-exporter-container", "docker.io/rocm/device-metrics-exporter:v1.6.0", "amd-metrics", "v1.6.0"},
		{"amd-device-metrics-exporter-container", "rocm/device-metrics-exporter:v1.6.0", "", ""},
		{"neuron-monitor", "public.ecr.aws/neuron/neuron-monitor:1.3.0", "neuron-monitor", "1.3.0"},
		{"mx-exporter", "harbor.internal/platform/mx-exporter:0.16.2", "mx", "0.16.2"},
		{"mx-exporter", "other/cloud/mx-exporter:0.16.2", "", ""},
	} {
		t.Run(tc.image, func(t *testing.T) {
			target := v1.ActiveTarget{DiscoveredLabels: map[string]string{
				"__meta_kubernetes_pod_uid": "pod-uid", "__meta_kubernetes_pod_controller_kind": "DaemonSet",
				"__meta_kubernetes_pod_label_app": tc.container, "__meta_kubernetes_pod_label_app_kubernetes_io_name": tc.container,
				"__meta_kubernetes_pod_container_name": tc.container, "__meta_kubernetes_pod_container_image": tc.image,
			}}
			id, version, _ := exporterIdentity(target, nil)
			if id != tc.id || version != tc.version {
				t.Fatalf("got (%q, %q), want (%q, %q)", id, version, tc.id, tc.version)
			}
			delete(target.DiscoveredLabels, "__meta_kubernetes_pod_uid")
			if id, _, _ := exporterIdentity(target, nil); id != "" {
				t.Fatalf("unattributed target classified as %q", id)
			}
		})
	}
}

func TestStaticTargetsCannotIdentifyAnExporter(t *testing.T) {
	static := v1.ActiveTarget{Labels: model.LabelSet{"job": "mx-exporter"}, Health: v1.HealthGood}
	result := deviceSources(biz.ClusterComponents{}, false, []v1.ActiveTarget{static}, true)
	if len(result.Exporters) != 0 || len(result.CheckedKeys) != 0 || !result.ExportersChecked {
		t.Fatalf("static target: %v", result)
	}
	if empty := deviceSources(biz.ClusterComponents{}, false, nil, true); len(empty.CheckedKeys) != len(exporterIdentities) {
		t.Fatalf("empty target list: %v", empty)
	}
}

func TestEndpointSliceWithoutContainerLabels(t *testing.T) {
	inventory := biz.ClusterComponents{MLUEndpoints: map[string]biz.ExporterEndpoint{"mlu-uid": {Container: "mlu-prod", Port: 30108, Image: "harbor.example/ops/cambricon-mlu-exporter:v2.5.0"}}}
	l := map[string]string{"__meta_kubernetes_pod_uid": "mlu-uid", "__meta_kubernetes_pod_controller_kind": "DaemonSet", "__meta_kubernetes_endpointslice_port_name": "http-metrics", "__address__": "[2001:db8::1]:30108"}
	target := v1.ActiveTarget{DiscoveredLabels: l, Health: v1.HealthGood}
	self := v1.ActiveTarget{ScrapeURL: "http://localhost:9090/metrics", DiscoveredLabels: map[string]string{"job": "prometheus"}}
	r := deviceSources(inventory, true, []v1.ActiveTarget{target, self}, true)
	if len(r.Exporters) != 1 || r.Exporters[0].Version != "v2.5.0" || r.Exporters[0].Up != 1 || !slices.Contains(r.CheckedKeys, "npu") {
		t.Fatalf("official ServiceMonitor and Prometheus self scrape: %v", r)
	}
	for _, tc := range []struct{ key, value string }{
		{"__address__", "[2001:db8::1]:9091"}, {"__meta_kubernetes_pod_uid", "recreated-pod"}, {"__meta_kubernetes_pod_container_name", "sidecar"},
	} {
		old := l[tc.key]
		l[tc.key] = tc.value
		if id, _, _ := exporterIdentity(target, inventory.MLUEndpoints); id != "" {
			t.Fatalf("misattributed %v to %s", tc, id)
		}
		l[tc.key] = old
	}
}

func TestExporterInspectionWithKubernetesSystemTargets(t *testing.T) {
	// These cases use the discovery labels available when an Endpoint lacks container metadata.
	for _, tc := range []struct {
		name    string
		url     string
		labels  map[string]string
		unknown bool
	}{
		{"kubelet Endpoints", "https://host:10250/metrics/cadvisor", map[string]string{"__meta_kubernetes_endpoint_address_target_kind": "Node"}, false},
		{"kubelet EndpointSlice", "https://host:10250/metrics/probes", map[string]string{"__meta_kubernetes_endpointslice_address_target_kind": "Node"}, false},
		{"other Node service", "http://host:9500/metrics", map[string]string{"__meta_kubernetes_endpointslice_address_target_kind": "Node"}, true},
		{"managed application", "http://host:9500/metrics", map[string]string{"__meta_kubernetes_pod_uid": "a", "__meta_kubernetes_pod_controller_kind": "ReplicaSet"}, false},
		{"unattributed DaemonSet", "http://host:10249/metrics", map[string]string{"__meta_kubernetes_namespace": "kube-system", "__meta_kubernetes_pod_uid": "a", "__meta_kubernetes_pod_controller_kind": "DaemonSet"}, true},
		{"kube-proxy", "http://host:10249/metrics", map[string]string{"__meta_kubernetes_namespace": "kube-system", "__meta_kubernetes_pod_uid": "a", "__meta_kubernetes_pod_controller_kind": "DaemonSet", "__meta_kubernetes_pod_controller_name": "kube-proxy", "__meta_kubernetes_pod_label_k8s_app": "kube-proxy"}, false},
		{"control plane service", "https://host:10259/metrics", map[string]string{"__meta_kubernetes_namespace": "kube-system", "__meta_kubernetes_service_label_jobLabel": "kube-scheduler"}, false},
		{"custom endpoint on system port", "https://host:10259/metrics", map[string]string{"__meta_kubernetes_namespace": "kube-system"}, true},
		{"federation", "http://host:19090/federate", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := v1.ActiveTarget{ScrapeURL: tc.url, DiscoveredLabels: tc.labels}
			if got := couldHideExporter(target); got != tc.unknown {
				t.Errorf("couldHideExporter() = %t, want %t", got, tc.unknown)
			}
		})
	}
}

func TestExporterImageAloneWithholdsOnlyItsOwnAbsence(t *testing.T) {
	target := func(container, image string) v1.ActiveTarget {
		return v1.ActiveTarget{Health: v1.HealthGood, DiscoveredLabels: map[string]string{
			"__meta_kubernetes_pod_uid": "pod-uid", "__meta_kubernetes_pod_controller_kind": "DaemonSet",
			"__meta_kubernetes_pod_container_name": container, "__meta_kubernetes_pod_label_app_kubernetes_io_name": container,
			"__meta_kubernetes_pod_container_image": image,
		}}
	}
	targets := []v1.ActiveTarget{
		target("gpu-metrics", "harbor.internal/platform/mx-exporter:0.16.2"),
		target("amd-device-metrics-exporter-container", "rocm/device-metrics-exporter:v1.6.0"),
		target("node-exporter", "prom/node-exporter:v1.8.0"),
	}
	r := deviceSources(biz.ClusterComponents{}, false, targets, true)
	if !r.ExportersChecked || len(r.Exporters) != 0 || slices.Contains(r.CheckedKeys, "mx") || len(r.CheckedKeys) != len(exporterIdentities)-1 {
		t.Fatalf("renamed MetaX exporter: %v", r)
	}
	targets = append(targets, target("mx-exporter", "harbor.internal/platform/mx-exporter:0.16.2"))
	r = deviceSources(biz.ClusterComponents{}, false, targets, true)
	if len(r.Exporters) != 1 || r.Exporters[0].Id != "mx" || r.Exporters[0].Targets != 1 || r.Exporters[0].Up != 1 {
		t.Fatalf("only the verified scrape should count: %v", r.Exporters)
	}
}
