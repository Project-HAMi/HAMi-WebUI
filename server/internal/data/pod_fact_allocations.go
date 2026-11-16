package data

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/biz"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

func factKnown(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Known, Source: source, Scope: scope}
}
func factUnknown(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Unknown, Reason: "unverified", Source: source, Scope: scope}
}
func factQuantity(value int64, unit string) podfacts.Quantity {
	return podfacts.Quantity{Value: resource.NewQuantity(value, resource.DecimalSI), Unit: unit, State: factKnown("hami_annotation", "container")}
}
func factString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (r *podRepo) hamiFactEvidence(pod *corev1.Pod, node *corev1.Node, catalog *devicecatalog.Snapshot) (podfacts.Evidence, podfacts.Options) {
	pod = alignNvidiaAllocation(pod, catalog)
	result := podfacts.Evidence{}
	options := podfacts.Options{}
	for name := range r.schedulingResourceNames {
		options.ResourceNames = append(options.ResourceNames, name)
	}
	var models map[string]devicecatalog.AscendModel
	if catalog.Loaded() {
		models = catalog.Ascend
		for _, names := range catalog.ResourceNames {
			for _, name := range []string{names.Count, names.Memory, names.Compute} {
				if name != "" {
					options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(name))
				}
			}
		}
		if model := catalog.Cambricon; model != nil {
			for _, name := range []string{model.ResourceCountName, model.ResourceMemoryName, model.ResourceCoreName} {
				if name != "" {
					options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(name))
				}
			}
		}
	}
	for _, model := range models {
		for _, name := range []string{model.ResourceName, model.ResourceMemoryName, model.ResourceCoreName} {
			if name != "" {
				options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(name))
			}
		}
	}
	names := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, c := range pod.Spec.InitContainers {
		names = append(names, c.Name)
	}
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	decoded, failures := util.DecodePodDevices(pod, r.log)
	failed := map[string]bool{}
	for _, failure := range failures {
		failed[failure.DeviceType] = true
	}
	modes := map[string]string{}
	if node != nil {
		registrations, _ := nvidia.RegistrationFacts(node.Annotations[nvidia.RegisterAnnos], r.log)
		for _, registration := range registrations {
			if registration.Known["mode"] {
				modes[registration.Device.ID] = registration.Device.Mode
			}
		}
	}
	// A requested mode is not proof of the actual allocation mode. Keep the raw
	// value, but resolve actual mode only from registration or MIG allocation evidence.
	modeEvidence := pod.DeepCopy()
	delete(modeEvidence.Annotations, nvidia.VGPUModeAnnotation)
	gpuAllocations := nvidia.ReadAllocations(modeEvidence, modes)
	recognizedKeys := map[string]bool{}
	types := make([]string, 0, len(util.SupportDevices))
	for kind, key := range util.SupportDevices {
		if _, exists := pod.Annotations[key]; exists {
			types = append(types, kind)
		}
	}
	sort.Strings(types)
	for _, kind := range types {
		key := util.SupportDevices[kind]
		recognizedKeys[key] = true
		source := newFactAllocationSnapshot(pod, kind)
		source.HasAllocationEvidence = strings.Trim(pod.Annotations[key], ";:") != ""
		valid, complete := allocationLayout(pod.Annotations[key], len(names))
		source.Complete = complete
		countKey := ""
		if catalog.Loaded() {
			switch kind {
			case util.NvidiaGPUDevice:
				countKey = catalog.ResourceNames[kind].Count
			case util.CambriconGPUDevice:
				if catalog.Cambricon != nil {
					countKey = catalog.Cambricon.ResourceCountName
				}
			}
		}
		checkAllocationRows(&result, &source, pod, kind, countKey, pod.Annotations[key], valid && !failed[kind])
		// This is the source's concrete resource type, not an inferred Kubernetes
		// extended-resource name. E2b must establish that mapping independently.
		options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(kind))
		for slot, devices := range decoded[kind] {
			if slot >= len(names) {
				source.Complete = false
				break
			}
			for index, device := range devices {
				if kind == util.NvidiaGPUDevice && device.Type != util.NvidiaGPUDevice {
					source.Source.State, source.Source.Reason = resourceview.Unknown, "identity_conflict"
					source.Complete = false
				}
				record := podfacts.Allocation{Container: names[slot], RecordID: fmt.Sprintf("%d/%d", slot, index), Node: pod.Annotations[util.AssignedNodeAnnotations], Resource: kind, DeviceID: device.UUID, RawDeviceID: device.UUID, States: map[string]*viewv1.FieldState{}, Quantities: map[string]podfacts.Quantity{
					"memory": factQuantity(int64(device.Usedmem)*1024*1024, "bytes"), "compute": factQuantity(int64(device.RecordedCores), "percent"), "slots": factQuantity(1, "slots"),
				}}
				if kind == util.NvidiaGPUDevice {
					gpu := gpuAllocations.Device(slot, index, device.UUID)
					record.DeviceID = gpu.UUID
					record.RawMode = pod.Annotations[nvidia.VGPUModeAnnotation]
					if configured := modes[gpu.UUID]; configured != "" {
						record.ConfiguredMode = factString(configured)
						record.States["configuredMode"] = factKnown("node_registration", "device")
					}
					_, legacyMIG := nvidia.LegacyMigUUID(device.UUID)
					if gpu.Mode == nvidia.ModeMig && (gpu.Profile != "" || legacyMIG) {
						record.AllocationMode = factString("mig")
						record.States["allocationMode"] = factKnown("hami_annotation", "container")
					}
					if gpu.Mode == nvidia.ModeHamiCore && modes[device.UUID] == nvidia.ModeHamiCore {
						record.AllocationMode = factString("soft")
						record.States["allocationMode"] = factKnown("hami_annotation_and_node_registration", "container")
						unlimited := device.RecordedCores == 0
						record.ComputeUnlimited = &unlimited
						record.States["computeUnlimited"] = factKnown("hami_annotation_and_node_registration", "container")
						if unlimited {
							state := factUnknown("hami_annotation_and_node_registration", "container")
							state.Params = map[string]string{"unlimited": "true"}
							record.Quantities["compute"] = podfacts.Quantity{Unit: "percent", State: state}
						}
					}
					if gpu.Profile != "" {
						record.Template = factString(gpu.Profile)
						record.States["template"] = factKnown("hami_annotation", "container")
					}
				}
				if kind == util.MetaxSGPUDevice {
					record.DeviceID = ""
				} // requires separately verified inventory association, not telemetry arithmetic
				if node != nil {
					record.NodeUID = node.UID
				}
				source.Records = append(source.Records, record)
			}
		}
		result.Allocations = append(result.Allocations, source)
	}
	// Bind the existing decoder to the immutable catalog snapshot used by this
	// transaction, not whichever configuration happens to be latest mid-decode.
	decoder := ascend.Decoder{Catalog: devicecatalog.Static{Current: catalog}, Policy: r.ascend.Policy}
	npu, _ := decoder.Decode(pod, node)
	for _, word := range decoder.CommonWords(pod.Annotations) {
		recognizedKeys["hami.io/"+word+"-devices-allocated"] = true
		source := newFactAllocationSnapshot(pod, word)
		raw := pod.Annotations["hami.io/"+word+"-devices-allocated"]
		source.HasAllocationEvidence = strings.Trim(raw, ";:") != ""
		valid, complete := allocationLayout(raw, len(names))
		source.Complete = complete
		slots, decodedOK := npu[word]
		checkAllocationRows(&result, &source, pod, biz.AscendGPUDevice, models[word].ResourceName, raw, valid && decodedOK)
		options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(word))
		for slot, devices := range slots {
			if slot >= len(names) {
				source.Complete = false
				break
			}
			for index, device := range devices {
				facts := device.Facts
				record := podfacts.Allocation{Container: names[slot], RecordID: fmt.Sprintf("%d/%d", slot, index), Node: pod.Annotations[util.AssignedNodeAnnotations], Resource: word, DeviceID: device.UUID, RawDeviceID: device.UUID, RawMode: pod.Annotations[ascend.VNPUModeAnnotation], States: map[string]*viewv1.FieldState{}, Quantities: map[string]podfacts.Quantity{"memory": factQuantity(facts.Memory*1024*1024, "bytes"), "slots": factQuantity(1, "slots"), "compute": {State: factUnknown("hami_annotation", "container")}}}
				if facts.Template != "" {
					record.Template = factString(facts.Template)
					record.States["template"] = factKnown("hami_annotation", "container")
				}
				if node != nil {
					if configured := ascend.NodeSplitMode(node); configured != "" {
						record.ConfiguredMode = factString(configured)
						record.States["configuredMode"] = factKnown("node_registration", "device")
					}
				}
				applySafeAscendInterpretation(&record, catalog, facts, r.ascend.Policy)
				if node != nil {
					record.NodeUID = node.UID
				}
				source.Records = append(source.Records, record)
			}
		}
		result.Allocations = append(result.Allocations, source)
	}
	// A nonempty allocator annotation outside the supported decoder set is
	// evidence of an unrecognized allocation, never proof of a CPU-only Pod.
	for key, raw := range pod.Annotations {
		if recognizedKeys[key] || !strings.HasPrefix(key, "hami.io/") || !strings.HasSuffix(key, "-devices-allocated") || strings.Trim(raw, ";:") == "" {
			continue
		}
		kind := strings.TrimSuffix(strings.TrimPrefix(key, "hami.io/"), "-devices-allocated")
		source := newFactAllocationSnapshot(pod, kind)
		source.Source.State, source.Source.Reason = resourceview.Unknown, "version_unsupported"
		source.HasAllocationEvidence = true
		result.Allocations = append(result.Allocations, source)
	}
	sort.Slice(result.Allocations, func(i, j int) bool { return result.Allocations[i].Source.Source < result.Allocations[j].Source.Source })
	return result, options
}

