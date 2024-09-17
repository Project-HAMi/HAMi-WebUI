package mlu

import (
	"vgpu/internal/provider/util"
)

const (
	CambriconMLUDevice     = "MLU"
	CambriconMLUCommonWord = "MLU"
	MluMemSplitLimit       = "CAMBRICON_SPLIT_MEMS"
	MluMemSplitIndex       = "CAMBRICON_SPLIT_VISIBLE_DEVICES"
	MluMemSplitEnable      = "CAMBRICON_SPLIT_ENABLE"
	MLUInUse               = "cambricon.com/use-mlutype"
	MLUNoUse               = "cambricon.com/nouse-mlutype"
	// MLUUseUUID is user can use specify MLU device for set MLU UUID.
	MLUUseUUID = "cambricon.com/use-gpuuuid"
	// MLUNoUseUUID is user can not use specify MLU device for set MLU UUID.
	MLUNoUseUUID            = "cambricon.com/nouse-gpuuuid"
	DsmluLockTime           = "cambricon.com/dsmlu.lock"
	DsmluProfile            = "CAMBRICON_DSMLU_PROFILE"
	DsmluResourceAssigned   = "CAMBRICON_DSMLU_ASSIGHED"
	retry                   = 5
	DsmluProfileAndInstance = "CAMBRICON_DSMLU_PROFILE_INSTANCE"

	CambriconDeviceAnnosOnNode = "cambricon.com/mlu"
	CambriconDeviceAnnosOnPod  = "cambricon.com/smlu"
	CambriconDeviceCoreAnnos   = "cambricon.com/mlu370.smlu.vcore"
	CambriconDeviceMemAnnos    = "cambricon.com/mlu370.smlu.vmemory"
	CambriconDeviceRealCount   = "cambricon.com/real-mlu-counts"
	CambriconMemUnit           = 1024
)

func init() {
	util.SupportDevices[CambriconMLUDevice] = DsmluProfile
}
