package nvidia

import (
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

func allocationAnnotationKey() string {
	if key := util.SupportDevices[NvidiaGPUDevice]; key != "" {
		return key
	}
	return SupportDevicesAnnos
}

// AlignPodAllocation restores omitted CPU-only slots in model-specific NVIDIA
// HAMi v2.10 allocations when configured GPU requests account for every row.
// The input informer Pod object stays untouched.
func AlignPodAllocation(pod *corev1.Pod, catalog *devicecatalog.Snapshot) *corev1.Pod {
	if pod == nil {
		return nil
	}
	key := allocationAnnotationKey()
	raw := pod.Annotations[key]
	n := len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	if !catalog.Loaded() || n < 2 || raw == "" {
		return pod
	}
	raw, terminated := strings.CutSuffix(raw, ";")
	if !terminated || strings.Count(raw, ";")+1 >= n {
		return pod
	}
	rows := strings.Split(raw, ";")
	slots := provider.RequestedSlots(pod, catalog.ResourceNames[NvidiaGPUDevice].Count)
	if len(rows) != len(slots) || slices.Contains(rows, "") {
		return pod
	}
	aligned := make([]string, n)
	for i, slot := range slots {
		aligned[slot] = rows[i]
	}
	cloned := *pod
	cloned.Annotations = maps.Clone(pod.Annotations)
	cloned.Annotations[key] = strings.Join(aligned, ";") + ";"
	return &cloned
}

// NewAllocationDialect returns the AllocationDialect for NVIDIA GPU Pods.
func NewAllocationDialect() provider.AllocationDialect {
	return inventoryDialect{}
}

func (inventoryDialect) CatalogResourceNames(_ *devicecatalog.Snapshot) []corev1.ResourceName {
	return nil
}

func (inventoryDialect) ExtractPodAllocations(ctx provider.PodAllocationContext) provider.PodAllocationResult {
	if ctx.Pod == nil {
		return provider.PodAllocationResult{}
	}
	key := allocationAnnotationKey()
	if _, exists := ctx.Pod.Annotations[key]; !exists {
		return provider.PodAllocationResult{}
	}
	pod := AlignPodAllocation(ctx.Pod, ctx.Catalog)
	ctx.Pod = pod

	countKey := ""
	if ctx.Catalog.Loaded() {
		countKey = ctx.Catalog.ResourceNames[NvidiaGPUDevice].Count
	}
	modes := map[string]string{}
	if ctx.Node != nil {
		registrations, _ := RegistrationFacts(ctx.Node.Annotations[RegisterAnnos], ctx.Logger)
		for _, registration := range registrations {
			if registration.Known["mode"] {
				modes[registration.Device.ID] = registration.Device.Mode
			}
		}
	}
	// A requested mode is not proof of the actual allocation mode. Keep the raw
	// value, but resolve actual mode only from registration or MIG allocation evidence.
	modeEvidence := pod.DeepCopy()
	delete(modeEvidence.Annotations, VGPUModeAnnotation)
	gpuAllocations := ReadAllocations(modeEvidence, modes)

	return provider.DecodeStandardPodAllocation(ctx, provider.StandardAllocationSpec{
		Kind:          NvidiaGPUDevice,
		AnnotationKey: key,
		CountResource: countKey,
		NormalizeID: func(id string) string {
			if parent, legacy := LegacyMigUUID(id); legacy {
				return parent
			}
			return id
		},
		EnrichRecord: func(source *podfacts.AllocationSnapshot, record *podfacts.Allocation, slot, index int, device util.ContainerDevice) {
			if device.Type != NvidiaGPUDevice {
				source.Source.State, source.Source.Reason = resourceview.Unknown, "identity_conflict"
				source.Complete = false
			}
			gpu := gpuAllocations.Device(slot, index, device.UUID)
			record.DeviceID = gpu.UUID
			record.RawMode = pod.Annotations[VGPUModeAnnotation]
			if configured := modes[gpu.UUID]; configured != "" {
				record.ConfiguredMode = provider.FactString(configured)
				record.States["configuredMode"] = provider.KnownState("node_registration", "device")
			}
			_, legacyMIG := LegacyMigUUID(device.UUID)
			if gpu.Mode == ModeMig && (gpu.Profile != "" || legacyMIG) {
				record.AllocationMode = provider.FactString("mig")
				record.States["allocationMode"] = provider.KnownState("hami_annotation", "container")
			}
			if gpu.Mode == ModeHamiCore && modes[device.UUID] == ModeHamiCore {
				record.AllocationMode = provider.FactString("soft")
				record.States["allocationMode"] = provider.KnownState("hami_annotation_and_node_registration", "container")
				unlimited := device.RecordedCores == 0
				record.ComputeUnlimited = &unlimited
				record.States["computeUnlimited"] = provider.KnownState("hami_annotation_and_node_registration", "container")
				if unlimited {
					state := provider.UnknownState("hami_annotation_and_node_registration", "container", "unverified")
					state.Params = map[string]string{"unlimited": "true"}
					record.Quantities["compute"] = podfacts.Quantity{Unit: "percent", State: state}
				}
			}
			if gpu.Profile != "" {
				record.Template = provider.FactString(gpu.Profile)
				record.States["template"] = provider.KnownState("hami_annotation", "container")
			}
		},
	})
}
