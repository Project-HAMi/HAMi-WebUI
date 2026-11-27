package resourceview

import (
	"context"
	"time"
)

// DeviceObservations is keyed by the exact source identity, including NodeUID.
// Public names alone cannot carry observations across a recreated Node.
type DeviceObservations map[DeviceIdentity]DeviceObservation
type DeviceObservation struct {
	MemoryPhysicalBytes, MemoryUsedBytes, UtilizationRatio Quantity
	TemperatureCelsius, PowerWatts                         Quantity
	MigMemoryUsedBytes, MigMemoryTotalBytes                Quantity
}

// ObservationReader returns detached readings owned by the caller.
type ObservationReader interface{ DeviceObservations() DeviceObservations }

func (d DeviceObservation) DeepCopy() DeviceObservation {
	return DeviceObservation{
		MemoryPhysicalBytes: d.MemoryPhysicalBytes.DeepCopy(),
		MemoryUsedBytes:     d.MemoryUsedBytes.DeepCopy(),
		UtilizationRatio:    d.UtilizationRatio.DeepCopy(),
		TemperatureCelsius:  d.TemperatureCelsius.DeepCopy(),
		PowerWatts:          d.PowerWatts.DeepCopy(),
		MigMemoryUsedBytes:  d.MigMemoryUsedBytes.DeepCopy(),
		MigMemoryTotalBytes: d.MigMemoryTotalBytes.DeepCopy(),
	}
}

// AgeObservation changes known to stale at read time as well as at refresh time.
// A stopped/failed producer can never leave its last snapshot known forever.
func AgeObservation(observation DeviceObservation, now time.Time, maxAge time.Duration) DeviceObservation {
	result := observation.DeepCopy()
	for _, field := range []*Quantity{
		&result.MemoryPhysicalBytes,
		&result.MemoryUsedBytes,
		&result.UtilizationRatio,
		&result.TemperatureCelsius,
		&result.PowerWatts,
		&result.MigMemoryUsedBytes,
		&result.MigMemoryTotalBytes,
	} {
		if field.State == nil || field.State.State != Known {
			continue
		}
		if field.State.ObservedAt == nil {
			field.Value = nil
			field.State.State, field.State.Reason = Unknown, "sample_time_unknown"
			continue
		}
		at := field.State.ObservedAt.AsTime()
		if at.After(now) {
			field.Value = nil
			field.State.State, field.State.Reason = Unknown, "sample_time_in_future"
		} else if maxAge <= 0 || now.Sub(at) > maxAge {
			field.State.State, field.State.Reason = Stale, "sample_expired"
		}
	}
	return result
}

// ObservedSnapshotSource is a distinct DI type from the base ledger source.
// Snapshot only joins detached local snapshots; it performs no Prometheus I/O.
type ObservedSnapshotSource struct {
	Base   SnapshotSource
	Reader ObservationReader
}

func WithObservations(base SnapshotSource, reader ObservationReader) *ObservedSnapshotSource {
	return &ObservedSnapshotSource{Base: base, Reader: reader}
}

func (s *ObservedSnapshotSource) Snapshot(ctx context.Context) (Snapshot, error) {
	snapshot, err := s.Base.Snapshot(ctx)
	if err != nil {
		return snapshot, err
	}
	if s.Reader == nil {
		return snapshot, nil
	}
	observations := s.Reader.DeviceObservations()
	for i := range snapshot.Devices {
		device := &snapshot.Devices[i]
		// Empty UID cannot prove that two snapshots refer to the same Node.
		if device.Identity.NodeUID == "" {
			continue
		}
		observation, exists := observations[device.Identity]
		if !exists {
			continue
		}
		device.MemoryPhysicalBytes = observation.MemoryPhysicalBytes
		device.MemoryUsedBytes = observation.MemoryUsedBytes
		device.UtilizationRatio = observation.UtilizationRatio
		device.TemperatureCelsius = observation.TemperatureCelsius
		device.PowerWatts = observation.PowerWatts
		device.MigMemoryUsedBytes = observation.MigMemoryUsedBytes
		device.MigMemoryTotalBytes = observation.MigMemoryTotalBytes
	}
	return snapshot, nil
}
