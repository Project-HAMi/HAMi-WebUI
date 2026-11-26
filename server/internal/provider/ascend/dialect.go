package ascend

import (
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

type inventoryDialect struct{}

// NewDialect returns the InventoryDialect for Huawei Ascend NPU nodes.
func NewDialect() provider.InventoryDialect {
	return inventoryDialect{}
}

func (inventoryDialect) Name() string        { return AscendDevice }
func (inventoryDialect) SelectorKey() string { return AscendDevice }

func (inventoryDialect) OwnsResource(name string, catalog *devicecatalog.Snapshot) bool {
	if catalog == nil {
		return false
	}
	_, _, ok := catalog.AscendResource(name)
	return ok
}

func (inventoryDialect) OwnsRegistrationAnnotation(key string, catalog *devicecatalog.Snapshot) bool {
	if strings.HasPrefix(key, "hami.io/node-register-Ascend") {
		return true
	}
	if catalog == nil {
		return false
	}
	for _, word := range catalog.AscendCommonWords() {
		if key == "hami.io/node-register-"+word {
			return true
		}
	}
	return false
}

func (d inventoryDialect) ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	if !selected || node == nil {
		return provider.NodeInventory{}
	}
	present := false
	for key := range node.Annotations {
		if d.OwnsRegistrationAnnotation(key, catalog) {
			present = true
			break
		}
	}
	if !present {
		return provider.NodeInventory{}
	}
	source := &viewv1.SourceState{Source: "hami_registration:" + AscendDevice, State: resourceview.Known}
	regs, err := RegistrationFacts(node, catalog)
	if err != nil {
		source.State, source.Reason = resourceview.Unknown, "unverified"
	}
	devices := make([]resourceview.Device, 0, len(regs))
	for _, reg := range regs {
		dev := provider.NewRegistrationDevice(node, catalog, AscendDevice, "huawei", "", reg)
		if reg.Device.Type != "" {
			dev.AllocationSource = reg.Device.Type
		}
		model, exists := catalog.AscendModel(reg.Device.Type)
		if catalog.Loaded() {
			provider.ApplyCatalogResourceNames(&dev, node, exists, devicecatalog.DeviceResourceNames{
				Count:   model.ResourceName,
				Memory:  model.ResourceMemoryName,
				Compute: model.ResourceCoreName,
			})
		}
		if reg.Device.Mode == VNPUModeHamiCore {
			provider.SetPercentCompute(&dev, reg)
		} else {
			dev.ComputeCapacityShare = provider.UnknownQuantity("share", "node_registration_and_device_config", "device", "unverified")
			if exists && reg.Device.Mode == ModeTemplate && reg.Known["devcore"] && model.AICore > 0 {
				if q, ok := provider.ExactRatQuantity(int64(reg.Device.Devcore), int64(model.AICore), "share", "node_registration_and_device_config", "device"); ok {
					dev.ComputeCapacityShare = q
				}
			}
		}
		if reg.Device.Type == "Ascend910C" {
			dev.Kind = "unknown"
			dev.States["kind"] = provider.UnknownState("node_registration", "device", "unverified")
			dev.States["kind"].Params = map[string]string{
				"identityLevel": "npu_module",
				"missing":       "physical_card_association",
			}
		}
		devices = append(devices, dev)
	}
	return provider.NodeInventory{
		Devices: devices,
		Sources: []*viewv1.SourceState{source},
	}
}
