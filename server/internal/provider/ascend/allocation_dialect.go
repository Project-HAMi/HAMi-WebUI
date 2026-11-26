package ascend

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
)

func (d inventoryDialect) ExtractPodAllocations(ctx provider.PodAllocationContext) provider.PodAllocationResult {
	if ctx.Pod == nil {
		return provider.PodAllocationResult{}
	}
	// Bind the existing decoder to the immutable catalog snapshot used by this
	// transaction, not whichever configuration happens to be latest mid-decode.
	decoder := Decoder{Catalog: devicecatalog.Static{Current: ctx.Catalog}, Policy: d.annotationlessPolicy}
	words := decoder.CommonWords(ctx.Pod.Annotations)
	if len(words) == 0 {
		return provider.PodAllocationResult{}
	}
	npu, _ := decoder.Decode(ctx.Pod, ctx.Node)
	var models map[string]devicecatalog.AscendModel
	if ctx.Catalog.Loaded() {
		models = ctx.Catalog.Ascend
	}

	var out provider.PodAllocationResult
	nodeName := ctx.Pod.Annotations[provider.AssignedNodeAnnotation]
	configuredMode := ""
	if ctx.Node != nil {
		configuredMode = NodeSplitMode(ctx.Node)
	}
	for _, word := range words {
		key := allocationKey(word)
		raw := ctx.Pod.Annotations[key]
		out.RecognizedAnnotationKeys = append(out.RecognizedAnnotationKeys, key)
		source := provider.NewFactAllocationSnapshot(ctx.Pod, word)
		source.HasAllocationEvidence = strings.Trim(raw, ";:") != ""
		valid, complete := provider.AllocationLayout(raw, len(ctx.ContainerNames))
		source.Complete = complete
		slots, decodedOK := npu[word]
		unreadable := provider.CheckAllocationRows(ctx.Pod, &source, AscendDevice, models[word].ResourceName, raw, valid && decodedOK, nil)
		out.UnreadableAllocations = append(out.UnreadableAllocations, unreadable...)
		out.ResourceNames = append(out.ResourceNames, corev1.ResourceName(word))

		for slot, devices := range slots {
			if slot >= len(ctx.ContainerNames) {
				source.Complete = false
				break
			}
			for index, device := range devices {
				facts := device.Facts
				record := podfacts.Allocation{
					Container:   ctx.ContainerNames[slot],
					RecordID:    fmt.Sprintf("%d/%d", slot, index),
					Node:        nodeName,
					Resource:    word,
					DeviceID:    device.UUID,
					RawDeviceID: device.UUID,
					RawMode:     ctx.Pod.Annotations[VNPUModeAnnotation],
					States:      map[string]*viewv1.FieldState{},
					Quantities: map[string]podfacts.Quantity{
						"memory":  provider.FactQuantity(facts.Memory*1024*1024, "bytes"),
						"slots":   provider.FactQuantity(1, "slots"),
						"compute": {State: provider.UnknownState("hami_annotation", "container", "unverified")},
					},
				}
				if facts.Template != "" {
					record.Template = provider.FactString(facts.Template)
					record.States["template"] = provider.KnownState("hami_annotation", "container")
				}
				if configuredMode != "" {
					record.ConfiguredMode = provider.FactString(configuredMode)
					record.States["configuredMode"] = provider.KnownState("node_registration", "device")
				}
				applySafeAscendInterpretation(&record, ctx.Catalog, facts, d.annotationlessPolicy)
				if ctx.Node != nil {
					record.NodeUID = ctx.Node.UID
				}
				source.Records = append(source.Records, record)
			}
		}
		out.Allocations = append(out.Allocations, source)
	}
	return out
}

func applySafeAscendInterpretation(record *podfacts.Allocation, catalog *devicecatalog.Snapshot, facts Facts, policy string) {
	raw := record.RawMode
	// The legacy ResolveMode treats arbitrary nonempty modes as template.
	// Preserve ENPU/other aliases as raw evidence until their semantics are proven.
	if raw != "" && raw != VNPUModeHamiCore && raw != ModeTemplate {
		return
	}
	if facts.Mode == ModeSoft && facts.Template != "" {
		return
	}
	model, configured := catalog.AscendModel(facts.CommonWord)
	wholeCapacity := facts.Memory > 0 && ((configured && facts.Memory == model.MemoryAllocatable) || facts.Memory == facts.CardMemory)
	if facts.Template == "" && facts.Mode != ModeSoft && !wholeCapacity {
		return
	}
	result := Interpret(catalog, facts, policy)
	if result.Shape != ShapeUnknown && result.Shape != "" {
		record.AllocationMode = provider.FactString(result.Shape)
		record.States["allocationMode"] = provider.KnownState("hami_annotation", "container")
	}
	if !result.Known {
		return
	}
	if result.Shape != ShapeTemplate {
		record.Quantities["compute"] = provider.FactQuantity(int64(result.Cores), "percent")
		return
	}
	template, ok := model.Template(result.Template)
	if !ok || model.AICore <= 0 || template.AICore <= 0 {
		return
	}
	aiCores := provider.FactQuantity(int64(template.AICore), "ai_cores")
	aiCores.State = provider.KnownState("device_catalog", "container")
	record.Quantities["aiCores"] = aiCores
	// The legacy percentage is rounded to an integer. Retain exact physical
	// AI Core counts and emit a percentage only if Quantity can represent it exactly.
	if q, ok := provider.ExactRatQuantity(int64(template.AICore)*100, int64(model.AICore), "percent", "device_catalog", "container"); ok {
		record.Quantities["compute"] = q
	}
}
