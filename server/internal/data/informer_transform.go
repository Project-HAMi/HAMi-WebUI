package data

import (
	"maps"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// trimInformerObject returns an independent, field-whitelisted copy of Node and
// Pod objects before they enter the shared informer indexer. Never mutate the
// input object in place (client-go v0.30.1 Resync replays cached items through
// the transformer), never clear ResourceVersion, and never return a non-nil
// error (which drops objects from the informer cache).
func trimInformerObject(obj interface{}) (interface{}, error) {
	switch v := obj.(type) {
	case cache.DeletedFinalStateUnknown:
		return v, nil
	case *corev1.Pod:
		return trimInformerPod(v), nil
	case *corev1.Node:
		return trimInformerNode(v), nil
	default:
		return obj, nil
	}
}

func trimInformerPod(pod *corev1.Pod) *corev1.Pod {
	if pod == nil {
		return nil
	}
	return &corev1.Pod{
		TypeMeta:   pod.TypeMeta,
		ObjectMeta: trimObjectMeta(pod.ObjectMeta, true),
		Spec: corev1.PodSpec{
			NodeName:                  pod.Spec.NodeName,
			RestartPolicy:             pod.Spec.RestartPolicy,
			SchedulerName:             pod.Spec.SchedulerName,
			PriorityClassName:         pod.Spec.PriorityClassName,
			SchedulingGates:           slices.Clone(pod.Spec.SchedulingGates),
			NodeSelector:              maps.Clone(pod.Spec.NodeSelector),
			Affinity:                  pod.Spec.Affinity.DeepCopy(),
			Tolerations:               cloneTolerations(pod.Spec.Tolerations),
			TopologySpreadConstraints: cloneTopologySpreadConstraints(pod.Spec.TopologySpreadConstraints),
			ResourceClaims:            clonePodResourceClaims(pod.Spec.ResourceClaims),
			InitContainers:            trimContainers(pod.Spec.InitContainers),
			Containers:                trimContainers(pod.Spec.Containers),
			EphemeralContainers:       trimEphemeralContainers(pod.Spec.EphemeralContainers),
		},
		Status: corev1.PodStatus{
			Phase:                     pod.Status.Phase,
			Reason:                    pod.Status.Reason,
			Message:                   pod.Status.Message,
			PodIP:                     pod.Status.PodIP,
			PodIPs:                    slices.Clone(pod.Status.PodIPs),
			StartTime:                 pod.Status.StartTime.DeepCopy(),
			Conditions:                clonePodConditions(pod.Status.Conditions),
			ResourceClaimStatuses:     clonePodResourceClaimStatuses(pod.Status.ResourceClaimStatuses),
			ContainerStatuses:         trimContainerStatuses(pod.Status.ContainerStatuses),
			InitContainerStatuses:     trimContainerStatuses(pod.Status.InitContainerStatuses),
			EphemeralContainerStatuses: trimContainerStatuses(pod.Status.EphemeralContainerStatuses),
		},
	}
}

func trimInformerNode(node *corev1.Node) *corev1.Node {
	if node == nil {
		return nil
	}
	return &corev1.Node{
		TypeMeta:   node.TypeMeta,
		ObjectMeta: trimObjectMeta(node.ObjectMeta, false),
		Spec: corev1.NodeSpec{
			Unschedulable: node.Spec.Unschedulable,
			Taints:        cloneTaints(node.Spec.Taints),
		},
		Status: corev1.NodeStatus{
			NodeInfo:    node.Status.NodeInfo,
			Addresses:   slices.Clone(node.Status.Addresses),
			Conditions:  cloneNodeConditions(node.Status.Conditions),
			Capacity:    cloneResourceList(node.Status.Capacity),
			Allocatable: cloneResourceList(node.Status.Allocatable),
		},
	}
}

func trimObjectMeta(meta metav1.ObjectMeta, keepOwners bool) metav1.ObjectMeta {
	annotations := maps.Clone(meta.Annotations)
	delete(annotations, lastAppliedConfigAnnotation)
	var owners []metav1.OwnerReference
	if keepOwners {
		owners = cloneOwnerReferences(meta.OwnerReferences)
	}
	return metav1.ObjectMeta{
		Name:              meta.Name,
		Namespace:         meta.Namespace,
		UID:               meta.UID,
		ResourceVersion:   meta.ResourceVersion,
		Generation:        meta.Generation,
		CreationTimestamp: meta.CreationTimestamp,
		DeletionTimestamp: meta.DeletionTimestamp.DeepCopy(),
		OwnerReferences:   owners,
		Labels:            maps.Clone(meta.Labels),
		Annotations:       annotations,
	}
}

func trimContainers(containers []corev1.Container) []corev1.Container {
	if containers == nil {
		return nil
	}
	out := make([]corev1.Container, len(containers))
	for i := range containers {
		out[i] = trimContainer(containers[i])
	}
	return out
}

func trimEphemeralContainers(containers []corev1.EphemeralContainer) []corev1.EphemeralContainer {
	if containers == nil {
		return nil
	}
	out := make([]corev1.EphemeralContainer, len(containers))
	for i := range containers {
		ec := containers[i]
		ec.EphemeralContainerCommon = corev1.EphemeralContainerCommon(trimContainer(corev1.Container(ec.EphemeralContainerCommon)))
		out[i] = ec
	}
	return out
}

func trimContainer(c corev1.Container) corev1.Container {
	var env []corev1.EnvVar
	if len(c.Env) > 0 {
		env = make([]corev1.EnvVar, 0, len(c.Env))
		for _, item := range c.Env {
			env = append(env, corev1.EnvVar{Name: item.Name, Value: item.Value})
		}
	}
	var readiness *corev1.Probe
	if c.ReadinessProbe != nil && c.ReadinessProbe.HTTPGet != nil {
		readiness = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: c.ReadinessProbe.HTTPGet.DeepCopy(),
			},
		}
	}
	return corev1.Container{
		Name:           c.Name,
		Image:          c.Image,
		Command:        slices.Clone(c.Command),
		Args:           slices.Clone(c.Args),
		Env:            env,
		Resources:      cloneResourceRequirements(c.Resources),
		ReadinessProbe: readiness,
		RestartPolicy:  cloneRestartPolicy(c.RestartPolicy),
	}
}

