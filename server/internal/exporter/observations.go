package exporter

import (
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func NewObservedSnapshotSource(base resourceview.SnapshotSource, generator *MetricsGenerator) *resourceview.ObservedSnapshotSource {
	return resourceview.WithObservations(base, generator)
}

// Each refresh replaces the entire map. Once published, its entries are immutable.
func (s *MetricsGenerator) publishedObservations() resourceview.DeviceObservations {
	s.observationsMu.RLock()
	defer s.observationsMu.RUnlock()
	return s.observations
}

// DeviceObservations never queries upstream. Both API source wrapping and the
// new metric collector consume this detached view of the same published round.
func (s *MetricsGenerator) DeviceObservations() resourceview.DeviceObservations {
	published := s.publishedObservations()
	result := make(resourceview.DeviceObservations, len(published))
	now := s.clockNow()
	for identity, observation := range published {
		result[identity] = resourceview.AgeObservation(observation, now, dcgmMaxSampleAge)
	}
	return result
}

func unknownObservation(unit, source, reason string) resourceview.Quantity {
	return resourceview.Quantity{Unit: unit, State: &viewv1.FieldState{State: resourceview.Unknown, Reason: reason, Source: source, Scope: "device"}}
}

func observationQuantity(reading dcgmReading, scale float64, unit, source string, previous resourceview.Quantity, now time.Time) resourceview.Quantity {
	if reading.err != nil {
		if errors.Is(reading.err, errTelemetryQuery) && previous.Value != nil && previous.State != nil && previous.State.ObservedAt != nil {
			at := previous.State.ObservedAt.AsTime()
			if !at.After(now) && now.Sub(at) <= dcgmMaxSampleAge {
				stale := previous.DeepCopy()
				stale.State.State, stale.State.Reason = resourceview.Stale, "query_failed"
				return stale
			}
		}
		result := unknownObservation(unit, source, "sample_missing")
		switch {
		case errors.Is(reading.err, errTelemetryQuery):
			result.State.Reason = "query_failed"
		case errors.Is(reading.err, errTargetDown):
			result.State.Reason = "target_down"
		case errors.Is(reading.err, errDCGMStale):
			result.State.Reason = "sample_expired"
		case errors.Is(reading.err, errDCGMInvalid):
			result.State.Reason = "invalid_sample"
		case errors.Is(reading.err, errDCGMAmbiguous):
			result.State.State, result.State.Reason = resourceview.Conflict, "ambiguous_source_or_identity"
		}
		return result
	}

	value := reading.selection.Value * scale
	if !dcgmFinite(value) || value < 0 || reading.selection.SampleAt.IsZero() {
		return unknownObservation(unit, source, "invalid_sample")
	}
	// The resource API represents bytes as int64. A malformed fractional or
	// out-of-range source sample must not break the entire device inventory,
	// and rounding it would silently change the observation's value.
	if unit == "bytes" && (math.Trunc(value) != value || value >= 0x1p63) {
		result := unknownObservation(unit, source, "invalid_sample")
		result.State.ObservedAt = timestamppb.New(reading.selection.SampleAt)
		result.State.Params = map[string]string{"expected": "nonnegative_int64_bytes", "rawName": reading.selection.RawName}
		return result
	}
	quantity, err := resource.ParseQuantity(strconv.FormatFloat(value, 'f', -1, 64))
	if err != nil {
		return unknownObservation(unit, source, "invalid_sample")
	}
	return resourceview.Quantity{Value: &quantity, Unit: unit, State: &viewv1.FieldState{State: resourceview.Known, Source: source, Scope: "device", ObservedAt: timestamppb.New(reading.selection.SampleAt), Params: map[string]string{"job": reading.selection.Source.Job, "instance": reading.selection.Source.Instance, "cluster": reading.selection.Source.Cluster, "rawName": reading.selection.RawName}}}
}