// Empty CPU positions vary by HAMi version. A positive device request
// needs an allocation row at the corresponding container position.
func checkAllocationRows(e *podfacts.Evidence, source *podfacts.AllocationSnapshot, pod *corev1.Pod, provider, countKey, raw string, valid bool) {
	rows := strings.Split(raw, ";")
	for _, slot := range requestedSlots(pod, countKey) {
		if slot >= len(rows) || strings.Trim(rows[slot], ":") == "" {
			// The missing device cannot be scoped to a particular card or phase.
			e.UnreadableAllocations = append(e.UnreadableAllocations, podfacts.UnreadableAllocation{Provider: provider})
			break
		}
	}
	if !valid {
		e.UnreadableAllocations = append(e.UnreadableAllocations, unreadableAllocations(provider, raw)...)
		source.Source.State, source.Source.Reason = resourceview.Unknown, "unverified"
		source.Complete = false
	}
}

// The source is incomplete. Use its identifiers only to locate affected cards.
func unreadableAllocations(provider, raw string) []podfacts.UnreadableAllocation {
	if provider == util.MetaxGPUDevice || provider == util.MetaxSGPUDevice {
		provider = biz.MetaxGPUDevice
	}
	var hints []podfacts.UnreadableAllocation
	for _, record := range strings.FieldsFunc(raw, func(r rune) bool { return r == ':' || r == ';' }) {
		id, _, ok := strings.Cut(record, ",")
		if !ok || id == "" {
			return []podfacts.UnreadableAllocation{{Provider: provider}}
		}
		if provider == biz.NvidiaGPUDevice {
			if parent, legacy := nvidia.LegacyMigUUID(id); legacy {
				id = parent
			}
		}
		hints = append(hints, podfacts.UnreadableAllocation{Provider: provider, DeviceID: id})
	}
	return hints
}

