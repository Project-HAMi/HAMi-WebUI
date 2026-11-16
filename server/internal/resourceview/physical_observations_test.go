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

func TestHygonRegistrationOverlap(t *testing.T) {
	device := func(provider, serial, node string) Device {
		return Device{Identity: DeviceIdentity{Provider: provider, DeviceID: provider + "-" + serial, Node: node, NodeUID: types.UID(node)}, Kind: "physical", Model: provider + "-K100", States: map[string]*pb.FieldState{"kind": {State: Known}, "model": {State: Known}}}
	}
	current := device("HCU", "board-A", "n1")
	legacy := device("DCU", "board-A", "n1")
	current.UtilizationRatio = reading(".1", "ratio", "hcu_exporter", 100)
	legacy.UtilizationRatio = reading(".9", "ratio", "dcu_exporter", 100)
	rows := []Device{legacy, device("DCU", "board-B", "n1"), device("DCU", "board-A", "n2"), device("DCU", "0", "n1"), current}
	inventory := GroupPhysicalDevices(Snapshot{Nodes: []Node{{Name: "n1", UID: "n1"}, {Name: "n2", UID: "n2"}}, Devices: rows})
	if !inventory.UnresolvedOnNode["n1"] {
		t.Fatal("legacy slot under dual registration counted as an identified card")
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
		if v, ok := r.UtilizationRatio.KnownValue(); !ok || v != .1 || g.Members[0].Identity != current.Identity || model != "HCU-K100" {
			t.Fatalf("overlap: %s, %+v, %+v", model, g, r)
		}
		return
	}
	t.Fatal("matching HCU and DCU board serials did not group")
}

func TestHygonUpgradeObservation(t *testing.T) {
	current := Device{Identity: DeviceIdentity{Provider: "HCU", DeviceID: "HCU-board-A", Node: "node", NodeUID: "uid"}}
	legacy := Device{Identity: DeviceIdentity{Provider: "DCU", DeviceID: "DCU-board-A", Node: "node", NodeUID: "uid"}}
	hcu := DeviceObservation{
		UtilizationRatio:    reading("0", "ratio", "hcu_exporter", 100),
		MemoryUsedBytes:     reading("0", "bytes", "hcu_exporter", 100),
		MemoryPhysicalBytes: reading("64", "bytes", "hcu_exporter", 100),
	}
	dcu := DeviceObservation{
		UtilizationRatio:    reading(".75", "ratio", "dcu_exporter", 100),
		MemoryUsedBytes:     reading("12", "bytes", "dcu_exporter", 100),
		MemoryPhysicalBytes: reading("32", "bytes", "dcu_exporter", 100),
	}
	conflict := unavailableMeasurement("ratio", Conflict, "ambiguous_source_or_identity")
	for _, tc := range []struct {
		name                          string
		current, legacy               DeviceObservation
		wantUtil, wantUsed, wantTotal string
	}{
		{"fresh HCU zero wins", hcu, dcu, "hcu_exporter", "hcu_exporter", "hcu_exporter"},
		{"old collector only", DeviceObservation{}, dcu, "dcu_exporter", "dcu_exporter", "dcu_exporter"},
		{"partial HCU memory switches both operands", DeviceObservation{UtilizationRatio: hcu.UtilizationRatio, MemoryPhysicalBytes: hcu.MemoryPhysicalBytes}, dcu, "hcu_exporter", "dcu_exporter", "dcu_exporter"},
		{"missing HCU compute", DeviceObservation{MemoryUsedBytes: hcu.MemoryUsedBytes, MemoryPhysicalBytes: hcu.MemoryPhysicalBytes}, dcu, "dcu_exporter", "hcu_exporter", "hcu_exporter"},
		{"partial pairs stay separate", DeviceObservation{MemoryPhysicalBytes: hcu.MemoryPhysicalBytes}, DeviceObservation{MemoryUsedBytes: dcu.MemoryUsedBytes}, "", "", "hcu_exporter"},
		{"conflicting HCU compute", DeviceObservation{UtilizationRatio: conflict}, dcu, "", "dcu_exporter", "dcu_exporter"},
		{"stale HCU falls back", AgeObservation(hcu, time.Unix(200, 0), time.Second), dcu, "dcu_exporter", "dcu_exporter", "dcu_exporter"},
		{"conflicting HCU memory", DeviceObservation{MemoryUsedBytes: unavailableMeasurement("bytes", Conflict, "ambiguous_source_or_identity")}, dcu, "dcu_exporter", "", ""},
		{"stale DCU cannot fill gap", DeviceObservation{}, AgeObservation(dcu, time.Unix(200, 0), time.Second), "", "", ""},
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
