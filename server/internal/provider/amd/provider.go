package amd

import (
	"encoding/json"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"vgpu/internal/provider/util"
)

// AMD reads the registration of amd-device-plugin. Its telemetry comes from the
// plugin's k8s-vgpu-monitor, which uses the metric names of the NVIDIA monitor and
// labels every series with the same device UUID the registration carries.
type AMD struct {
	log           *log.Helper
	labelSelector string
}

func NewAMD(logger *log.Helper, labelSelector string) *AMD {
	if labelSelector == "" {
		// Configs written before AMD support lack this key. An empty selector
		// would list every node as an accelerator node, so use the label the
		// AMD node labeller sets.
		labelSelector = "amd.com/gpu.family"
	}
	return &AMD{log: logger, labelSelector: labelSelector}
}

func (a *AMD) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(a.labelSelector)
}

func (a *AMD) GetProvider() string {
	return AMDGPUDevice
}

func (a *AMD) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	encoded, ok := node.Annotations[RegisterAnnos]
	if !ok {
		return []*util.DeviceInfo{}, nil
	}
	var registered []*util.NewDeviceInfo
	if err := json.Unmarshal([]byte(encoded), &registered); err != nil {
		return nil, err
	}
	devices := make([]*util.DeviceInfo, 0, len(registered))
	for _, entry := range registered {
		if entry == nil {
			continue
		}
		device := util.MapNewDeviceInfoToDeviceInfo(entry)
		// Registered for the HAMi format only; AMD has no MIG-like modes.
		device.Mode = ""
		devices = append(devices, device)
	}
	return devices, nil
}
