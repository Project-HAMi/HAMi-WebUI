package metax

import (
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
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

func (sgpuDialect) Name() string        { return metaxProviderName }
func (sgpuDialect) SelectorKey() string { return metaxProviderName }

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
