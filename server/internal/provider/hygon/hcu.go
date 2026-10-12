package hygon

import (
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"vgpu/internal/provider/util"
)

// HCU consumes the HAMi-mode registration of k8s-hcu-device-plugin. Legacy
// DCU registration uses a different device identity and remains in Hygon.
type HCU struct {
	log           *log.Helper
	labelSelector string
}

func NewHCU(logger *log.Helper, labelSelector string) *HCU {
	if labelSelector == "" {
		// Configs written before HCU support lack this key. An empty selector
		// would list every node as an accelerator node, so use the vendor label.
		labelSelector = "hcu=on"
	}
	return &HCU{log: logger, labelSelector: labelSelector}
}

func (h *HCU) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(h.labelSelector)
}

func (h *HCU) GetProvider() string {
	return HygonHCUDevice
}

func (h *HCU) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	encoded, ok := node.Annotations[HCURegisterAnnos]
	if !ok {
		return []*util.DeviceInfo{}, nil
	}
	devices, err := util.DecodeNodeDevices(encoded, h.log)
	if err != nil {
		return nil, err
	}
	valid := make([]*util.DeviceInfo, 0, len(devices))
	for _, device := range devices {
		// As for DCU, the registered hami-core is a format default.
		device.Mode = ""
		// The plugin prefixes the physical serial with HCU- in both Node and
		// Pod annotations. hcu-exporter exposes that same serial as device_id;
		// unlike legacy DCU, this is not a node-local minor number.
		serial, ok := strings.CutPrefix(device.ID, "HCU-")
		if !ok || serial == "" {
			// The plugin registers "HCU-" + serial even when the serial read
			// fails. Such a device cannot be joined to telemetry or told apart
			// from another failed read, so drop only that device.
			h.log.Warnf("skip HCU device with invalid registration ID %q on node %s", device.ID, node.Name)
			continue
		}
		device.AliasId = device.ID
		device.ID = serial
		valid = append(valid, device)
	}
	return valid, nil
}
