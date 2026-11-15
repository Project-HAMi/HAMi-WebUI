package podfacts

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func (f *Facts) readAllocations(pod *corev1.Pod, snapshots []AllocationSnapshot, options Options) {
	seenSources := map[string]bool{}
	sourceCounts := map[string]int{}
	for _, snapshot := range snapshots {
		if snapshot.Source != nil {
			sourceCounts[snapshot.Source.Source]++
		}
	}
	for _, snapshot := range snapshots {
		source := cloneSource(snapshot.Source)
		if seenSources[source.Source] {
			continue
		}
		seenSources[source.Source] = true
		validSource := snapshot.Source != nil && sourceCounts[source.Source] == 1 && resourceview.ValidateCoverage(&viewv1.Coverage{Entity: "pod", Sources: []*viewv1.SourceState{source}}) == nil
		bound := snapshot.PodUID == pod.UID && snapshot.ResourceVersion != "" && snapshot.ResourceVersion == pod.ResourceVersion
		complete := snapshot.Complete && validSource && bound && source.State == resourceview.Known && source.Reason == ""
		if !validSource || !bound {
			source.State, source.Reason = resourceview.Unknown, "unverified"
			f.Issues = append(f.Issues, Issue{"allocations." + source.Source, "pod_snapshot_or_source_mismatch"})
		}
		records := map[string][]Allocation{}
		if validSource && bound && source.State != resourceview.Known && (len(snapshot.Records) > 0 || snapshot.HasAllocationEvidence) {
			f.unresolved("allocation_source_unavailable:" + source.Source)
		}
		if validSource && bound && source.State == resourceview.Known && source.Reason == "" {
			seenRecords := map[string]bool{}
			for _, record := range snapshot.Records {
				exists := false
				for _, ctr := range f.Containers {
					if ctr.Key.Name == record.Container && ctr.Kind != Ephemeral {
						exists = true
						break
					}
				}
				key := record.Container + "\x00" + record.RecordID
				if !exists || record.RecordID == "" || seenRecords[key] {
					complete = false
					f.unresolved("invalid_allocation_record:" + source.Source)
					f.Issues = append(f.Issues, Issue{"allocations." + source.Source, "unknown_container_or_duplicate_record"})
					continue
				}
				seenRecords[key] = true
				cloned, err := cloneAllocation(record, source.Source)
				if err != nil {
					complete = false
					f.unresolved("invalid_allocation_record:" + source.Source)
					f.Issues = append(f.Issues, Issue{"allocations." + source.Source, err.Error()})
					continue
				}
				records[record.Container] = append(records[record.Container], cloned)
				if record.Resource == "" {
					f.unresolved("allocation_resource_unidentified:" + source.Source)
				}
				if source.State == resourceview.Known && source.Reason == "" && recognizes(corev1.ResourceName(record.Resource), options) {
					f.Candidate = CandidateAccelerator
					f.CandidateEvidence = append(f.CandidateEvidence, "allocation:"+source.Source+":"+record.Resource)
				}
			}
		}
		for i := range f.Containers {
			ctr := &f.Containers[i]
			if ctr.Kind == Ephemeral {
				continue
			}
			ctr.Allocations = append(ctr.Allocations, AllocationSet{Accounting: snapshot.Accounting, Source: cloneSource(source), Complete: complete, Records: records[ctr.Key.Name]})
		}
	}
}
func cloneAllocation(record Allocation, source string) (Allocation, error) {
	out := record
	if record.ComputeUnlimited != nil {
		value := *record.ComputeUnlimited
		out.ComputeUnlimited = &value
	}
	out.ConfiguredMode = copyString(record.ConfiguredMode)
	out.AllocationMode = copyString(record.AllocationMode)
	out.Template = copyString(record.Template)
	out.States = map[string]*viewv1.FieldState{}
	for key, state := range record.States {
		out.States[key] = cloneState(state)
	}
	if state := out.States["computeUnlimited"]; state == nil {
		if out.ComputeUnlimited != nil {
			return Allocation{}, fmt.Errorf("missing_computeUnlimited_evidence")
		}
		out.States["computeUnlimited"] = unknown(source, "container")
	} else if err := resourceview.ValidateField(out.ComputeUnlimited != nil, state, false); err != nil {
		return Allocation{}, fmt.Errorf("computeUnlimited: %w", err)
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{"configuredMode", out.ConfiguredMode}, {"allocationMode", out.AllocationMode}, {"template", out.Template}} {
		if field.value != nil && *field.value == "" {
			return Allocation{}, fmt.Errorf("empty_%s_evidence", field.name)
		}
		state := out.States[field.name]
		if state == nil {
			if field.value != nil {
				return Allocation{}, fmt.Errorf("missing_%s_evidence", field.name)
			}
			out.States[field.name] = unknown(source, "container")
			continue
		}
		if err := resourceview.ValidateField(field.value != nil, state, false); err != nil {
			return Allocation{}, fmt.Errorf("invalid_%s_evidence", field.name)
		}
	}
	out.Quantities = map[string]Quantity{}
	for key, quantity := range record.Quantities {
		if key == "" || (quantity.Value != nil && (quantity.Unit == "" || quantity.Value.Sign() < 0)) || quantity.State == nil {
			return Allocation{}, fmt.Errorf("invalid_quantity_evidence")
		}
		if err := resourceview.ValidateField(quantity.Value != nil, quantity.State, false); err != nil {
			return Allocation{}, fmt.Errorf("invalid_quantity_evidence")
		}
		quantity.State = cloneState(quantity.State)
		if quantity.Value != nil {
			value := quantity.Value.DeepCopy()
			quantity.Value = &value
		}
		out.Quantities[key] = quantity
	}
	return out, nil
}
func copyString(value *string) *string {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
