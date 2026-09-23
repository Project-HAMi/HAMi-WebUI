package nvidia

import (
	"encoding/json"
	"strings"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
)

// Operating modes HAMi's NVIDIA device plugin registers for a GPU.
const (
	ModeHamiCore = "hami-core"
	ModeMig      = "mig"
	// Registered, but the plugin only special-cases mig and its MPS setup is a
	// stub (plugin/mps.go, v2.9.0 to v2.10.0), so an mps GPU runs HAMi-core.
	modeMps = "mps"
)

// registeredDevices decodes a node's GPU registration, JSON or the older
// comma-separated form. A device whose MIG profiles do not decode keeps its
// other fields.
func registeredDevices(encoded string, logger *log.Helper) ([]*util.DeviceInfo, error) {
	var devices []*util.DeviceInfo
	if strings.HasPrefix(strings.TrimSpace(encoded), "[") {
		var registered []*util.NewDeviceInfo
		if err := json.Unmarshal([]byte(encoded), &registered); err != nil {
			return nil, err
		}
		devices = make([]*util.DeviceInfo, 0, len(registered))
		for _, entry := range registered {
			if entry == nil {
				continue
			}
			device := util.MapNewDeviceInfoToDeviceInfo(entry)
			profiles, err := util.ParseMigProfiles(entry.MIGProfiles)
			if err != nil {
				logger.Warnf("ignore MIG profiles of GPU %s: %v", entry.ID, err)
			}
			device.MigProfiles = profiles
			devices = append(devices, device)
		}
	} else {
		var err error
		if devices, err = util.DecodeNodeDevices(encoded, logger); err != nil {
			return nil, err
		}
	}
	for _, device := range devices {
		// Registrations older than device modes only shared GPUs through HAMi-core.
		if device.Mode == "" || device.Mode == modeMps {
			device.Mode = ModeHamiCore
		}
	}
	return devices, nil
}

// RegisteredModes maps each GPU on the node to the mode its plugin registered.
// It returns nil when the node or its registration cannot be read.
func RegisteredModes(node *corev1.Node, logger *log.Helper) map[string]string {
	if node == nil {
		return nil
	}
	encoded, ok := node.Annotations[RegisterAnnos]
	if !ok {
		return nil
	}
	devices, err := registeredDevices(encoded, logger)
	if err != nil {
		return nil
	}
	modes := make(map[string]string, len(devices))
	for _, device := range devices {
		modes[device.ID] = device.Mode
	}
	return modes
}
