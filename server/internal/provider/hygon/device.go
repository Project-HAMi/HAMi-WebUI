package hygon

import "vgpu/internal/provider/util"

const (
	HandshakeAnnos     = "hami.io/node-handshake-dcu"
	RegisterAnnos      = "hami.io/node-dcu-register"
	HygonDCUDevice     = "DCU"
	HygonDCUCommonWord = "DCU"
	DCUInUse           = "hygon.com/use-dcutype"
	DCUNoUse           = "hygon.com/nouse-dcutype"
	// DCUUseUUID is user can use specify DCU device for set DCU UUID.
	DCUUseUUID = "hygon.com/use-gpuuuid"
	// DCUNoUseUUID is user can not use specify DCU device for set DCU UUID.
	DCUNoUseUUID = "hygon.com/nouse-gpuuuid"
)

var (
	HygonResourceCount  string
	HygonResourceMemory string
	HygonResourceCores  string
)

func init() {
	util.InRequestDevices[HygonDCUDevice] = "hami.io/dcu-devices-to-allocate"
	util.SupportDevices[HygonDCUDevice] = "hami.io/dcu-devices-allocated"
}
