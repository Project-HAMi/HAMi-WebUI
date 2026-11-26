package resourceview

import (
	"k8s.io/apimachinery/pkg/api/resource"

	pb "vgpu/api/observability/v1"
)

func unavailableMeasurement(unit, state, reason string) Quantity {
	return Quantity{Unit: unit, State: &pb.FieldState{State: state, Reason: reason, Source: "physical_identity", Scope: "device"}}
}

func measurement(q Quantity, unit string) Quantity {
	if q.State == nil {
		return unavailableMeasurement(unit, Unknown, "sample_missing")
	}
	if q.State.State != Known {
		q.Value = nil
		return q
	}
	if q.Unit != unit || q.Value == nil || q.Value.Sign() < 0 || q.State.Scope != "device" || q.State.Source == "" || !q.State.ObservedAt.IsValid() {
		return unavailableMeasurement(unit, Unknown, "invalid_sample")
	}
	if unit == "ratio" && q.Value.CmpInt64(1) > 0 {
		return unavailableMeasurement(unit, Unknown, "invalid_sample")
	}
	if unit == "bytes" {
		v := q.Value.Value()
		// Quantity.Value rounds fractional bytes. Verify the exact integer fits.
		if v < 0 || q.Value.Cmp(*resource.NewQuantity(v, resource.DecimalSI)) != 0 {
			return unavailableMeasurement(unit, Unknown, "invalid_sample")
		}
	}
	return q
}

// A metric name may differ between memory members. Source, scrape identity,
// and original sample time must still establish the same observation.
func sameReading(a, b Quantity) bool {
	x, y := a.State, b.State
	if x.Source != y.Source || !x.ObservedAt.AsTime().Equal(y.ObservedAt.AsTime()) {
		return false
	}
	for _, key := range []string{"job", "instance", "cluster"} {
		if x.Params[key] != y.Params[key] {
			return false
		}
	}
	return true
}

func mergeMeasurement(current, next Quantity) Quantity {
	if current.State.GetState() == Conflict {
		return current
	}
	if next.State.GetState() == Conflict || current.State == nil {
		return next
	}
	if next.Value == nil {
		return current
	}
	if current.Value == nil {
		return next
	}
	if current.Value.Cmp(*next.Value) != 0 || !sameReading(current, next) {
		return unavailableMeasurement(next.Unit, Conflict, "identity_conflict")
	}
	return current
}

// Observation reconciles already aged samples for this physical device. A
// missing value on one alias may be filled by another, but the two operands
// of a memory ratio must have been sampled together on at least one alias.
func (g PhysicalGroup) Observation(read func(Device) DeviceObservation) DeviceObservation {
	primary, fallback := g.reportingMembers()
	out := reconcileObservations(primary, read)
	if len(fallback) == 0 {
		return out
	}

	// The remaining registrations are lower-priority aliases of this same board.
	// Select memory as one pair so a handover never mixes two exporters.
	needUtil := needsFallback(out.UtilizationRatio)
	needMemory := needsFallback(out.MemoryUsedBytes, out.MemoryPhysicalBytes)
	if needUtil || needMemory {
		legacy := reconcileObservations(fallback, read)
		if needUtil && legacy.UtilizationRatio.Value != nil {
			out.UtilizationRatio = legacy.UtilizationRatio
		}
		if needMemory && legacy.MemoryUsedBytes.Value != nil && legacy.MemoryPhysicalBytes.Value != nil {
			out.MemoryUsedBytes, out.MemoryPhysicalBytes = legacy.MemoryUsedBytes, legacy.MemoryPhysicalBytes
		}
	}
	return out
}

// Conflicting or inapplicable primary fields must retain their diagnosis.
func needsFallback(fields ...Quantity) bool {
	missing := false
	for _, field := range fields {
		switch field.State.GetState() {
		case Known:
		case Unknown, Stale:
			missing = true
		default:
			return false
		}
	}
	return missing
}

func reconcileObservations(members []Device, read func(Device) DeviceObservation) DeviceObservation {
	var out DeviceObservation
	paired := false
	for _, member := range members {
		sample := read(member)
		used, capacity := measurement(sample.MemoryUsedBytes, "bytes"), measurement(sample.MemoryPhysicalBytes, "bytes")
		out.UtilizationRatio = mergeMeasurement(out.UtilizationRatio, measurement(sample.UtilizationRatio, "ratio"))
		out.MemoryUsedBytes = mergeMeasurement(out.MemoryUsedBytes, used)
		out.MemoryPhysicalBytes = mergeMeasurement(out.MemoryPhysicalBytes, capacity)
		paired = paired || (used.Value != nil && capacity.Value != nil && sameReading(used, capacity))
	}
	if !paired && out.MemoryUsedBytes.Value != nil && out.MemoryPhysicalBytes.Value != nil {
		out.MemoryUsedBytes = unavailableMeasurement("bytes", Conflict, "unpaired_samples")
		out.MemoryPhysicalBytes = unavailableMeasurement("bytes", Conflict, "unpaired_samples")
	}
	return out
}
