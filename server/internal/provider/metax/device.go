// Copyright (c) 2025 MetaX Integrated Circuits (Shanghai) Co., Ltd.  All rights reserved.
package metax

import "vgpu/internal/provider/util"

const (
	DeviceAvaliable = 1

	MetaxGPUDevice  = "Metax-GPU"
	MetaxSGPUDevice = "Metax-SGPU"

	MeteaxDevice  = "metax-tech.com/gpu"
	MeteaxSDevice = "metax-tech.com/sgpu"

	MetaxSDeviceAnno = "metax-tech.com/node-gpu-devices"
)

func init() {
	util.InRequestDevices[MetaxGPUDevice] = "hami.io/metax-gpu-devices-to-allocate"
	util.SupportDevices[MetaxGPUDevice] = "hami.io/metax-gpu-devices-allocated"

	util.InRequestDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-to-allocate"
	util.SupportDevices[MetaxSGPUDevice] = "hami.io/metax-sgpu-devices-allocated"
}
