package data

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/provider/util"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
)

const schedulingGPUIndex = "scheduling-gpu-candidate"

var defaultSchedulingResources = []string{
	"nvidia.com/gpu", "nvidia.com/gpucores", "nvidia.com/gpumem", "nvidia.com/gpumem-percentage",
}

func schedulingResources(config *conf.Scheduling) (map[corev1.ResourceName]struct{}, error) {
	names := config.GetResourceNames()
	if len(names) == 0 {
		names = defaultSchedulingResources
	}
	if len(names) > 64 {
		return nil, fmt.Errorf("scheduling.resource_names supports at most 64 resource names")
	}
	result := make(map[corev1.ResourceName]struct{}, len(names))
	for _, name := range names {
		if !strings.Contains(name, "/") || len(validation.IsQualifiedName(name)) != 0 {
			return nil, fmt.Errorf("scheduling.resource_names contains invalid extended resource name %q", name)
		}
		result[corev1.ResourceName(name)] = struct{}{}
	}
	return result, nil
}

func (r *podRepo) schedulingCandidate(pod *corev1.Pod) bool {
	// Also index Pods that already hold a HAMi assignment.
	if pod.Annotations[util.AssignedNodeAnnotations] != "" {
		return true
	}
	for _, containers := range [][]corev1.Container{pod.Spec.Containers, pod.Spec.InitContainers} {
		for _, ctr := range containers {
			for name := range r.schedulingResourceNames {
				if value, ok := ctr.Resources.Requests[name]; ok && value.Sign() > 0 {
					return true
				}
				if value, ok := ctr.Resources.Limits[name]; ok && value.Sign() > 0 {
					return true
				}
			}
		}
	}
	return false
}

func (r *podRepo) schedulingIndex(obj interface{}) ([]string, error) {
	pod, ok := obj.(*corev1.Pod)
	if ok && r.schedulingCandidate(pod) {
		return []string{"gpu"}, nil
	}
	return nil, nil
}

func schedulingStage(pod *corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "terminating"
	}
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return "finished"
	}
	// Proves assignment, not which scheduler made it.
	if pod.Spec.NodeName != "" {
		return "bound"
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionTrue {
			return "unknown"
		}
	}
	if pod.Status.Phase != "" && pod.Status.Phase != corev1.PodPending {
		return "unknown"
	}
	if len(pod.Spec.SchedulingGates) > 0 {
		return "gated"
	}
	return "waiting"
}

func (r *podRepo) ListSchedulingPods(ctx context.Context) ([]*biz.SchedulingPod, error) {
	objects, err := r.podIndexer.ByIndex(schedulingGPUIndex, "gpu")
	if err != nil {
		return nil, err
	}
	result := make([]*biz.SchedulingPod, 0)
	for _, obj := range objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pod := obj.(*corev1.Pod)
		// A finished Pod holds no allocation, as in the allocation list.
		if biz.IsPodInTerminatedState(pod) {
			continue
		}
		stage := schedulingStage(pod)
		if pod.Spec.NodeName != "" || stage == "waiting" || stage == "gated" || stage == "unknown" {
			result = append(result, r.schedulingPodView(pod, false))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.UID < b.UID
	})
	return result, nil
}

func (r *podRepo) checkedSchedulingPod(namespace, name, uid string) (*corev1.Pod, error) {
	if namespace == "" || name == "" || uid == "" {
		return nil, kratoserrors.BadRequest("POD_IDENTITY_REQUIRED", "namespace, name and uid are required")
	}
	pod, err := r.podLister.Pods(namespace).Get(name)
	if apierrors.IsNotFound(err) {
		return nil, kratoserrors.NotFound("POD_NOT_FOUND", "The selected Pod is no longer available")
	}
	if err != nil {
		return nil, err
	}
	if string(pod.UID) != uid {
		return nil, kratoserrors.Conflict("POD_RECREATED", "A different Pod now uses this name")
	}
	if !r.schedulingCandidate(pod) {
		return nil, kratoserrors.NotFound("GPU_POD_NOT_FOUND", "The selected Pod does not request a configured GPU resource")
	}
	return pod, nil
}

func (r *podRepo) GetSchedulingPod(ctx context.Context, namespace, name, uid string) (*biz.SchedulingDetail, error) {
	if _, err := r.checkedSchedulingPod(namespace, name, uid); err != nil {
		return nil, err
	}
	events, err := r.schedulingEvents.get(ctx, namespace, uid)
	if err != nil {
		return nil, err
	}
	// Recheck the Pod after the slow event read.
	pod, err := r.checkedSchedulingPod(namespace, name, uid)
	if err != nil {
		return nil, err
	}
	return &biz.SchedulingDetail{Pod: r.schedulingPod(pod), Events: events}, nil
}

func (r *podRepo) schedulingPod(pod *corev1.Pod) *biz.SchedulingPod {
	return r.schedulingPodView(pod, true)
}

