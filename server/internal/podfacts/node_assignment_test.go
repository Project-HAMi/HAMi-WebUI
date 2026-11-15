package podfacts

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeAssignmentBindingEvidence(t *testing.T) {
	birth := metav1.NewTime(time.Unix(2000, 0))
	before := metav1.NewTime(birth.Add(-time.Minute))
	after := metav1.NewTime(birth.Add(time.Minute))
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "new", CreationTimestamp: birth}}
	pod := &corev1.Pod{Spec: corev1.PodSpec{NodeName: node.Name}}
	scheduled := func(at metav1.Time, status corev1.ConditionStatus) []corev1.PodCondition {
		return []corev1.PodCondition{{Type: corev1.PodScheduled, Status: status, LastTransitionTime: at}}
	}
	check := func(a NodeAssignment, n *corev1.Node, wantConflict bool) NodeAssignment {
		t.Helper()
		got, conflict := a.Observe(pod, n, nil)
		if conflict != wantConflict || (n != nil && (got.UID == n.UID) == wantConflict) {
			t.Fatalf("assignment = %+v, conflict = %t, want conflict = %t", got, conflict, wantConflict)
		}
		return got
	}

	// Neither a failed scheduling attempt nor a same-second binding proves
	// the Pod was scheduled on an earlier node.
	pod.Status.Conditions = scheduled(before, corev1.ConditionFalse)
	check(NodeAssignment{}, node, false)
	pod.Status.Conditions = scheduled(birth, corev1.ConditionTrue)
	check(NodeAssignment{}, node, false)

	// Learn a timestamp even when the Node isn't in the cache yet.
	pod.Status.Conditions = scheduled(before, corev1.ConditionTrue)
	a := check(NodeAssignment{}, nil, false)
	pod.Status.Conditions = nil
	a = check(a, node, true)
	check(a, node, true)

	// A positive scheduling condition supersedes an earlier kubelet-clock
	// fallback. An incomplete later status cannot undo that observation.
	pod.Status.StartTime = &before
	a = check(NodeAssignment{}, node, true)
	pod.Status.Conditions = scheduled(after, corev1.ConditionTrue)
	a = check(a, node, false)
	pod.Status.Conditions = nil
	check(a, node, false)

	// With no recorded time, two creation timestamps rounded to the same
	// second cannot establish which generation a cold reservation belongs to.
	pod.CreationTimestamp = birth
	pod.Status = corev1.PodStatus{}
	check(NodeAssignment{}, node, true)
	pod.CreationTimestamp = after
	check(NodeAssignment{}, node, false)

	// A node with no creation timestamp supplies no temporal contradiction.
	node.CreationTimestamp = metav1.Time{}
	check(NodeAssignment{}, node, false)
}

func TestNodeAssignmentWithdrawnReservation(t *testing.T) {
	pod := &corev1.Pod{}
	old := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "old"}}
	current := old.DeepCopy()
	current.UID = "new"
	reservation := []AllocationSnapshot{{HasAllocationEvidence: true}}

	a, conflict := (NodeAssignment{}).Observe(pod, old, reservation)
	if conflict || a.UID != old.UID {
		t.Fatalf("original reservation = %+v, conflict = %t", a, conflict)
	}
	a, conflict = a.Observe(pod, current, reservation)
	if !conflict || a.UID != old.UID {
		t.Fatalf("unchanged reservation followed replacement: %+v", a)
	}
	// HAMi can withdraw a reservation while the Pod is still unbound.
	a, _ = a.Observe(pod, nil, nil)
	pod.Spec.NodeName = current.Name
	a, conflict = a.Observe(pod, current, reservation)
	if conflict || a.UID != current.UID {
		t.Fatalf("new assignment after release = %+v, conflict = %t", a, conflict)
	}
}

func TestNodeAssignmentReservationReissue(t *testing.T) {
	birth := metav1.NewTime(time.Unix(2000, 0))
	scheduled := metav1.NewTime(birth.Add(time.Minute))
	old := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker", UID: "old"}}
	reservation := []AllocationSnapshot{{HasAllocationEvidence: true}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		"hami.io/vgpu-node": old.Name, "hami.io/vgpu-time": "1900",
	}}}
	a, _ := (NodeAssignment{}).Observe(pod, old, reservation)
	for _, tc := range []struct {
		name, target, filterTime string
		conflict                 bool
	}{
		{"Bind alone", "worker", "1900", true},
		{"same-second Filter", "worker", "2000", true},
		{"malformed Filter time", "worker", "bad", true},
		{"new Filter on replacement", "worker", "2001", false},
		{"Filter moved reservation to another node", "another", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: tc.target, UID: "new", CreationTimestamp: birth}}
			bound := pod.DeepCopy()
			bound.Spec.NodeName = next.Name
			bound.Annotations["hami.io/vgpu-node"] = next.Name
			bound.Annotations["hami.io/vgpu-time"] = tc.filterTime
			bound.Status.Conditions = []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: scheduled,
			}}
			got, conflict := a.Observe(bound, next, reservation)
			if conflict != tc.conflict || (got.UID == next.UID) != !tc.conflict {
				t.Fatalf("assignment = %+v, conflict = %t, want conflict = %t", got, conflict, tc.conflict)
			}
			// Once bound, even a rewritten annotation cannot reassign this Pod UID.
			if !conflict {
				third := next.DeepCopy()
				third.UID = "third"
				bound.Annotations["hami.io/vgpu-time"] = "3000"
				if _, conflict := got.Observe(bound, third, reservation); !conflict {
					t.Fatal("a bound Pod followed its reservation onto another node UID")
				}
			}
		})
	}
}
