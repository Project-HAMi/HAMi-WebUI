package service

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

var dcgmTag = regexp.MustCompile(`^(?:[0-9]+\.[0-9]+\.[0-9]+-)?([0-9]+\.[0-9]+\.[0-9]+)(?:-distroless|-ubuntu[0-9.]+|-ubi[0-9.]+)?$`)

// An aggregate has a version only when all members report the same identifiable tag.
func addVersion(current, next string, first bool) string {
	if first || current == next {
		return next
	}
	return ""
}

type sourceIdentity struct{ id, vendor string }

var pluginIdentities = []sourceIdentity{
	{"nvidia-hami", "NVIDIA"}, {"ascend-hami", "Ascend"}, {"hcu-plugin", "Hygon"},
	{"legacy-dcu", "Hygon"}, {"metax-native", "Metax"}, {"hami-device-plugin", ""},
	{"mlu-plugin", "MLU"}, {"amd-hami", "AMD"}, {"gcu-plugin", "Enflame"}, {"vxpu", "Kunlunxin"},
	{"neuron", "AWS"}, {"biren-plugin", "Biren"}, {"vastai-plugin", "Vastai"},
}
var exporterIdentities = []sourceIdentity{
	{"dcgm", "NVIDIA"}, {"npu", "Ascend"}, {"hcu-exporter", "Hygon"}, {"dcu-exporter", "Hygon"}, {"mx", "Metax"},
	{"mlu-exporter", "MLU"}, {"amd-metrics", "AMD"}, {"ix-exporter", "Iluvatar"},
	{"gcu-exporter", "Enflame"}, {"neuron-monitor", "AWS"}, {"xpu-exporter", "Kunlunxin"},
}

