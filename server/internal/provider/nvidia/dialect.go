package nvidia

import (
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

type inventoryDialect struct{}

// NewInventoryDialect returns the InventoryDialect for NVIDIA GPU nodes.
func NewInventoryDialect() provider.InventoryDialect {
	return inventoryDialect{}
}

func (inventoryDialect) Name() string        { return NvidiaGPUDevice }
func (inventoryDialect) SelectorKey() string { return NvidiaGPUDevice }

func (inventoryDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	return name == "nvidia.com/gpu" || name == "nvidia.com/gpu.shared" || strings.HasPrefix(name, "nvidia.com/mig-")
}

func (inventoryDialect) OwnsRegistrationAnnotation(key string, _ *devicecatalog.Snapshot) bool {
	return key == RegisterAnnos
}

func (inventoryDialect) ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, logger *log.Helper, _ time.Time) provider.NodeInventory {
	return provider.DecodeAnnotationInventory(
		node,
		selected,
		NvidiaGPUDevice,
		RegisterAnnos,
		func(raw string) ([]util.RegistrationDevice, error) {
			return RegistrationFacts(raw, logger)
		},
		func(reg util.RegistrationDevice) resourceview.Device {
			dev := provider.NewRegistrationDevice(node, catalog, NvidiaGPUDevice, "nvidia", NvidiaGPUDevice, reg)
			uuid := reg.Device.ID
			dev.UUID = &uuid
			dev.States["uuid"] = provider.KnownState("node_registration", "device")
			if dev.Mode != ModeHamiCore && dev.Mode != ModeMig {
				dev.Mode = ""
				dev.States["mode"] = provider.UnknownState("node_registration", "device", "unverified")
				dev.States["mode"].Params = map[string]string{"raw": reg.RawMode}
			}
			provider.SetPercentCompute(&dev, reg)
			return dev
		},
	)
}