func (r *podRepo) schedulingPodView(pod *corev1.Pod, detail bool) *biz.SchedulingPod {
	result := &biz.SchedulingPod{
		Name: pod.Name, Namespace: pod.Namespace, UID: string(pod.UID),
		CreatedAt: pod.CreationTimestamp.Time, SchedulerName: pod.Spec.SchedulerName,
		NodeName: pod.Spec.NodeName, Stage: schedulingStage(pod), ReasonSource: "unknown",
		Preallocated: pod.Annotations[util.AssignedNodeAnnotations] != "",
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled {
			message, truncated := boundedSchedulingText(condition.Message, 16*1024)
			result.Condition = &biz.SchedulingCondition{Status: string(condition.Status), Reason: condition.Reason,
				Message: message, TransitionAt: condition.LastTransitionTime.Time, MessageTruncated: truncated}
			if result.Stage == "waiting" && condition.Status == corev1.ConditionFalse {
				result.ReasonCodes, result.ReasonSource = schedulingReasons(condition.Message, r.schedulingResourceNames)
			}
			break
		}
	}
	for _, gate := range pod.Spec.SchedulingGates {
		result.Gates = append(result.Gates, gate.Name)
	}
	if result.Stage == "gated" {
		result.ReasonCodes, result.ReasonSource = []string{"SchedulingGated"}, "kubernetes"
	} else if result.Stage == "waiting" && len(result.ReasonCodes) == 0 {
		result.ReasonCodes = []string{"NoFeedback"}
	}
	for _, ctr := range pod.Spec.Containers {
		if request := r.schedulingContainerRequest(ctr, "regular"); len(request.Resources) > 0 {
			request.Status, request.StatusDetail = schedulingContainerStatus(pod, ctr, "regular")
			result.Requests = append(result.Requests, request)
		}
	}
	for _, ctr := range pod.Spec.InitContainers {
		kind := "init"
		if ctr.RestartPolicy != nil && *ctr.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			kind = "sidecar"
		}
		if request := r.schedulingContainerRequest(ctr, kind); len(request.Resources) > 0 {
			request.Status, request.StatusDetail = schedulingContainerStatus(pod, ctr, kind)
			result.Requests = append(result.Requests, request)
		}
	}
	if detail {
		result.Constraints = schedulingConstraints(pod)
	}
	if result.NodeName != "" {
		r.mutex.RLock()
		if allocated, ok := r.pods[pod.UID]; ok {
			for _, ctr := range allocated.Ctrs {
				if len(ctr.ContainerDevices) > 0 {
					result.AllocatedContainers = append(result.AllocatedContainers, ctr.Name)
				}
			}
		}
		r.mutex.RUnlock()
	}
	return result
}

func schedulingContainerStatus(pod *corev1.Pod, ctr corev1.Container, kind string) (string, *biz.ContainerStatusDetail) {
	statuses := pod.Status.ContainerStatuses
	if kind != "regular" {
		statuses = pod.Status.InitContainerStatuses
	}
	var observed *corev1.ContainerStatus
	for i := range statuses {
		if statuses[i].Name == ctr.Name {
			observed = &statuses[i]
			break
		}
	}
	if kind == "regular" {
		return classifyContainerStatus(pod, observed)
	}
	statePod := *pod
	if kind == "sidecar" {
		statePod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	} else if observed != nil && observed.State.Terminated != nil && observed.State.Terminated.ExitCode == 0 {
		// A completed ordinary init container is not restarted by Pod Always.
		statePod.Spec.RestartPolicy = corev1.RestartPolicyNever
	}
	status, detail := classifyContainerStatus(&statePod, observed)
	if kind == "init" && status == biz.ContainerStatusNotReady && observed != nil && observed.State.Running != nil {
		// Init containers have no readiness probe; not Ready while running is normal.
		status, detail.Ready = biz.ContainerStatusSuccess, nil
	}
	return status, detail
}

func (r *podRepo) schedulingContainerRequest(ctr corev1.Container, kind string) biz.SchedulingContainerRequest {
	result := biz.SchedulingContainerRequest{Container: ctr.Name, ContainerKind: kind}
	for name := range r.schedulingResourceNames {
		quantity, ok := ctr.Resources.Requests[name]
		if !ok {
			quantity, ok = ctr.Resources.Limits[name]
		}
		if !ok {
			continue
		}
		result.Resources = append(result.Resources, schedulingResource(name, quantity))
	}
	sort.Slice(result.Resources, func(i, j int) bool { return result.Resources[i].Name < result.Resources[j].Name })
	return result
}

func schedulingResource(name corev1.ResourceName, quantity resource.Quantity) biz.SchedulingResource {
	result := biz.SchedulingResource{Name: string(name), Value: quantity.String(), Kind: "raw"}
	switch name {
	case "nvidia.com/gpu":
		result.Kind = "count"
	case "nvidia.com/gpucores":
		result.Kind, result.Unit = "core", "%"
	case "nvidia.com/gpumem":
		result.Kind, result.Unit = "memory", "MiB"
	case "nvidia.com/gpumem-percentage":
		result.Kind, result.Unit = "memory_percentage", "%"
	}
	if result.Kind != "raw" {
		result.Value = quantity.AsDec().String()
	}
	return result
}

