package ascend

import (
	"fmt"

	"vgpu/internal/provider/util"
)

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
	AscendResourceCount     string
	AscendResourceMemory    string
	AscendResourceCores     string
	AscendNodeRegisterAnnos []string
)

// ascendCommonWords lists the HAMi commonWords of the Ascend chip variants
// this build recognizes, in addition to the legacy Ascend910B/Ascend310P
// constants above. HAMi derives every Ascend annotation from the commonWord:
// node side "hami.io/node-register-<commonWord>", pod side
// "hami.io/<commonWord>-devices-to-allocate" / "-allocated".
var ascendCommonWords = []string{
	"Ascend910A",
	"Ascend910B2",
	"Ascend910B3",
	"Ascend910B4",
	"Ascend910B4-1",
	"Ascend910C",
}

func init() {
	AscendNodeRegisterAnnos = []string{Ascend910BNodeRegisterAnno, Ascend310PNodeRegisterAnno}
	util.InRequestDevices[AscendDevice] = "hami.io/Ascend910B-devices-to-allocate"
	util.SupportDevices[AscendDevice] = "hami.io/Ascend910B-devices-allocated"
	util.InRequestDevices[Ascend310PDevice] = "hami.io/Ascend310P-devices-to-allocate"
	util.SupportDevices[Ascend310PDevice] = "hami.io/Ascend310P-devices-allocated"
	for _, cw := range ascendCommonWords {
		AscendNodeRegisterAnnos = append(AscendNodeRegisterAnnos, fmt.Sprintf("hami.io/node-register-%s", cw))
		util.InRequestDevices[cw] = fmt.Sprintf("hami.io/%s-devices-to-allocate", cw)
		util.SupportDevices[cw] = fmt.Sprintf("hami.io/%s-devices-allocated", cw)
	}
}
