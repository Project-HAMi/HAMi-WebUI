package podfacts

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

// Extract returns detached facts for every declared container, including Pods
// with no accelerator evidence and terminal Pods. Callers own inventory coverage.
func Extract(pod *corev1.Pod, evidence Evidence, options Options) (*Facts, error) {
	if pod == nil || pod.UID == "" || pod.Name == "" || pod.Namespace == "" {
		return nil, errors.New("pod facts require namespace, name and UID")
	}
	result := &Facts{
		UnreadableAllocations: slices.Clone(evidence.UnreadableAllocations),
		Metadata: metav1.ObjectMeta{
			Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID,
			ResourceVersion: pod.ResourceVersion, DeletionTimestamp: pod.DeletionTimestamp.DeepCopy(),
		},
		NodeName:  pod.Spec.NodeName,
		Terminal:  pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed,
		Candidate: NoAcceleratorEvidence,
	}
	if assigned := pod.Annotations["hami.io/vgpu-node"]; assigned != "" {
		result.Metadata.Annotations = map[string]string{"hami.io/vgpu-node": assigned}
	}
	names := map[string]bool{}
	appendContainer := func(spec corev1.Container, kind ContainerKind, index int) error {
		if spec.Name == "" || names[spec.Name] {
			return fmt.Errorf("empty or duplicate container name %q", spec.Name)
		}
		names[spec.Name] = true
		ctr := Container{
			Key:  ContainerKey{PodUID: pod.UID, Name: spec.Name},
			Kind: kind, Index: index, States: map[string]*viewv1.FieldState{},
		}
		ctr.States["allocations"] = unknown("", "container") // no evidence is not an empty allocation inventory
		if kind == Ephemeral {
			ctr.States["resources"] = &viewv1.FieldState{State: resourceview.NotApplicable, Reason: "unsupported", Source: "pod_spec", Scope: "container"}
			if len(spec.Resources.Requests)+len(spec.Resources.Limits)+len(spec.Resources.Claims) > 0 {
				result.Issues = append(result.Issues, Issue{"spec.ephemeralContainers." + spec.Name + ".resources", "ephemeral_resources_not_supported"})
			}
		} else {
			ctr.States["resources"] = known("pod_spec", "container")
			ctr.Requests = resourceRequests(spec.Resources, options)
			ctr.Claims = claimReferences(pod, spec)
			for _, request := range ctr.Requests {
				result.consider(request)
			}
			if len(ctr.Claims) > 0 {
				result.unresolved("dra_claim_reference")
			}
		}
		result.Containers = append(result.Containers, ctr)
		return nil
	}
	for i, spec := range pod.Spec.InitContainers {
		kind := Init
		if spec.RestartPolicy != nil && *spec.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			kind = NativeSidecar
		}
		if err := appendContainer(spec, kind, i); err != nil {
			return nil, err
		}
	}
	for i, spec := range pod.Spec.Containers {
		if err := appendContainer(spec, Regular, i); err != nil {
			return nil, err
		}
	}
	for i, spec := range pod.Spec.EphemeralContainers {
		if err := appendContainer(corev1.Container(spec.EphemeralContainerCommon), Ephemeral, i); err != nil {
			return nil, err
		}
	}
	result.readAnnotations(pod)
	result.readAllocations(pod, evidence.Allocations, options)
	if len(pod.Spec.InitContainers) > 0 {
		// readAllocations attaches the same source completeness to each member.
		result.hasInitReservation = hasDecodedReservation(result.Containers[0].Allocations)
		result.InitUsageReleased = result.hasInitReservation && allOrdinaryInitSucceeded(pod)
	}
	slices.Sort(result.CandidateEvidence)
	result.CandidateEvidence = slices.Compact(result.CandidateEvidence)
	return result, nil
}

// A failed decode cannot replace the scheduler's cached reservation.
func hasDecodedReservation(sources []AllocationSet) bool {
	hasSource := false
	for _, set := range sources {
		if set.Accounting != AccountingPodPeak {
			continue
		}
		if !set.Complete || set.Source.State != resourceview.Known {
			return false
		}
		hasSource = true
	}
	return hasSource
}

// Match HAMi v2.11 AllNonSidecarInitContainersSucceeded. The current status
// must show success for every declared ordinary init; neither the Pod phase
// nor an earlier terminated attempt establishes that the init phase is over.
func allOrdinaryInitSucceeded(pod *corev1.Pod) bool {
	succeeded := make(map[string]bool, len(pod.Status.InitContainerStatuses))
	for _, status := range pod.Status.InitContainerStatuses {
		succeeded[status.Name] = status.State.Terminated != nil && status.State.Terminated.ExitCode == 0
	}
	for _, spec := range pod.Spec.InitContainers {
		if spec.RestartPolicy != nil && *spec.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			continue
		}
		if !succeeded[spec.Name] {
			return false
		}
	}
	return true
}

func known(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Known, Source: source, Scope: scope}
}
func unknown(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Unknown, Reason: "unverified", Source: source, Scope: scope}
}
func (f *Facts) unresolved(reason string) {
	if f.Candidate == NoAcceleratorEvidence {
		f.Candidate = CandidateUnresolved
	}
	f.CandidateEvidence = append(f.CandidateEvidence, reason)
}
func (f *Facts) consider(request Request) {
	if request.State.State != resourceview.Known {
		f.unresolved(request.Source + ":" + request.Field)
		return
	}
	if request.Positive {
		f.Candidate = CandidateAccelerator
		f.CandidateEvidence = append(f.CandidateEvidence, request.Source+":"+request.Field+":"+request.Resource)
	}
}
func recognizes(name corev1.ResourceName, options Options) bool {
	if slices.Contains(options.ResourceNames, name) {
		return true
	}
	switch name {
	case "nvidia.com/gpu", "nvidia.com/gpucores", "nvidia.com/gpumem", "nvidia.com/gpumem-percentage", "amd.com/gpu":
		return true
	}
	value := string(name)
	return (strings.HasPrefix(value, "nvidia.com/mig-") && len(value) > len("nvidia.com/mig-")) || (strings.HasPrefix(value, "amd.com/gpu-") && len(value) > len("amd.com/gpu-"))
}
func cloneState(s *viewv1.FieldState) *viewv1.FieldState {
	if s == nil {
		return nil
	}
	return proto.Clone(s).(*viewv1.FieldState)
}
func cloneSource(s *viewv1.SourceState) *viewv1.SourceState {
	if s == nil {
		return &viewv1.SourceState{Source: "unidentified", State: resourceview.Unknown, Reason: "unverified"}
	}
	return proto.Clone(s).(*viewv1.SourceState)
}
