package amd

import "vgpu/internal/provider/util"

const (
	// RegisterAnnos is the node annotation amd-device-plugin registers its GPUs in.
	RegisterAnnos = "hami.io/node-amd-register"
	// AMDGPUDevice is the device type HAMi's AMD backend uses.
	AMDGPUDevice = util.AMDGPUDevice
)

func init() {
	util.InRequestDevices[AMDGPUDevice] = "hami.io/amd-devices-to-allocate"
	util.SupportDevices[AMDGPUDevice] = "hami.io/amd-devices-allocated"
}