func trimContainerStatuses(statuses []corev1.ContainerStatus) []corev1.ContainerStatus {
	if statuses == nil {
		return nil
	}
	out := make([]corev1.ContainerStatus, len(statuses))
	for i := range statuses {
		s := statuses[i]
		out[i] = corev1.ContainerStatus{
			Name:                 s.Name,
			ContainerID:          s.ContainerID,
			Ready:                s.Ready,
			RestartCount:         s.RestartCount,
			Image:                s.Image,
			ImageID:              s.ImageID,
			State:                *s.State.DeepCopy(),
			LastTerminationState: *s.LastTerminationState.DeepCopy(),
		}
	}
	return out
}

func cloneResourceRequirements(req corev1.ResourceRequirements) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: cloneResourceList(req.Requests),
		Limits:   cloneResourceList(req.Limits),
		Claims:   slices.Clone(req.Claims),
	}
}

func cloneResourceList(list corev1.ResourceList) corev1.ResourceList {
	if list == nil {
		return nil
	}
	out := make(corev1.ResourceList, len(list))
	for k, v := range list {
		out[k] = v.DeepCopy()
	}
	return out
}

func cloneRestartPolicy(p *corev1.ContainerRestartPolicy) *corev1.ContainerRestartPolicy {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneOwnerReferences(owners []metav1.OwnerReference) []metav1.OwnerReference {
	if owners == nil {
		return nil
	}
	out := make([]metav1.OwnerReference, len(owners))
	for i := range owners {
		out[i] = *owners[i].DeepCopy()
	}
	return out
}

func cloneTolerations(items []corev1.Toleration) []corev1.Toleration {
	if items == nil {
		return nil
	}
	out := make([]corev1.Toleration, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func cloneTopologySpreadConstraints(items []corev1.TopologySpreadConstraint) []corev1.TopologySpreadConstraint {
	if items == nil {
		return nil
	}
	out := make([]corev1.TopologySpreadConstraint, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func clonePodResourceClaims(items []corev1.PodResourceClaim) []corev1.PodResourceClaim {
	if items == nil {
		return nil
	}
	out := make([]corev1.PodResourceClaim, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func clonePodResourceClaimStatuses(items []corev1.PodResourceClaimStatus) []corev1.PodResourceClaimStatus {
	if items == nil {
		return nil
	}
	out := make([]corev1.PodResourceClaimStatus, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func clonePodConditions(items []corev1.PodCondition) []corev1.PodCondition {
	if items == nil {
		return nil
	}
	out := make([]corev1.PodCondition, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func cloneNodeConditions(items []corev1.NodeCondition) []corev1.NodeCondition {
	if items == nil {
		return nil
	}
	out := make([]corev1.NodeCondition, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}

func cloneTaints(items []corev1.Taint) []corev1.Taint {
	if items == nil {
		return nil
	}
	out := make([]corev1.Taint, len(items))
	for i := range items {
		out[i] = *items[i].DeepCopy()
	}
	return out
}
