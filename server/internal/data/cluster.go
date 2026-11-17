package data

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"slices"
	"strings"

	"vgpu/internal/biz"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/version"
)

const hamiComponentLabel = "app.kubernetes.io/component"

type clusterRepo struct {
	data *Data
	pods *podRepo
}

func NewClusterRepo(data *Data, pods *podRepo) biz.ClusterRepo {
	return &clusterRepo{data: data, pods: pods}
}

// ServerVersion reads /version itself because Discovery().ServerVersion ignores the context.
func (r *clusterRepo) ServerVersion(ctx context.Context) (string, error) {
	client := r.data.k8sCl.Discovery().RESTClient()
	if client == nil {
		return "", errors.New("discovery client has no REST client")
	}
	body, err := client.Get().AbsPath("/version").Do(ctx).Raw()
	if err != nil {
		return "", err
	}
	var info version.Info
	if err := json.Unmarshal(body, &info); err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

// Components reads the shared informer once, including vendor DaemonSets outside the HAMi chart.
func (r *clusterRepo) Components(context.Context) (biz.ClusterComponents, error) {
	result := biz.ClusterComponents{MLUEndpoints: make(map[string]biz.ExporterEndpoint)}
	pods, err := r.pods.podLister.List(labels.Everything())
	if err != nil {
		return result, err
	}
	type key struct{ name, namespace string }
	groups := map[key]*biz.HAMiComponent{}
	nodes := map[key]map[string]struct{}{}
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || biz.IsPodInTerminatedState(pod) {
			continue
		}
		if endpoint, ok := mluMetricsEndpoint(pod); ok {
			result.MLUEndpoints[string(pod.UID)] = endpoint
		}
		role := pod.Labels[hamiComponentLabel]
		pluginID, image := devicePluginIdentity(pod, role)
		if pluginID != "" {
			result.Plugins = append(result.Plugins, biz.DevicePluginPod{ID: pluginID, Image: image, Ready: podReady(pod)})
		}
		if role != biz.HAMiScheduler && role != biz.HAMiDevicePlugin {
			continue
		}
		id := key{pod.Labels[hamiComponentLabel], pod.Namespace}
		group, ok := groups[id]
		if !ok {
			group = &biz.HAMiComponent{Name: id.name, Namespace: id.namespace}
			groups[id] = group
			nodes[id] = map[string]struct{}{}
		}
		group.Pods++
		member := biz.HAMiPod{Ready: podReady(pod), UID: string(pod.UID), Name: pod.Name, Namespace: pod.Namespace}
		for _, ip := range pod.Status.PodIPs {
			member.IPs = append(member.IPs, ip.IP)
		}
		group.Members = append(group.Members, member)
		if member.Ready {
			group.Ready++
		}
		if pod.Spec.NodeName != "" {
			nodes[id][pod.Spec.NodeName] = struct{}{}
		}
		for _, status := range pod.Status.ContainerStatuses {
			group.Restarts += status.RestartCount
		}
	}
	components := make([]*biz.HAMiComponent, 0, len(groups))
	for id, group := range groups {
		group.Nodes = int32(len(nodes[id]))
		components = append(components, group)
	}
	slices.SortFunc(components, func(a, b *biz.HAMiComponent) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Namespace, b.Namespace)
	})
	result.HAMi = components
	return result, nil
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// The official Cambricon ServiceMonitor scrapes the same numeric port as /healthz.
func mluMetricsEndpoint(pod *corev1.Pod) (biz.ExporterEndpoint, bool) {
	for _, c := range pod.Spec.Containers {
		if len(c.Command) == 0 || c.Command[0] != "/usr/bin/mlu-exporter" || c.ReadinessProbe == nil {
			continue
		}
		probe := c.ReadinessProbe.HTTPGet
		repo, _ := biz.ImageReference(c.Image)
		owner := metav1.GetControllerOfNoCopy(pod)
		if probe != nil && probe.Path == "/healthz" && probe.Port.IntVal > 0 && owner != nil && owner.Kind == "DaemonSet" &&
			biz.MatchesImageRepository(repo, "cambricon-mlu-exporter") {
			return biz.ExporterEndpoint{Container: c.Name, Port: probe.Port.IntVal, Image: c.Image}, true
		}
	}
	return biz.ExporterEndpoint{}, false
}

