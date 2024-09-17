package nvidia

import (
	"vgpu/internal/provider/util"
)

const (
	HandshakeAnnos      = "hami.io/node-handshake"
	RegisterAnnos       = "hami.io/node-nvidia-register"
	NvidiaGPUDevice     = "NVIDIA"
	NvidiaGPUCommonWord = "GPU"
	GPUInUse            = "nvidia.com/use-gputype"
	GPUNoUse            = "nvidia.com/nouse-gputype"
	NumaBind            = "nvidia.com/numa-bind"
	NodeLockNvidia      = "hami.io/mutex.lock"
	// GPUUseUUID is user can use specify GPU device for set GPU UUID.
	GPUUseUUID = "nvidia.com/use-gpuuuid"
	// GPUNoUseUUID is user can not use specify GPU device for set GPU UUID.
	GPUNoUseUUID = "nvidia.com/nouse-gpuuuid"

	InRequestDevicesAnnos = "hami.io/vgpu-devices-to-allocate"
	SupportDevicesAnnos   = "hami.io/vgpu-devices-allocated"
)

func init() {
	util.InRequestDevices[NvidiaGPUDevice] = InRequestDevicesAnnos
	util.SupportDevices[NvidiaGPUDevice] = SupportDevicesAnnos
}
