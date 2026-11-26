package resourceview

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	pb "vgpu/api/observability/v1"
)

func reading(text, unit, source string, sec int64) Quantity {
	value := resource.MustParse(text)
	return Quantity{Value: &value, Unit: unit, State: &pb.FieldState{State: Known, Source: source, Scope: "device", ObservedAt: timestamppb.New(time.Unix(sec, 0)), Params: map[string]string{"instance": "gpu:1234"}}}
}

func TestGroupObservationRequiresPairedMeasurements(t *testing.T) {
	read := func(d Device) DeviceObservation {
		return DeviceObservation{MemoryPhysicalBytes: d.MemoryPhysicalBytes, MemoryUsedBytes: d.MemoryUsedBytes, UtilizationRatio: d.UtilizationRatio}
	}
	compute := reading("0", "ratio", "DCGM", 100)
	used := reading("3", "bytes", "DCGM", 100)
	total := reading("8", "bytes", "DCGM", 100)
	cases := []struct {
		name                string
		members             []Device
		wantUsed, wantTotal bool
	}{
		{"paired", []Device{{MemoryUsedBytes: used, MemoryPhysicalBytes: total}, {}}, true, true},
		{"partial", []Device{{MemoryUsedBytes: used}, {}}, true, false},
		{"same scrape but different aliases", []Device{{MemoryUsedBytes: used}, {MemoryPhysicalBytes: total}}, false, false},
		{"different times on same card", []Device{{MemoryUsedBytes: used, MemoryPhysicalBytes: reading("8", "bytes", "DCGM", 101)}}, false, false},
		{"different sources on same card", []Device{{MemoryUsedBytes: used, MemoryPhysicalBytes: reading("8", "bytes", "other", 100)}}, false, false},
		{"fractional bytes", []Device{{MemoryUsedBytes: reading("1.1", "bytes", "DCGM", 100), MemoryPhysicalBytes: total}}, false, true},
		{"overflow bytes", []Device{{MemoryUsedBytes: reading("9223372036854775808", "bytes", "DCGM", 100)}}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.members[0].UtilizationRatio = compute
			got := (PhysicalGroup{Members: tc.members}).Observation(read)
			if (got.MemoryUsedBytes.Value != nil) != tc.wantUsed || (got.MemoryPhysicalBytes.Value != nil) != tc.wantTotal || got.UtilizationRatio.Value == nil || got.UtilizationRatio.Value.Sign() != 0 {
				t.Fatalf("got %v", got)
			}
		})
	}
	target := total.DeepCopy()
	target.State.Params["instance"] = "gpu:other"
	got := (PhysicalGroup{Members: []Device{{MemoryUsedBytes: used, MemoryPhysicalBytes: target}, {UtilizationRatio: reading("1.01", "ratio", "DCGM", 100)}}}).Observation(read)
	if got.MemoryUsedBytes.Value != nil || got.MemoryPhysicalBytes.Value != nil || got.UtilizationRatio.Value != nil {
		t.Fatalf("unsupported pair or rate: %v", got)
	}

	compute2 := reading("0", "ratio", "DCGM", 101)
	got = (PhysicalGroup{Members: []Device{{UtilizationRatio: compute}, {UtilizationRatio: compute2}}}).Observation(read)
	if got.UtilizationRatio.Value != nil || got.UtilizationRatio.State.GetState() != Conflict {
		t.Fatalf("accepted samples from different times: %v", got)
	}
}

func TestMultiGenerationRegistrationOverlap(t *testing.T) {
	device := func(provider, boardKey, id, node string, priority int) Device {
		return Device{
			Identity: DeviceIdentity{Provider: provider, DeviceID: id, Node: node, NodeUID: types.UID(node)},
			Physical: PhysicalAssociation{Domain: "board", BoardKey: boardKey, Priority: priority},
			Kind:     "physical",
			Model:    provider + "-Model",
			States:   map[string]*pb.FieldState{"kind": {State: Known}, "model": {State: Known}},
		}
	}
	current := device("Gen2", "board-A", "Gen2-board-A", "n1", 1)
	legacy := device("Gen1", "board-A", "Gen1-board-A", "n1", 0)
	current.UtilizationRatio = reading(".1", "ratio", "gen2_exporter", 100)
	legacy.UtilizationRatio = reading(".9", "ratio", "gen1_exporter", 100)
	rows := []Device{
		legacy,
		device("Gen1", "board-B", "Gen1-board-B", "n1", 0),
		device("Gen1", "board-A", "Gen1-board-A", "n2", 0),
		device("Gen1", "", "Gen1-0", "n1", 0),
		current,
	}
	inventory := GroupPhysicalDevices(Snapshot{Nodes: []Node{{Name: "n1", UID: "n1"}, {Name: "n2", UID: "n2"}}, Devices: rows})
	if !inventory.UnresolvedOnNode["n1"] {
		t.Fatal("slot-only registration under higher-priority generation counted as an identified card")
	}
	groups := inventory.Groups
	if len(groups) != 3 {
		t.Fatalf("different boards or nodes merged: %+v", groups)
	}
	for _, g := range groups {
		if len(g.Members) != 2 {
			continue
		}
		model, _ := g.Model()
		r := g.Observation(func(d Device) DeviceObservation { return DeviceObservation{UtilizationRatio: d.UtilizationRatio} })
		if v, ok := r.UtilizationRatio.KnownValue(); !ok || v != .1 || g.Members[0].Identity != current.Identity || model != "Gen2-Model" {
			t.Fatalf("overlap: %s, %+v, %+v", model, g, r)
		}
		return
	}
	t.Fatal("matching multi-generation board serials did not group")
}