func hcuHAMiRegistration(c corev1.Container) bool {
	// The upstream --strategy option overrides RESOURCE_REGISTER_STRATEGY. Its default is mixed.
	for i, arg := range c.Args {
		if arg == "--strategy" && i+1 < len(c.Args) {
			return c.Args[i+1] == "hami"
		}
		if value, ok := strings.CutPrefix(arg, "--strategy="); ok {
			return value == "hami"
		}
	}
	for _, env := range c.Env {
		if env.Name == "RESOURCE_REGISTER_STRATEGY" {
			return env.Value == "hami"
		}
	}
	return false
}

func devicePluginIdentity(pod *corev1.Pod, role string) (string, string) {
	owner := metav1.GetControllerOfNoCopy(pod)
	daemonSet := owner != nil && owner.Kind == "DaemonSet"
	// Cambricon also publishes a single-Pod manifest. Kubelet adds the node name to its mirror Pod.
	const mluStatic = "cambricon-device-plugin-static-pod"
	standaloneMLU := (owner == nil && pod.Name == mluStatic) || (pod.Annotations[corev1.MirrorPodAnnotationKey] != "" && pod.Name == mluStatic+"-"+pod.Spec.NodeName)
	if role != biz.HAMiDevicePlugin && !daemonSet && !standaloneMLU {
		return "", ""
	}
	for _, c := range pod.Spec.Containers {
		if role == biz.HAMiDevicePlugin && c.Name == "device-plugin" && len(c.Command) > 0 && path.Base(c.Command[0]) == "nvidia-device-plugin" {
			return "nvidia-hami", c.Image
		}
		repo, _ := biz.ImageReference(c.Image)
		if (standaloneMLU || daemonSet) && c.Name == "cambricon-device-plugin-ctr" && biz.MatchesImageRepository(repo, "cambricon-device-plugin") && len(c.Command) > 0 && c.Command[0] == "/usr/bin/k8s-device-plugin" {
			return "mlu-plugin", c.Image
		}
		if !daemonSet {
			continue
		}
		switch {
		case role == "hami-ascend-device-plugin" && c.Name == "device-plugin" && biz.MatchesImageRepository(repo, "projecthami/ascend-device-plugin"):
			return "ascend-hami", c.Image
		case c.Name == "hcu-dp-cntr" && hcuHAMiRegistration(c) && biz.MatchesImageRepository(repo, "hcu/admin/base/hcu-device-plugin"):
			return "hcu-plugin", c.Image
		case c.Name == "dcu-dp-cntr" && biz.MatchesImageRepository(repo, "projecthami/dcu-vgpu-device-plugin"):
			return "legacy-dcu", c.Image
		case owner.Name == "metax-gpu-device" && c.Name == "gpu-device" && biz.MatchesImageRepository(repo, "gpu-device"):
			return "metax-native", c.Image
		case c.Name == "amd-gpu-dp-cntr" && biz.MatchesImageRepository(repo, "project-hami/amd-device-plugin"):
			return "amd-hami", c.Image
		case c.Name == "gcushare" && biz.MatchesImageRepository(repo, "enflame/gcushare-device-plugin"):
			return "gcu-plugin", c.Image
		case c.Name == "device-plugin" && biz.MatchesImageRepository(repo, "projecthami/vxpu-device-plugin") && role == "vxpu-device-plugin":
			return "vxpu", c.Image
		case c.Name == "vastai-device-plugin-dp" && biz.MatchesImageRepository(repo, "projecthami/vastai-device-plugin"):
			return "vastai-plugin", c.Image
		case c.Name == "neuron-device-plugin" && biz.MatchesImageRepository(repo, "neuron/neuron-device-plugin"):
			return "neuron", c.Image
		case c.Name == "k8s-device-plugin" && biz.MatchesImageRepository(repo, "projecthami/biren-device-plugin"):
			return "biren-plugin", c.Image
		}
	}
	if role == biz.HAMiDevicePlugin {
		return "hami-device-plugin", ""
	}
	return "", ""
}
