// Package resourceview enforces the common evidence rules of the new API.
// It does not reinterpret old DTOs or invent missing source measurements.
package resourceview

import (
	"fmt"
	"math"

	viewv1 "vgpu/api/observability/v1"
)

const (
	Known         = "known"
	Unknown       = "unknown"
	Stale         = "stale"
	Conflict      = "conflict"
	NotApplicable = "not_applicable"
)

func validState(state string) bool {
	switch state {
	case Known, Unknown, Stale, Conflict, NotApplicable:
		return true
	default:
		return false
	}
}

func validEntity(entity string) bool {
	switch entity {
	case "device", "node", "occupancy", "container", "pod", "workload":
		return true
	default:
		return false
	}
}

// ValidateField checks value presence separately from its numeric value: zero
// is evidence, while an absent key is not. Every observation needs provenance;
// a known ledger value may omit metadata. A nil sample time remains unknown.
func ValidateField(hasValue bool, state *viewv1.FieldState, observation bool) error {
	if state == nil {
		if !hasValue || observation {
			return fmt.Errorf("missing field state")
		}
		return nil
	}
	if !validState(state.State) {
		return fmt.Errorf("invalid field state %q", state.State)
	}
	if state.ObservedAt != nil {
		if err := state.ObservedAt.CheckValid(); err != nil {
			return fmt.Errorf("invalid sample time: %w", err)
		}
	}
	switch state.State {
	case Known:
		if !hasValue || state.Reason != "" {
			return fmt.Errorf("known field requires a value and no reason")
		}
	case Stale:
		if state.ObservedAt == nil || state.Reason == "" {
			return fmt.Errorf("stale field requires its sample time and reason")
		}
	default:
		if hasValue || state.Reason == "" {
			return fmt.Errorf("%s field must omit its value and explain why", state.State)
		}
	}
	if observation && (state.State == Known || state.State == Stale) {
		if state.Source == "" {
			return fmt.Errorf("observation requires a source")
		}
		switch state.Scope {
		case "device", "instance", "pod", "container":
		default:
			return fmt.Errorf("invalid observation scope %q", state.Scope)
		}
	}
	return nil
}

// ValidateRatio accepts overcommit above one. Callers must not clamp it away.
// Non-finite PromQL samples belong to the PromQL contract, not resource values.
func ValidateRatio(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return fmt.Errorf("resource ratio must be finite and non-negative")
	}
	return nil
}

// ValidateCoverage never equates capacity, slots or requested resources with
// an entity inventory. Semantic comparability must be established by the source.
func ValidateCoverage(coverage *viewv1.Coverage) error {
	if coverage == nil {
		return fmt.Errorf("missing inventory coverage")
	}
	if !validEntity(coverage.Entity) {
		return fmt.Errorf("invalid coverage entity %q", coverage.Entity)
	}
	if coverage.Observed < 0 || (coverage.Total != nil && *coverage.Total < coverage.Observed) {
		return fmt.Errorf("invalid inventory counts")
	}
	if coverage.Complete && (coverage.Total == nil || *coverage.Total != coverage.Observed) {
		return fmt.Errorf("complete coverage requires total to equal observed")
	}
	seen := make(map[string]bool, len(coverage.Sources))
	if len(coverage.Sources) == 0 {
		return fmt.Errorf("coverage must identify its inventory sources")
	}
	for _, source := range coverage.Sources {
		if source == nil || source.Source == "" || !validState(source.State) {
			return fmt.Errorf("invalid inventory source")
		}
		if seen[source.Source] {
			return fmt.Errorf("duplicate inventory source %q", source.Source)
		}
		seen[source.Source] = true
		if source.State == Known && source.Reason != "" {
			return fmt.Errorf("known inventory source must not have a failure reason")
		}
		if source.State != Known && source.Reason == "" {
			return fmt.Errorf("unavailable inventory source must explain why")
		}
		if coverage.Complete && source.State != Known && source.State != NotApplicable {
			return fmt.Errorf("unavailable inventory source cannot prove complete coverage")
		}
	}
	for field, count := range coverage.Fields {
		if field == "" || count == nil {
			return fmt.Errorf("invalid field coverage")
		}
		if count.Entity != "" && !validEntity(count.Entity) {
			return fmt.Errorf("invalid field coverage entity %q", count.Entity)
		}
		var total int64
		for _, n := range []int32{count.Known, count.Unknown, count.Stale, count.Conflict, count.NotApplicable} {
			if n < 0 {
				return fmt.Errorf("negative field coverage count")
			}
			total += int64(n)
		}
		if (count.Entity == "" || count.Entity == coverage.Entity) && total != int64(coverage.Observed) {
			return fmt.Errorf("field %q must classify every observed entity once", field)
		}
		var reasonTotal int64
		for reason, n := range count.Reasons {
			if reason == "" || n < 0 {
				return fmt.Errorf("invalid field coverage reason")
			}
			reasonTotal += int64(n)
		}
		if reasonTotal != total-int64(count.Known) {
			return fmt.Errorf("field %q must give one reason per non-known entity", field)
		}
	}
	return nil
}
