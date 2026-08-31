package ascend

import "vgpu/internal/provider/util"

const (
	AscendDevice          = "Ascend"
	Ascend310PDevice      = "Ascend310P"
	AscendDeviceSelection = "huawei.com/predicate-ascend-idx-"
	// IluvatarUseUUID is user can use specify Iluvatar device for set Iluvatar UUID.
	AscendDeviceUseUUID = "huawei.com/use-ascenduuid"
	// IluvatarNoUseUUID is user can not use specify Iluvatar device for set Iluvatar UUID.
	AscendNoUseUUID            = "huawei.com/nouse-ascenduuid"
	Ascend910BNodeRegisterAnno = "hami.io/node-register-Ascend910B"
	Ascend310PNodeRegisterAnno = "hami.io/node-register-Ascend310P"
)

var (
	AscendResourceCount  string
	AscendResourceMemory string
	AscendResourceCores  string
)

func init() {
	util.InRequestDevices[AscendDevice] = "hami.io/Ascend910B-devices-to-allocate"
	util.SupportDevices[AscendDevice] = "hami.io/Ascend910B-devices-allocated"
	util.InRequestDevices[Ascend310PDevice] = "hami.io/Ascend310P-devices-to-allocate"
	util.SupportDevices[Ascend310PDevice] = "hami.io/Ascend310P-devices-allocated"
}
