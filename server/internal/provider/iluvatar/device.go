package iluvatar

import "vgpu/internal/provider/util"

const (
	IluvatarBI150Device = "Iluvatar-BI-V150"
	IluvatarBI100Device = "Iluvatar-BI-V100"
	IluvatarMR100Device = "Iluvatar-MR-V100"

	BI150NodeRegisterAnno = "hami.io/node-BI-V150-register"
	MR100NodeRegisterAnno = "hami.io/node-MR-V100-register"
	BI100NodeRegisterAnno = "hami.io/node-BI-V100-register"
)

var (
	IluvatarResourceCount     string
	IluvatarResourceMemory    string
	IluvatarResourceCores     string
	IluvatarNodeRegisterAnnos []string
	IluvatarProviders         map[string]string
)

func init() {
	IluvatarProviders = map[string]string{
		"BI-V150": "Iluvatar",
		"BI-V100": "Iluvatar",
		"MR-V100": "Iluvatar",
	}
	IluvatarNodeRegisterAnnos = []string{BI150NodeRegisterAnno, MR100NodeRegisterAnno, BI100NodeRegisterAnno}
	util.InRequestDevices[IluvatarBI150Device] = "hami.io/BI-V150-devices-to-allocate"
	util.SupportDevices[IluvatarBI150Device] = "hami.io/BI-V150-devices-allocated"
	util.InRequestDevices[IluvatarBI100Device] = "hami.io/BI-V100-devices-to-allocate"
	util.SupportDevices[IluvatarBI100Device] = "hami.io/BI-V100-devices-allocated"
	util.InRequestDevices[IluvatarMR100Device] = "hami.io/MR-V100-devices-to-allocate"
	util.SupportDevices[IluvatarMR100Device] = "hami.io/MR-V100-devices-allocated"
}
