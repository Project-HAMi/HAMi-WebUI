/*
Copyright 2024 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

const (
	AssignedNodeAnnotations = "hami.io/vgpu-node"

	ResourcePoolAnnotations = "hami.io/resource-pool"
	FlavorAnnotations       = "hami.io/flavor"
)

type DevicePluginConfigs struct {
	Nodeconfig []struct {
		Name                string  `json:"name"`
		Devicememoryscaling float64 `json:"devicememoryscaling"`
		Devicecorescaling   float64 `json:"devicecorescaling"`
		Devicesplitcount    uint    `json:"devicesplitcount"`
		Migstrategy         string  `json:"migstrategy"`
	} `json:"nodeconfig"`
}

type DeviceConfig struct {
	ResourceName *string
	DebugMode    *bool
}

var (
	DebugMode bool

	DeviceSplitCount    *uint
	DeviceMemoryScaling *float64
	DeviceCoresScaling  *float64
	NodeName            string
	RuntimeSocketFlag   string
	DisableCoreLimit    *bool
)

type ContainerDevice struct {
	Idx       int
	UUID      string
	Type      string
	Usedmem   int32
	Usedcores int32
	Priority  string
}

type ContainerDeviceRequest struct {
	Nums             int32
	Type             string
	Memreq           int32
	MemPercentagereq int32
	Coresreq         int32
}

type ContainerDevices []ContainerDevice
type ContainerDeviceRequests map[string]ContainerDeviceRequest

type PodSingleDevice []ContainerDevices

type PodDeviceRequests []ContainerDeviceRequests
type PodDevices map[string]PodSingleDevice

type DeviceUsage struct {
	ID        string
	Index     uint
	Used      int32
	Count     int32
	Usedmem   int32
	Totalmem  int32
	Totalcore int32
	Usedcores int32
	Numa      int
	Type      string
	Health    bool
}

type DeviceInfo struct {
	ID      string
	AliasId string
	Index   uint
	Count   int32
	Devmem  int32
	Devcore int32
	Type    string
	Numa    int
	Mode    string
	Health  bool
	Driver  string
}

type NewDeviceInfo struct {
	ID              string          `json:"id,omitempty"`
	Index           uint            `json:"index,omitempty"`
	Count           int32           `json:"count,omitempty"`
	Devmem          int32           `json:"devmem,omitempty"`
	Devcore         int32           `json:"devcore,omitempty"`
	Type            string          `json:"type,omitempty"`
	Numa            int             `json:"numa,omitempty"`
	Mode            string          `json:"mode,omitempty"`
	Health          bool            `json:"health,omitempty"`
	DeviceVendor    string          `json:"devicevendor,omitempty"`
	CustomInfo      map[string]any  `json:"custominfo,omitempty"`
}

type NodeInfo struct {
	ID      string
	Devices []DeviceInfo
}