func TestGenerationUpgradeObservation(t *testing.T) {
	current := Device{
		Identity: DeviceIdentity{Provider: "Gen2", DeviceID: "Gen2-board-A", Node: "node", NodeUID: "uid"},
		Physical: PhysicalAssociation{Domain: "board", BoardKey: "board-A", Priority: 1},
	}
	legacy := Device{
		Identity: DeviceIdentity{Provider: "Gen1", DeviceID: "Gen1-board-A", Node: "node", NodeUID: "uid"},
		Physical: PhysicalAssociation{Domain: "board", BoardKey: "board-A", Priority: 0},
	}
	gen2 := DeviceObservation{
		UtilizationRatio:    reading("0", "ratio", "gen2_exporter", 100),
		MemoryUsedBytes:     reading("0", "bytes", "gen2_exporter", 100),
		MemoryPhysicalBytes: reading("64", "bytes", "gen2_exporter", 100),
	}
	gen1 := DeviceObservation{
		UtilizationRatio:    reading(".75", "ratio", "gen1_exporter", 100),
		MemoryUsedBytes:     reading("12", "bytes", "gen1_exporter", 100),
		MemoryPhysicalBytes: reading("32", "bytes", "gen1_exporter", 100),
	}
	conflict := unavailableMeasurement("ratio", Conflict, "ambiguous_source_or_identity")
	for _, tc := range []struct {
		name                          string
		current, legacy               DeviceObservation
		wantUtil, wantUsed, wantTotal string
	}{
		{"fresh Gen2 zero wins", gen2, gen1, "gen2_exporter", "gen2_exporter", "gen2_exporter"},
		{"old collector only", DeviceObservation{}, gen1, "gen1_exporter", "gen1_exporter", "gen1_exporter"},
		{"partial Gen2 memory switches both operands", DeviceObservation{UtilizationRatio: gen2.UtilizationRatio, MemoryPhysicalBytes: gen2.MemoryPhysicalBytes}, gen1, "gen2_exporter", "gen1_exporter", "gen1_exporter"},
		{"missing Gen2 compute", DeviceObservation{MemoryUsedBytes: gen2.MemoryUsedBytes, MemoryPhysicalBytes: gen2.MemoryPhysicalBytes}, gen1, "gen1_exporter", "gen2_exporter", "gen2_exporter"},
		{"partial pairs stay separate", DeviceObservation{MemoryPhysicalBytes: gen2.MemoryPhysicalBytes}, DeviceObservation{MemoryUsedBytes: gen1.MemoryUsedBytes}, "", "", "gen2_exporter"},
		{"conflicting Gen2 compute", DeviceObservation{UtilizationRatio: conflict}, gen1, "", "gen1_exporter", "gen1_exporter"},
		{"stale Gen2 falls back", AgeObservation(gen2, time.Unix(200, 0), time.Second), gen1, "gen1_exporter", "gen1_exporter", "gen1_exporter"},
		{"conflicting Gen2 memory", DeviceObservation{MemoryUsedBytes: unavailableMeasurement("bytes", Conflict, "ambiguous_source_or_identity")}, gen1, "gen1_exporter", "", ""},
		{"stale Gen1 cannot fill gap", DeviceObservation{}, AgeObservation(gen1, time.Unix(200, 0), time.Second), "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := func(d Device) DeviceObservation {
				if d.Identity == current.Identity {
					return tc.current
				}
				return tc.legacy
			}
			source := func(q Quantity) string {
				if q.Value == nil {
					return ""
				}
				return q.State.GetSource()
			}
			for _, members := range [][]Device{{current, legacy}, {legacy, current}} {
				got := (PhysicalGroup{NodeUID: "uid", Members: members}).Observation(read)
				if source(got.UtilizationRatio) != tc.wantUtil || source(got.MemoryUsedBytes) != tc.wantUsed || source(got.MemoryPhysicalBytes) != tc.wantTotal {
					t.Fatalf("%s first: got compute, used and total from %q, %q, %q", members[0].Identity.Provider, source(got.UtilizationRatio), source(got.MemoryUsedBytes), source(got.MemoryPhysicalBytes))
				}
			}
		})
	}
}
