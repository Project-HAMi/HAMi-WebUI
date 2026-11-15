package podfacts

import (
	"reflect"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestLedgerDeleteReplacementGuardAndDetachedReads(t *testing.T) {
	p := fixture()
	f := extract(t, p, Evidence{}, Options{})
	l := NewLedger()
	if !l.Put(f, func() bool { return true }) {
		t.Fatal("put rejected")
	}
	f.Metadata.Name = "mutated"
	got, _ := l.Get(p.UID)
	if got.Facts.Metadata.Name != "worker" {
		t.Fatal("write aliases caller")
	}
	got.Facts.Metadata.Labels = map[string]string{"fake": "true"}
	if copy, _ := l.Get(p.UID); len(copy.Facts.Metadata.Labels) != 0 {
		t.Fatal("read aliases cache")
	}
	p.UID = "new-uid"
	p.ResourceVersion = "11"
	newer := extract(t, p, Evidence{}, Options{})
	l.Put(newer, func() bool { return true })
	l.Delete("pod-uid")
	if _, exists := l.Get("new-uid"); !exists {
		t.Fatal("late old delete removed replacement")
	}
	f.Metadata.Name = "worker"
	if l.Put(f, func() bool { return false }) {
		t.Fatal("stale catalog replay accepted")
	}
	if _, exists := l.Get("pod-uid"); exists {
		t.Fatal("old UID restored")
	}
	snapshot := l.Snapshot()
	if snapshot.Coverage.Complete || snapshot.Coverage.Total != nil || snapshot.Coverage.Observed != 1 {
		t.Fatal("untracked informer claimed a total")
	}
	l.Delete("new-uid")
	if len(l.Snapshot().Entries) != 0 {
		t.Fatal("delete failed")
	}
}
func TestAllocationSnapshotRetainsIdentityAndAccountingEvidence(t *testing.T) {
	l := NewLedger()
	p := fixture()
	put := func(name string, evidence Evidence) {
		p.Name, p.UID = name, types.UID(name)
		f := extract(t, p, evidence, Options{})
		l.Put(f, func() bool { return true })
	}
	put("cpu", Evidence{})
	p.Spec.Containers[0].Resources.Claims = []corev1.ResourceClaim{{Name: "gpu"}}
	put("claimed", Evidence{})
	p.Spec.Containers[0].Resources.Claims = nil
	p.Name, p.UID = "allocated", "allocated"
	set := allocationFixture(p)
	put("allocated", Evidence{Allocations: []AllocationSnapshot{set}})
	p.Status.Phase = corev1.PodSucceeded
	put("terminal", Evidence{})

	s := l.AllocationSnapshot()
	if len(s.Entries) != 4 {
		t.Fatalf("allocation snapshot lost Pod identities: %d", len(s.Entries))
	}
	for _, entry := range s.Entries {
		f := entry.Facts
		full, _ := l.Get(f.Metadata.UID)
		if !reflect.DeepEqual(f.Metadata, full.Facts.Metadata) || !reflect.DeepEqual(entry.Occupancies, full.Occupancies) {
			t.Fatalf("%s lost identity or occupancies", f.Metadata.Name)
		}
		switch f.Metadata.Name {
		case "cpu", "terminal":
			if len(f.Containers) != 0 || len(full.Facts.Containers) != 1 {
				t.Fatalf("%s: full and allocation views must retain different declarations", f.Metadata.Name)
			}
		case "claimed":
			if len(f.Containers[0].Claims) != 1 {
				t.Fatal("dropped unresolved resource claim")
			}
		case "allocated":
			r := &f.Containers[0].Allocations[0].Records[0]
			r.Quantities["memory"].Value.Set(0)
			next := l.AllocationSnapshot().Entries[0].Facts
			if next.Containers[0].Allocations[0].Records[0].Quantities["memory"].Value.IsZero() {
				t.Fatal("mutable allocation view aliases the ledger")
			}
		}
	}
}

func TestLedgerConcurrentSnapshotsAndUpdates(t *testing.T) {
	l := NewLedger()
	p := fixture()
	f := extract(t, p, Evidence{}, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 80; j++ {
				l.Put(f, func() bool { return true })
				l.Snapshot()
				l.Get(p.UID)
				l.Delete(p.UID)
			}
		}()
	}
	wg.Wait()
}
func peakFixture(t *testing.T) *Facts {
	p := fixture()
	always := corev1.ContainerRestartPolicyAlways
	p.Spec.InitContainers = []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "init"}}
	source := allocationFixture(p)
	source.Accounting = AccountingPodPeak
	source.Records = nil
	for name, value := range map[string]string{"sidecar": "2", "init": "3", "app": "1"} {
		source.Records = append(source.Records, Allocation{Container: name, RecordID: name, Resource: "nvidia.com/gpu", DeviceID: "GPU-a", Node: "node-a", NodeUID: "node-uid-a", Quantities: map[string]Quantity{"memory": {Value: quantity(value), Unit: "bytes", State: known("hami_annotation", "container")}}})
	}
	return extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
}
func TestPodPeakIncludesPrecedingSidecarsAndRetainsTerminalMembers(t *testing.T) {
	f := peakFixture(t)
	rows := EffectiveOccupancies(f)
	if len(rows) != 1 || !rows[0].Complete || rows[0].Quantities["memory"].Value.Cmp(*quantity("5")) != 0 {
		t.Fatalf("peak must be 2+3, not max(2,3,1): %+v", rows)
	}
	// Reverse init order: the ordinary init no longer overlaps this sidecar.
	f.Containers[0].Index = 1
	f.Containers[1].Index = 0
	if row := EffectiveOccupancies(f)[0]; row.Quantities["memory"].Value.Cmp(*quantity("3")) != 0 {
		t.Fatal("ignored original init order")
	}
	f.Terminal = true
	l := NewLedger()
	l.Put(f, func() bool { return true })
	entry, _ := l.Get(f.Metadata.UID)
	if len(entry.Occupancies) != 0 || len(entry.Facts.Containers) != 3 || len(entry.Facts.Containers[0].Allocations[0].Records) != 1 {
		t.Fatal("terminal cleared member facts or retained active occupancy")
	}
}
func TestPeakDoesNotGuessUnknownOrMixedUnitQuantities(t *testing.T) {
	for _, mutate := range []func(*Facts){
		func(f *Facts) { f.Containers[0].Allocations[0].Complete = false },
		func(f *Facts) { f.Containers[0].Allocations[0].Accounting = "" },
		func(f *Facts) {
			f.Containers[0].Allocations[0].Records[0].Quantities["memory"] = Quantity{State: unknown("hami_annotation", "container")}
		},
		func(f *Facts) {
			q := f.Containers[0].Allocations[0].Records[0].Quantities["memory"]
			q.Unit = "MiB"
			f.Containers[0].Allocations[0].Records[0].Quantities["memory"] = q
		},
	} {
		f := peakFixture(t)
		mutate(f)
		row := EffectiveOccupancies(f)[0]
		if row.Complete || row.Quantities["memory"].Value != nil {
			t.Fatal("incomplete/foreign source or mismatched units produced a value")
		}
	}
}
func TestPeakPreservesExactQuantities(t *testing.T) {
	f := peakFixture(t)
	for i, value := range []string{"2m", "3m", "1m"} {
		q := f.Containers[i].Allocations[0].Records[0].Quantities["memory"]
		q.Value = quantity(value)
		f.Containers[i].Allocations[0].Records[0].Quantities["memory"] = q
	}
	if got := EffectiveOccupancies(f)[0].Quantities["memory"].Value; got.Cmp(*quantity("5m")) != 0 {
		t.Fatalf("rounded quantity %s", got.String())
	}
}
