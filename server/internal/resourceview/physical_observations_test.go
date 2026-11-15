package resourceview

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"

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
