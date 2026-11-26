package metax

import (
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

const metaxProviderName = "Metax"

type sgpuDialect struct{}

// NewSGPUDialect returns the InventoryDialect for MetaX sGPU nodes.
func NewSGPUDialect() provider.InventoryDialect {
	return sgpuDialect{}
}

// NewAllocationDialect returns the AllocationDialect for MetaX GPU and sGPU Pods.
func NewAllocationDialect() provider.AllocationDialect {
	return sgpuDialect{}
}

func (sgpuDialect) Name() string        { return metaxProviderName }
func (sgpuDialect) SelectorKey() string { return metaxProviderName }

func (sgpuDialect) CatalogResourceNames(_ *devicecatalog.Snapshot) []corev1.ResourceName {
	return nil
}

func (sgpuDialect) ExtractPodAllocations(ctx provider.PodAllocationContext) provider.PodAllocationResult {
	var merged provider.PodAllocationResult
	specs := []struct {
		kind          string
		fallbackKey   string
		clearDeviceID bool
	}{
		{kind: MetaxGPUDevice, fallbackKey: "hami.io/metax-gpu-devices-allocated"},
		{kind: MetaxSGPUDevice, fallbackKey: "hami.io/metax-sgpu-devices-allocated", clearDeviceID: true},
	}
	for _, spec := range specs {
		key := util.SupportDevices[spec.kind]
		if key == "" {
			key = spec.fallbackKey
		}
		var enrich func(source *podfacts.AllocationSnapshot, record *podfacts.Allocation, slot, index int, device util.ContainerDevice)
		if spec.clearDeviceID {
			enrich = func(_ *podfacts.AllocationSnapshot, record *podfacts.Allocation, _, _ int, _ util.ContainerDevice) {
				// Requires separately verified inventory association, not telemetry arithmetic.
				record.DeviceID = ""
			}
		}
		part := provider.DecodeStandardPodAllocation(ctx, provider.StandardAllocationSpec{
			Kind:               spec.kind,
			AnnotationKey:      key,
			UnreadableProvider: metaxProviderName,
			DecodeContainer: func(rawSlot, _ string) (util.ContainerDevices, error) {
				return util.DecodeMetaxContainerDevices(rawSlot)
			},
			EnrichRecord: enrich,
		})
		merged.Allocations = append(merged.Allocations, part.Allocations...)
		merged.UnreadableAllocations = append(merged.UnreadableAllocations, part.UnreadableAllocations...)
		merged.RecognizedAnnotationKeys = append(merged.RecognizedAnnotationKeys, part.RecognizedAnnotationKeys...)
		merged.ResourceNames = append(merged.ResourceNames, part.ResourceNames...)
	}
	return merged
}

func (sgpuDialect) OwnsResource(_ string, _ *devicecatalog.Snapshot) bool {
	return false
}

func (sgpuDialect) OwnsRegistrationAnnotation(key string, _ *devicecatalog.Snapshot) bool {
	return key == MetaxSDeviceAnno
}

func (sgpuDialect) ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	return provider.DecodeAnnotationInventory(
		node,
		selected,
		metaxProviderName,
		MetaxSDeviceAnno,
		SGPURegistrationFacts,
		func(reg util.RegistrationDevice) resourceview.Device {
			dev := provider.NewRegistrationDevice(node, catalog, metaxProviderName, "metax", MetaxSGPUDevice, reg)
			uuid := reg.Device.ID
			dev.UUID = &uuid
			dev.States["uuid"] = provider.KnownState("node_registration", "device")
			dev.AllocationSource = MetaxSGPUDevice
			dev.RequireVerifiedRawBinding = true
			provider.SetPercentCompute(&dev, reg)
			return dev
		},
	)
}
