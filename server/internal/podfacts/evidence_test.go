package podfacts

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func allocationFixture(p *corev1.Pod) AllocationSnapshot {
	return AllocationSnapshot{PodUID: p.UID, ResourceVersion: p.ResourceVersion, Source: &viewv1.SourceState{Source: "hami_annotation", State: resourceview.Known}, Complete: true, Records: []Allocation{{Container: "app", RecordID: "slot0", Resource: "nvidia.com/gpu", DeviceID: "GPU-a", RawMode: "ENPU-unknown-alias", ConfiguredMode: textPointer("template"), States: map[string]*viewv1.FieldState{"configuredMode": known("node_config", "device")}, Quantities: map[string]Quantity{"memory": {Value: quantity("4Gi"), Unit: "bytes", State: known("hami_annotation", "container")}, "compute": {State: unknown("hami_annotation", "container")}}}}}
}
func TestObservedAllocationIsSeparateFromRequestsAndModeConfiguration(t *testing.T) {
	p := fixture()
	source := allocationFixture(p)
	f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
	app := container(f, "app")
	if f.Candidate != CandidateAccelerator || len(app.Requests) != 0 || len(app.Allocations) != 1 || !app.Allocations[0].Complete {
		t.Fatalf("allocation/request separation: %+v", app)
	}
	got := app.Allocations[0].Records[0]
	if got.AllocationMode != nil || got.Template != nil || got.States["allocationMode"].State != resourceview.Unknown || got.RawMode != "ENPU-unknown-alias" || *got.ConfiguredMode != "template" {
		t.Fatal("configured mode or absent template fabricated allocation mode")
	}
	if got.Quantities["memory"].Value.Value() != 4*1024*1024*1024 || got.Quantities["compute"].Value != nil {
		t.Fatal("known memory or unknown compute lost")
	}
	if app.States["allocations"].State != resourceview.Unknown {
		t.Fatal("one source fabricated completeness of every allocation source")
	}
	got.Quantities["memory"].Value.Set(0)
	got.States["allocationMode"].Reason = "changed"
	*got.ConfiguredMode = "changed"
	if source.Records[0].Quantities["memory"].Value.Value() == 0 || *source.Records[0].ConfiguredMode != "template" {
		t.Fatal("allocation input aliased")
	}
}
func TestCompleteEmptyAllocationEvidenceIsSourceScoped(t *testing.T) {
	p := fixture()
	source := allocationFixture(p)
	source.Records = nil
	f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
	sets := container(f, "app").Allocations
	if len(sets) != 1 || !sets[0].Complete || len(sets[0].Records) != 0 || f.Candidate != NoAcceleratorEvidence {
		t.Fatal("known empty source not represented truthfully")
	}
	if container(f, "app").States["allocations"].State != resourceview.Unknown {
		t.Fatal("single empty source became no allocations overall")
	}
}
func TestAllocationEvidenceCannotCrossUIDVersionOrUnavailableSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*AllocationSnapshot)
		want   Candidate
	}{
		{"replaced UID", func(s *AllocationSnapshot) { s.PodUID = "previous-pod" }, NoAcceleratorEvidence},
		{"older snapshot", func(s *AllocationSnapshot) { s.ResourceVersion = "9" }, NoAcceleratorEvidence},
		{"missing snapshot version", func(s *AllocationSnapshot) { s.ResourceVersion = "" }, NoAcceleratorEvidence},
		{"watch unavailable", func(s *AllocationSnapshot) {
			s.Source.State = resourceview.Unknown
			s.Source.Reason = "temporarily_unavailable"
		}, CandidateUnresolved},
		{"invalid source state", func(s *AllocationSnapshot) { s.Source.State = "available" }, NoAcceleratorEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixture()
			source := allocationFixture(p)
			tc.mutate(&source)
			f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
			set := container(f, "app").Allocations[0]
			if set.Complete || len(set.Records) != 0 || f.Candidate != tc.want {
				t.Fatalf("untrusted allocation became current: %+v", f)
			}
		})
	}
}
func TestMalformedOrDuplicateRecordsDoNotProveAnEmptyInventory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*AllocationSnapshot)
	}{
		{"unknown container", func(s *AllocationSnapshot) { s.Records[0].Container = "missing" }},
		{"missing record identity", func(s *AllocationSnapshot) { s.Records[0].RecordID = "" }},
		{"missing mode evidence", func(s *AllocationSnapshot) { s.Records[0].AllocationMode = textPointer("whole") }},
		{"unknown carrying quantity", func(s *AllocationSnapshot) {
			q := s.Records[0].Quantities["memory"]
			q.State = unknown("hami_annotation", "container")
			s.Records[0].Quantities["memory"] = q
		}},
		{"negative quantity", func(s *AllocationSnapshot) {
			q := s.Records[0].Quantities["memory"]
			q.Value = quantity("-1")
			s.Records[0].Quantities["memory"] = q
		}},
		{"duplicate record", func(s *AllocationSnapshot) { s.Records = append(s.Records, s.Records[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := fixture()
			source := allocationFixture(p)
			tc.mutate(&source)
			f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
			if container(f, "app").Allocations[0].Complete || len(f.Issues) == 0 || f.Candidate == NoAcceleratorEvidence {
				t.Fatal("invalid record was silently counted as complete empty")
			}
		})
	}
	p := fixture()
	source := allocationFixture(p)
	f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source, source}}, Options{})
	if sets := container(f, "app").Allocations; len(sets) != 1 || sets[0].Complete || len(sets[0].Records) != 0 {
		t.Fatal("duplicate source inflated allocation evidence")
	}
}
func TestRegularInitSidecarAllocationsAreNotSummed(t *testing.T) {
	p := fixture()
	always := corev1.ContainerRestartPolicyAlways
	p.Spec.InitContainers = []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "init"}}
	source := allocationFixture(p)
	sidecar := source.Records[0]
	sidecar.Container = "sidecar"
	sidecar.RecordID = "slot1"
	init := source.Records[0]
	init.Container = "init"
	init.RecordID = "slot2"
	source.Records = append(source.Records, sidecar, init)
	f := extract(t, p, Evidence{Allocations: []AllocationSnapshot{source}}, Options{})
	for _, ctr := range f.Containers {
		if len(ctr.Allocations[0].Records) != 1 {
			t.Fatal("container records merged/dropped")
		}
	}
	if f.Containers[0].Index != 0 || f.Containers[1].Index != 1 {
		t.Fatal("init peak evaluation lost original order")
	}
	// No Pod allocation total exists here: effective lifetime occupancy is E2a-2.
}
func TestInvalidIdentityFailsInsteadOfInventingStableIDs(t *testing.T) {
	for _, mutate := range []func(*corev1.Pod){func(p *corev1.Pod) { p.UID = "" }, func(p *corev1.Pod) { p.Spec.InitContainers = []corev1.Container{{Name: "app"}} }, func(p *corev1.Pod) { p.Spec.Containers[0].Name = "" }} {
		p := fixture()
		mutate(p)
		if _, err := Extract(p, Evidence{}, Options{}); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}