// MindCluster and the legacy DCU exporter also ship dotted RC and four-part tags.
var releaseTag = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(?:\.(?:RC[0-9]+|[0-9]+(?:\.[0-9]+)?))?(?:[-.+][[:alnum:]._-]+)?$`)

func readDeviceSources(components biz.ClusterComponents, podsChecked bool, walkTargets func(func(v1.ActiveTarget)) error) (*pb.DeviceSources, error) {
	result := &pb.DeviceSources{PluginsChecked: podsChecked}
	if podsChecked {
		rows := map[string]*pb.DevicePlugin{}
		for _, pod := range components.Plugins {
			row := rows[pod.ID]
			if row == nil {
				row = &pb.DevicePlugin{Id: pod.ID}
				rows[pod.ID] = row
			}
			_, tag := biz.ImageReference(pod.Image)
			if !releaseTag.MatchString(tag) {
				tag = ""
			}
			row.Version = addVersion(row.Version, tag, row.Pods == 0)
			row.Pods++
			if pod.Ready {
				row.Ready++
			}
		}
		for _, s := range pluginIdentities {
			if s.vendor != "" {
				result.CheckedKeys = append(result.CheckedKeys, s.id)
			}
			if row := rows[s.id]; row != nil {
				row.Vendor = s.vendor
				result.Plugins = append(result.Plugins, row)
			}
		}
	}

	rows := map[string]*pb.DeviceExporter{}
	// An unattributed scrape can hide a vendor. Report matches, but leave missing exporters unchecked.
	inspected := true
	uncertain := map[string]bool{}
	err := walkTargets(func(target v1.ActiveTarget) {
		id, version, candidate := exporterIdentity(target, components.MLUEndpoints)
		if candidate != "" {
			uncertain[candidate] = true
			return
		}
		if id == "" {
			if couldHideExporter(target) {
				inspected = false
			}
			return
		}
		row := rows[id]
		if row == nil {
			row = &pb.DeviceExporter{Id: id}
			rows[id] = row
		}
		row.Version = addVersion(row.Version, version, row.Targets == 0)
		row.Targets++
		if target.Health == v1.HealthGood {
			row.Up++
		}
	})
	if err != nil {
		// An incomplete response cannot establish counts or absence. Keep the independent Pod inventory.
		return result, err
	}
	result.ExportersChecked = true
	for _, s := range exporterIdentities {
		if inspected && !uncertain[s.id] {
			result.CheckedKeys = append(result.CheckedKeys, s.id)
		}
		if row := rows[s.id]; row != nil {
			row.Vendor = s.vendor
			result.Exporters = append(result.Exporters, row)
		}
	}
	return result, nil
}

func matchesMLUEndpoint(target v1.ActiveTarget, ep biz.ExporterEndpoint) bool {
	l := target.DiscoveredLabels
	if l["__meta_kubernetes_pod_container_image"] != "" || l["__meta_kubernetes_pod_container_name"] != "" {
		return l["__meta_kubernetes_pod_container_name"] == ep.Container && l["__meta_kubernetes_pod_container_image"] == ep.Image
	}
	if l["__meta_kubernetes_endpoint_port_name"] != "http-metrics" && l["__meta_kubernetes_endpointslice_port_name"] != "http-metrics" {
		return false
	}
	_, foundPort, err := net.SplitHostPort(l["__address__"])
	return err == nil && foundPort == strconv.Itoa(int(ep.Port))
}

// All supported exporter deployments are DaemonSets. A target with a known
// different Pod controller cannot hide one, even without container metadata.
func couldHideExporter(target v1.ActiveTarget) bool {
	l := target.DiscoveredLabels
	if l["__meta_kubernetes_pod_uid"] != "" {
		kind := l["__meta_kubernetes_pod_controller_kind"]
		if kind != "" && kind != "DaemonSet" {
			return false
		}
		if kind == "DaemonSet" && l["__meta_kubernetes_pod_container_image"] != "" && l["__meta_kubernetes_pod_container_name"] != "" {
			return false
		}
	}
	return !systemScrape(target)
}

// These standard scrapes have a known non-device endpoint. Arbitrary static jobs remain unchecked.
func systemScrape(target v1.ActiveTarget) bool {
	l := target.DiscoveredLabels
	u, err := url.Parse(target.ScrapeURL)
	if err != nil {
		return false
	}
	isNode := l["__meta_kubernetes_node_name"] != "" || l["__meta_kubernetes_endpoint_address_target_kind"] == "Node" || l["__meta_kubernetes_endpointslice_address_target_kind"] == "Node"
	if isNode && l["__meta_kubernetes_pod_uid"] == "" && (u.Port() == "10250" || u.Port() == "10255") {
		return u.Path == "/metrics" || u.Path == "/metrics/cadvisor" || u.Path == "/metrics/resource" || u.Path == "/metrics/probes"
	}
	if l["__meta_kubernetes_service_name"] == "kubernetes" && l["__meta_kubernetes_namespace"] == "default" &&
		(l["__meta_kubernetes_endpoint_port_name"] == "https" || l["__meta_kubernetes_endpointslice_port_name"] == "https") && u.Path == "/metrics" {
		return true
	}
	if l["__meta_kubernetes_namespace"] == "kube-system" && u.Path == "/metrics" {
		if l["__meta_kubernetes_pod_controller_kind"] == "DaemonSet" && l["__meta_kubernetes_pod_controller_name"] == "kube-proxy" && l["__meta_kubernetes_pod_label_k8s_app"] == "kube-proxy" && u.Port() == "10249" {
			return true
		}
		// kube-prometheus-stack labels control-plane Services even when their Endpoints lack a targetRef.
		if l["__meta_kubernetes_pod_uid"] == "" {
			switch l["__meta_kubernetes_service_label_jobLabel"] {
			case "kube-etcd":
				return u.Port() == "2381"
			case "kube-controller-manager":
				return u.Port() == "10257"
			case "kube-scheduler":
				return u.Port() == "10259"
			}
		}
	}
	return l["job"] == "prometheus" && (u.Hostname() == "localhost" || net.ParseIP(u.Hostname()).IsLoopback()) && u.Port() == "9090" && u.Path == "/metrics"
}

// A candidate matches the image only. It establishes neither a scrape nor absence.
func exporterIdentity(target v1.ActiveTarget, mluEndpoints map[string]biz.ExporterEndpoint) (id, version, candidate string) {
	l := target.DiscoveredLabels
	if l["__meta_kubernetes_pod_uid"] == "" || l["__meta_kubernetes_pod_controller_kind"] != "DaemonSet" {
		return "", "", ""
	}
	container := l["__meta_kubernetes_pod_container_name"]
	repo, tag := biz.ImageReference(l["__meta_kubernetes_pod_container_image"])
	component := l["__meta_kubernetes_pod_label_app_kubernetes_io_component"]
	app := l["__meta_kubernetes_pod_label_app_kubernetes_io_name"]
	// The standalone Helm chart calls its container "exporter" and uses its configurable
	// chart name for both labels. GPU Operator manifests also use two label conventions.
	dcgmChart := container == "exporter" && app != "" && app == component
	dcgmOperator := (component == "dcgm-exporter" || component == "nvidia-dcgm-exporter") &&
		(container == "dcgm-exporter" || container == "nvidia-dcgm-exporter" || container == "nvidia-dcgm-exporter-dra")
	// GPU Operator v26.3.3 uses the same name for the DaemonSet, app label and container.
	dcgmOperatorApp := container == "nvidia-dcgm-exporter" && l["__meta_kubernetes_pod_label_app"] == container &&
		l["__meta_kubernetes_pod_controller_name"] == container
	ep, knownPod := mluEndpoints[l["__meta_kubernetes_pod_uid"]]
	matchesVerifiedMLU := knownPod && matchesMLUEndpoint(target, ep)
	verified := true
	switch {
	case matchesVerifiedMLU:
		id = "mlu-exporter"
		_, tag = biz.ImageReference(ep.Image)
	case strings.HasSuffix("/"+repo, "/nvidia/k8s/dcgm-exporter"):
		if !dcgmChart && !dcgmOperator && !dcgmOperatorApp {
			return "", "", "dcgm"
		}
		if m := dcgmTag.FindStringSubmatch(tag); m != nil {
			return "dcgm", m[1], ""
		}
		return "dcgm", "", ""
	case biz.MatchesImageRepository(repo, "npu-exporter"):
		id = "npu"
		verified = container == "npu-exporter" && l["__meta_kubernetes_pod_label_app"] == "npu-exporter"
	case biz.MatchesImageRepository(repo, "hcu/admin/base/hcu-exporter"):
		id = "hcu-exporter"
		verified = container == "hcu-exporter" && app == "hcu-exporter"
	case biz.MatchesImageRepository(repo, "dcu-exporter"):
		id = "dcu-exporter"
		verified = container == "dcu-exporter" && app == "dcu-exporter"
	case biz.MatchesImageRepository(repo, "mx-exporter"):
		id = "mx"
		verified = container == "mx-exporter" && app == "mx-exporter"
	case biz.MatchesImageRepository(repo, "cambricon-mlu-exporter"):
		id = "mlu-exporter"
		verified = container == "cambricon-mlu-monitor" && l["__meta_kubernetes_pod_label_app"] == "mlu-monitoring"
	case biz.MatchesImageRepository(repo, "rocm/device-metrics-exporter"):
		// The NIC collector ships in the same image as the GPU collector.
		if container == "amd-device-metrics-exporter-container" {
			return "", "", ""
		}
		id = "amd-metrics"
		verified = container == "amdgpu-metrics-exporter-container"
	case biz.MatchesImageRepository(repo, "k8s/ix-exporter"):
		id = "ix-exporter"
		verified = container != "" && container == app && container == l["__meta_kubernetes_pod_controller_name"]
	case biz.MatchesImageRepository(repo, "enflame/gcu-exporter"):
		id = "gcu-exporter"
		verified = container == "gcu-exporter"
	case biz.MatchesImageRepository(repo, "neuron/neuron-monitor"):
		id = "neuron-monitor"
		verified = container == "neuron-monitor"
	case biz.MatchesImageRepository(repo, "cce-plugin-pro/xpu-exporter"):
		id = "xpu-exporter"
		verified = container == "xpu-exporter" && l["__meta_kubernetes_pod_label_k8s_app"] == "cce-xpu-exporter"
	default:
		return "", "", ""
	}
	if !verified {
		return "", "", id
	}
	if !releaseTag.MatchString(tag) {
		tag = ""
	}
	return id, tag, ""
}
