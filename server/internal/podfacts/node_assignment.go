package podfacts

import (
	"slices"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NodeAssignment keeps the node identity and binding evidence seen for one Pod UID.
type NodeAssignment struct {
	UID             types.UID
	nodeBirth       time.Time
	boundBy         time.Time
	hasScheduleTime bool
	// A reservation can be replaced until Kubernetes binds the Pod. HAMi's
	// Filter writes the chosen node, allocation and Unix second together.
	reservedNode string
	reservedAt   int64
}

// Observe returns the accumulated evidence and whether this allocation conflicts
// with the observed node. Pod creation alone says nothing about binding time.
func (a NodeAssignment) Observe(pod *corev1.Pod, node *corev1.Node, allocations []AllocationSnapshot) (NodeAssignment, bool) {
	bound := pod.Spec.NodeName != ""
	reserved := hasReservation(allocations)
	if bound {
		a.observeBindingTime(pod)
	} else if !reserved {
		// A nomination alone doesn't reserve a device. A withdrawal also
		// ends the previous reservation while the Pod is still unbound.
		return NodeAssignment{}, false
	}
	if node == nil {
		return a, false
	}
	var second int64
	if reserved {
		second = filterTime(pod)
	}
	if a.UID != "" && a.UID != node.UID && (!reserved || !a.reservationOnReplacement(pod, node, second)) {
		return a, true
	}
	// Retain a documented stale Filter decision if a later Pod status
	// loses its timestamp without issuing a new reservation.
	if second == 0 && reserved && a.reservedNode == node.Name {
		second = a.reservedAt
	}
	if a.conflictsWithLifetime(pod, node, reserved, second) {
		a.UID = ""
		if reserved {
			a.reservedNode, a.reservedAt = node.Name, second
		}
		return a, true
	}
	if !bound || a.UID == "" || a.reservedNode != "" {
		a.reservedAt = second
	}
	a.UID, a.nodeBirth = node.UID, node.CreationTimestamp.Time
	if bound {
		a.reservedNode = ""
	} else {
		a.reservedNode = node.Name
	}
	return a, false
}

func (a NodeAssignment) conflictsWithLifetime(pod *corev1.Pod, node *corev1.Node, reserved bool, second int64) bool {
	birth := node.CreationTimestamp
	if birth.IsZero() || birth.Before(&pod.CreationTimestamp) {
		// A Pod created after this node cannot have held an older generation.
		return false
	}
	if !a.boundBy.IsZero() && a.boundBy.Before(birth.Time) {
		return true
	}
	if reserved && second > 0 {
		if second < birth.Unix() {
			return true
		}
		// Filter has only second precision. A later Bind cannot prove on
		// which generation a reservation in the creation second was made.
		if second == birth.Unix() && a.UID == "" {
			return true
		}
	}
	// On a cold LIST, a Pod already present before the node needs some
	// evidence dating the allocation. A name alone can't resolve its UID.
	return a.UID == "" && !pod.CreationTimestamp.IsZero() && a.boundBy.IsZero() && second <= birth.Unix()
}

func (a *NodeAssignment) observeBindingTime(pod *corev1.Pod) {
	if a.hasScheduleTime {
		return
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionTrue && !c.LastTransitionTime.IsZero() {
			a.boundBy, a.hasScheduleTime = c.LastTransitionTime.Time, true
			return
		}
	}
	// The kubelet's start time is a fallback until we observe PodScheduled.
	if a.boundBy.IsZero() && pod.Status.StartTime != nil {
		a.boundBy = pod.Status.StartTime.Time
	}
}

func hasReservation(allocations []AllocationSnapshot) bool {
	return slices.ContainsFunc(allocations, func(s AllocationSnapshot) bool {
		return s.HasAllocationEvidence || len(s.Records) > 0
	})
}

func filterTime(pod *corev1.Pod) int64 {
	second, err := strconv.ParseInt(pod.Annotations["hami.io/vgpu-time"], 10, 64)
	if err != nil || second <= 0 {
		return 0
	}
	return second
}

func (a NodeAssignment) reservationOnReplacement(pod *corev1.Pod, node *corev1.Node, second int64) bool {
	if pod.Annotations["hami.io/vgpu-node"] != node.Name {
		return false
	}
	if a.reservedNode != "" && a.reservedNode != node.Name {
		return true
	}
	birth := node.CreationTimestamp
	if birth.IsZero() || second <= birth.Unix() {
		return false
	}
	// An unbound reservation can receive a fresh Filter decision on the replacement.
	if a.reservedNode != "" && second > a.reservedAt {
		return true
	}
	// The original Filter decision can also arrive before the new Node event.
	// A rewritten annotation after Bind cannot move that Pod to another UID.
	return birth.After(a.nodeBirth) && second == a.reservedAt
}
