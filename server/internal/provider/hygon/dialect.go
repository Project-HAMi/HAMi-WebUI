package hygon

import (
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

type inventoryDialect struct {
	hcu bool
}

// NewHCUDialect returns the InventoryDialect for Hygon HCU nodes.
func NewHCUDialect() provider.InventoryDialect {
	return inventoryDialect{hcu: true}
}

// NewDCUDialect returns the InventoryDialect for legacy Hygon DCU nodes.
func NewDCUDialect() provider.InventoryDialect {
	return inventoryDialect{hcu: false}
}

func (d inventoryDialect) Name() string {
	if d.hcu {
		return HygonHCUDevice
	}
	return HygonDCUDevice
}

func (d inventoryDialect) SelectorKey() string {
	return d.Name()
}

func (inventoryDialect) OwnsResource(_ string, _ *devicecatalog.Snapshot) bool {
	return false
}

func (d inventoryDialect) OwnsRegistrationAnnotation(key string, _ *devicecatalog.Snapshot) bool {
	if d.hcu {
		return key == HCURegisterAnnos
	}
	return key == RegisterAnnos
}

func (d inventoryDialect) ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	annoKey := RegisterAnnos
	priority := 0
	if d.hcu {
		annoKey = HCURegisterAnnos
		priority = 1
	}
	providerName := d.Name()
	return provider.DecodeAnnotationInventory(
		node,
		selected,
		providerName,
		annoKey,
		func(raw string) ([]util.RegistrationDevice, error) {
			return RegistrationFacts(raw, node.Name, d.hcu)
		},
		func(reg util.RegistrationDevice) resourceview.Device {
			dev := provider.NewRegistrationDevice(node, catalog, providerName, "hygon", providerName, reg, HygonDCUDevice)
			provider.SetPercentCompute(&dev, reg)
			if d.hcu {
				dev.Identity.DeviceID = reg.Device.AliasId
			}
			serial, _ := BoardSerial(providerName, dev.Identity.DeviceID)
			dev.Physical = resourceview.PhysicalAssociation{
				Domain:   "hygon-board",
				BoardKey: serial,
				Priority: priority,
			}
			if !d.hcu && reg.Device.AliasId != "" && reg.Device.AliasId != dev.Identity.DeviceID {
				dev.BindingAliases = append(dev.BindingAliases, reg.Device.AliasId)
			}
			return dev
		},
	)
}
