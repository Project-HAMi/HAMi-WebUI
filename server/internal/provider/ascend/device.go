package ascend

import "vgpu/internal/provider/util"

const (
	AscendDevice          = "Ascend"
	AscendDeviceSelection = "huawei.com/predicate-ascend-idx-"
	// IluvatarUseUUID is user can use specify Iluvatar device for set Iluvatar UUID.
	AscendDeviceUseUUID = "huawei.com/use-ascenduuid"
	// IluvatarNoUseUUID is user can not use specify Iluvatar device for set Iluvatar UUID.
	AscendNoUseUUID         = "huawei.com/nouse-ascenduuid"
	AscendResourceCoreCount = "huawei.com/Ascend910"
)

var (
	AscendResourceCount  string
	AscendResourceMemory string
	AscendResourceCores  string
)

func init() {
	util.InRequestDevices[AscendDevice] = "hami.io/ascend-devices-to-allocate"
	util.SupportDevices[AscendDevice] = "hami.io/ascend-devices-allocated"
}