func schedulingConstraints(pod *corev1.Pod) []biz.SchedulingConstraint {
	result := make([]biz.SchedulingConstraint, 0)
	for key, value := range pod.Spec.NodeSelector {
		result = append(result, biz.SchedulingConstraint{Name: "nodeSelector." + key, Value: value})
	}
	for _, key := range []string{"nvidia.com/use-gputype", "nvidia.com/nouse-gputype", "nvidia.com/use-gpuuuid", "nvidia.com/nouse-gpuuuid", "hami.io/node-scheduler-policy", "hami.io/gpu-scheduler-policy", "nvidia.com/numa-bind"} {
		if value := pod.Annotations[key]; value != "" {
			result = append(result, biz.SchedulingConstraint{Name: key, Value: value})
		}
	}
	appendJSON := func(name string, value interface{}) {
		if data, err := json.Marshal(value); err == nil {
			result = append(result, biz.SchedulingConstraint{Name: name, Value: string(data)})
		}
	}
	if pod.Spec.Affinity != nil {
		appendJSON("affinity", pod.Spec.Affinity)
	}
	if len(pod.Spec.Tolerations) > 0 {
		appendJSON("tolerations", pod.Spec.Tolerations)
	}
	if len(pod.Spec.TopologySpreadConstraints) > 0 {
		appendJSON("topologySpreadConstraints", pod.Spec.TopologySpreadConstraints)
	}
	if pod.Spec.PriorityClassName != "" {
		result = append(result, biz.SchedulingConstraint{Name: "priorityClassName", Value: pod.Spec.PriorityClassName})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

var hamiSchedulingReasons = []string{
	"CardInsufficientMemory", "CardInsufficientCore", "CardComputeUnitsExhausted", "CardTimeSlicingExhausted",
	"NodeInsufficientDevice", "AllocatedCardsInsufficientRequest", "CardTypeMismatch", "CardUuidMismatch",
	"CardNotHealth", "CardCordoned", "ExclusiveDeviceAllocateConflict", "NumaNotFit", "CardMigTopologyInfeasible", "ResourceQuotaNotFit",
	"CardNotFoundCustomFilterRule", "ModeNotFit",
}

var schedulingToken = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*`)

var insufficientResource = regexp.MustCompile(`(?i)\binsufficient\s+([a-z0-9][a-z0-9_./-]*)`)
var missingPVC = regexp.MustCompile(`(?i)\bpersistentvolumeclaims?\s+["'][^"'\r\n]+["']\s+not\s+found\b`)

func schedulingReasons(message string, resourceNames map[corev1.ResourceName]struct{}) ([]string, string) {
	if strings.TrimSpace(message) == "" {
		return nil, "unknown"
	}
	tokens := make(map[string]bool)
	for _, token := range schedulingToken.FindAllString(insufficientResource.ReplaceAllString(message, ""), -1) {
		tokens[token] = true
	}
	result := make([]string, 0)
	hami, kube := false, false
	for _, code := range hamiSchedulingReasons {
		if tokens[code] {
			result = append(result, code)
			hami = true
		}
	}
	insufficient := make(map[string]bool)
	for _, match := range insufficientResource.FindAllStringSubmatch(message, -1) {
		name := strings.TrimRight(match[1], ".")
		switch strings.ToLower(name) {
		case "cpu":
			insufficient["InsufficientCPU"] = true
		case "memory":
			insufficient["InsufficientHostMemory"] = true
		default:
			if _, configured := resourceNames[corev1.ResourceName(name)]; configured {
				insufficient["ExtendedResourceUnavailable"] = true
			}
		}
	}
	for _, code := range []string{"InsufficientCPU", "InsufficientHostMemory", "ExtendedResourceUnavailable"} {
		if insufficient[code] {
			result = append(result, code)
			kube = true
		}
	}
	if missingPVC.MatchString(message) {
		result = append(result, "PVCNotFound")
		kube = true
	}
	lower := strings.ToLower(message)
	matched := make(map[string]bool)
	for _, item := range []struct{ phrase, code string }{
		{"untolerated taint", "UntoleratedTaint"}, {"didn't match pod's node affinity/selector", "NodeAffinity"},
		{"unbound immediate persistentvolumeclaims", "UnboundPVC"}, {"volume node affinity conflict", "VolumeNodeAffinity"},
		{"were unschedulable", "NodeUnschedulable"},
		{"didn't match pod affinity rules", "PodAffinity"}, {"didn't match pod anti-affinity rules", "PodAffinity"},
		{"didn't satisfy existing pods anti-affinity rules", "PodAffinity"},
		{"too many pods", "TooManyPods"}, {"didn't have free ports", "HostPortConflict"},
	} {
		if strings.Contains(lower, item.phrase) && !matched[item.code] {
			matched[item.code] = true
			result = append(result, item.code)
			kube = true
		}
	}
	if hami && kube {
		return result, "mixed"
	}
	if hami {
		return result, "hami"
	}
	if kube {
		return result, "kubernetes"
	}
	return []string{"UnknownSchedulingReason"}, "unknown"
}

func boundedSchedulingText(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		return value, false
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return strings.Clone(value[:maxBytes]), true
}

var _ biz.SchedulingRepo = (*podRepo)(nil)