func newFactAllocationSnapshot(pod *corev1.Pod, kind string) podfacts.AllocationSnapshot {
	source := &viewv1.SourceState{Source: "hami_annotation:" + kind, State: resourceview.Known}
	// Allocation context may precede binding. A conflicting bound node is not an
	// association we can repair using the Pod's requested/bound node.
	if assigned := pod.Annotations[util.AssignedNodeAnnotations]; assigned == "" || (pod.Spec.NodeName != "" && pod.Spec.NodeName != assigned) {
		source.State, source.Reason = resourceview.Unknown, "identity_conflict"
	}
	return podfacts.AllocationSnapshot{PodUID: pod.UID, ResourceVersion: pod.ResourceVersion, Source: source, Accounting: podfacts.AccountingPodPeak}
}

// Existing decoders intentionally tolerate truncated slot streams and Ascend
// skips some malformed segments. Such a read cannot prove source completeness.
func allocationLayout(raw string, count int) (valid, complete bool) {
	slots := strings.Split(raw, ";")
	if len(slots) > count && slots[len(slots)-1] == "" {
		slots = slots[:len(slots)-1]
	}
	complete = len(slots) == count
	valid = len(slots) <= count
	for _, slot := range slots {
		for _, entry := range strings.Split(slot, ":") {
			if entry == "" {
				continue
			}
			fields := strings.Split(entry, ",")
			if len(fields) < 4 || fields[0] == "" || fields[1] == "" {
				return false, false
			}
			for _, field := range fields[2:4] {
				v, err := strconv.ParseInt(field, 10, 32)
				if err != nil || v < 0 {
					return false, false
				}
			}
		}
	}
	return valid, complete
}
func applySafeAscendInterpretation(record *podfacts.Allocation, catalog *devicecatalog.Snapshot, facts ascend.Facts, policy string) {
	raw := record.RawMode
	// V6a: the legacy ResolveMode treats arbitrary nonempty modes as template.
	// Preserve ENPU/other aliases as raw evidence until their semantics are proven.
	if raw != "" && raw != ascend.VNPUModeHamiCore && raw != ascend.ModeTemplate {
		return
	}
	if facts.Mode == ascend.ModeSoft && facts.Template != "" {
		return
	}
	model, configured := catalog.AscendModel(facts.CommonWord)
	wholeCapacity := facts.Memory > 0 && ((configured && facts.Memory == model.MemoryAllocatable) || facts.Memory == facts.CardMemory)
	if facts.Template == "" && facts.Mode != ascend.ModeSoft && !wholeCapacity {
		return
	}
	result := ascend.Interpret(catalog, facts, policy)
	if result.Shape != ascend.ShapeUnknown && result.Shape != "" {
		record.AllocationMode = factString(result.Shape)
		record.States["allocationMode"] = factKnown("hami_annotation", "container")
	}
	if !result.Known {
		return
	}
	if result.Shape != ascend.ShapeTemplate {
		record.Quantities["compute"] = factQuantity(int64(result.Cores), "percent")
		return
	}
	template, ok := model.Template(result.Template)
	if !ok || model.AICore <= 0 || template.AICore <= 0 {
		return
	}
	aiCores := factQuantity(int64(template.AICore), "ai_cores")
	aiCores.State = factKnown("device_catalog", "container")
	record.Quantities["aiCores"] = aiCores
	// The legacy percentage is rounded to an integer. Retain exact physical
	// AI Core counts and emit a percentage only if Quantity can represent it exactly.
	ratio := big.NewRat(int64(template.AICore)*100, int64(model.AICore))
	decimal := ratio.FloatString(9)
	exact, ok := new(big.Rat).SetString(decimal)
	if !ok || exact.Cmp(ratio) != 0 {
		return
	}
	value, err := resource.ParseQuantity(decimal)
	if err != nil {
		return
	}
	record.Quantities["compute"] = podfacts.Quantity{Value: &value, Unit: "percent", State: factKnown("device_catalog", "container")}

}
