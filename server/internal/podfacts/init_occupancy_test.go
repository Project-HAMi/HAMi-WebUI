package podfacts

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func initReservationPod() *corev1.Pod {
	p := fixture()
	always := corev1.ContainerRestartPolicyAlways
	p.Spec.InitContainers = []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "setup"}, {Name: "verify"}}
	p.Status.Phase = corev1.PodRunning
	return p
}

func initStatus(name string, exit int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: name, State: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{ExitCode: exit},
	}}
}

func initReservationFacts(t *testing.T, p *corev1.Pod) *Facts {
	t.Helper()
	s := allocationFixture(p)
	s.Accounting = AccountingPodPeak
	s.Records = nil
	for _, a := range []struct{ name, gpu, memory, cores string }{
		{"sidecar", "A", "2Gi", "20"}, {"setup", "A", "8Gi", "80"}, {"setup", "B", "8Gi", "80"}, {"app", "A", "1Gi", "10"},
	} {
		s.Records = append(s.Records, Allocation{
			Container: a.name, RecordID: a.name + a.gpu, Node: "node", NodeUID: "node-uid", Resource: "nvidia.com/gpu", DeviceID: a.gpu,
			Quantities: map[string]Quantity{
				"memory":  {Value: quantity(a.memory), Unit: "bytes", State: known("hami_annotation", "container")},
				"compute": {Value: quantity(a.cores), Unit: "percent", State: known("hami_annotation", "container")},
				"slots":   {Value: quantity("1"), Unit: "slots", State: known("hami_annotation", "container")},
			},
		})
	}
	return extract(t, p, Evidence{Allocations: []AllocationSnapshot{s}}, Options{})
}

func TestHAMiCurrentInitReservation(t *testing.T) {
	ok := initStatus("setup", 0)
	verified := initStatus("verify", 0)
	retry := corev1.ContainerStatus{Name: "verify", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}, LastTerminationState: verified.State}
	for _, tc := range []struct {
		name     string
		statuses []corev1.ContainerStatus
		phase    corev1.PodPhase
		wantRows int
	}{
		{"missing statuses", nil, corev1.PodRunning, 2},
		{"missing declared init", []corev1.ContainerStatus{ok}, corev1.PodRunning, 2},
		{"failed attempt", []corev1.ContainerStatus{ok, initStatus("verify", 1)}, corev1.PodRunning, 2},
		{"restarting despite prior success", []corev1.ContainerStatus{ok, retry}, corev1.PodPending, 2},
		{"all ordinary init succeeded", []corev1.ContainerStatus{verified, ok}, corev1.PodRunning, 1},
		{"sidecar still running", []corev1.ContainerStatus{verified, {Name: "sidecar", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}, ok}, corev1.PodPending, 1},
		{"terminal succeeded", []corev1.ContainerStatus{verified, ok}, corev1.PodSucceeded, 0},
		{"terminal failed during init", []corev1.ContainerStatus{initStatus("setup", 1)}, corev1.PodFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := initReservationPod()
			p.Status.Phase, p.Status.InitContainerStatuses = tc.phase, tc.statuses
			f := initReservationFacts(t, p)
			rows := EffectiveOccupancies(f)
			if len(rows) != tc.wantRows {
				t.Fatalf("got %d devices, want %d", len(rows), tc.wantRows)
			}
			if len(container(f, "setup").Allocations[0].Records) != 2 {
				t.Fatal("lost original init records")
			}
			if len(rows) == 0 {
				return
			}
			mem, cores := "10Gi", "100"
			if len(rows) == 1 {
				mem, cores = "3Gi", "30"
			} else if rows[1].DeviceID != "B" || rows[1].Quantities["slots"].Value.Cmp(*quantity("1")) != 0 {
				t.Fatal("init-only card should retain its slot until release")
			}
			for field, want := range map[string]string{"memory": mem, "compute": cores, "slots": "2"} {
				got := rows[0].Quantities[field]
				if rows[0].DeviceID != "A" || !rows[0].Complete || got.Value == nil || got.Value.Cmp(*quantity(want)) != 0 {
					t.Fatalf("%s: got %+v, want %s", field, got, want)
				}
			}
		})
	}
}

func TestHAMiIncompleteDecodeCannotLatchInitRelease(t *testing.T) {
	p := initReservationPod()
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{initStatus("setup", 0), initStatus("verify", 0)}
	set := allocationFixture(p)
	set.Complete = false
	set.Accounting = AccountingPodPeak
	f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{set}}, Options{})
	ledger := NewLedger()
	ledger.Put(f, func() bool { return true })
	p.Status.InitContainerStatuses = nil
	ledger.Put(initReservationFacts(t, p), func() bool { return true })
	entry, _ := ledger.Get(p.UID)
	if len(entry.Occupancies) != 2 {
		t.Fatal("a failed decode must not release the init reservation")
	}
}

func TestHAMiInitReleaseFollowsPodUID(t *testing.T) {
	l := NewLedger()
	p := initReservationPod()
	put := func(want int) {
		t.Helper()
		l.Put(initReservationFacts(t, p), func() bool { return true })
		entry, _ := l.Get(p.UID)
		if len(entry.Occupancies) != want {
			t.Fatalf("got %d occupied devices, want %d", len(entry.Occupancies), want)
		}
	}
	put(2)
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{initStatus("setup", 0), initStatus("verify", 0)}
	put(1)
	p.Status.InitContainerStatuses = nil
	put(1) // An informer replay cannot inflate the released reservation for this UID.
	p.UID = "replacement"
	put(2)
	now := metav1.Now()
	p.DeletionTimestamp = &now
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{initStatus("setup", 0), initStatus("verify", 0)}
	put(2) // A cached terminating Pod keeps its previous reservation.
	p.UID = "first-seen-terminating"
	put(1) // Initial sync restores the completed init transition.
	p.UID = "unassigned-then-terminating"
	l.Put(extract(t, p, Evidence{}, Options{}), func() bool { return true })
	put(1) // Seeing the Pod before a decodable assignment was not a cached reservation.
}
