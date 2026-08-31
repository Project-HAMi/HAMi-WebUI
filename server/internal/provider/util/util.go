package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
)

const (
	// OneContainerMultiDeviceSplitSymbol this is when one container use multi device, use : symbol to join device info.
	OneContainerMultiDeviceSplitSymbol = ":"

	// OnePodMultiContainerSplitSymbol this is when one pod having multi container and more than one container use device, use ; symbol to join device info.
	OnePodMultiContainerSplitSymbol = ";"

	NvidiaGPUDevice     = "NVIDIA"
	AscendGPUDevice     = "Ascend"
	Ascend310PGPUDevice = "Ascend310P"
	HygonGPUDevice      = "DCU"
	CambriconGPUDevice  = "MLU"
	MetaxGPUDevice      = "Metax-GPU"
	MetaxSGPUDevice     = "Metax-SGPU"

	DsmluProfileAndInstance = "CAMBRICON_DSMLU_PROFILE_INSTANCE"

	NVIDIAPriority = "nvidia.com/priority"

	AscendVNPUModeAnnotation     = "huawei.com/vnpu-mode"
	AscendVNPUModeHamiCore       = "hami-core"
	AscendNodeHamiCoreAnnotation = "hami-vnpu-core"
)

// VendorOf maps a concrete device type to the provider name used for metric
// dispatch. Ascend annotations carry the chip commonWord as their type; keep
// that concrete type in inventory and normalize only at the provider boundary.
func VendorOf(deviceType string) string {
	if strings.HasPrefix(deviceType, AscendGPUDevice) {
		return AscendGPUDevice
	}
	return deviceType
}

type ascendDeviceConfig struct {
	Usedmem   int32
	Usedcores int32
}

var (
	InRequestDevices    map[string]string
	SupportDevices      map[string]string
	ascendDeviceConfigs map[string]map[int32]ascendDeviceConfig
)

func init() {
	InRequestDevices = make(map[string]string)
	SupportDevices = make(map[string]string)
	// This is a compatibility snapshot of the templates shipped by HAMi. Custom
	// operator templates are intentionally not guessed: their core allocation is
	// reported as unavailable until WebUI has a configured source of truth.
	ascendDeviceConfigs = map[string]map[int32]ascendDeviceConfig{
		"Ascend910A": {
			2184:  {Usedmem: 2184, Usedcores: 7},
			4369:  {Usedmem: 4369, Usedcores: 13},
			8738:  {Usedmem: 8738, Usedcores: 27},
			17476: {Usedmem: 17476, Usedcores: 53},
			32768: {Usedmem: 32768, Usedcores: 100},
		},
		"Ascend910B": {
			16384: {Usedmem: 16384, Usedcores: 25},
			32768: {Usedmem: 32768, Usedcores: 50},
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		"Ascend910B2": {
			8192:  {Usedmem: 8192, Usedcores: 13},
			16384: {Usedmem: 16384, Usedcores: 25},
			32768: {Usedmem: 32768, Usedcores: 50},
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		"Ascend910B3": {
			16384: {Usedmem: 16384, Usedcores: 25},
			32768: {Usedmem: 32768, Usedcores: 50},
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		"Ascend910B4": {
			8192:  {Usedmem: 8192, Usedcores: 25},
			16384: {Usedmem: 16384, Usedcores: 50},
			32768: {Usedmem: 32768, Usedcores: 100},
		},
		"Ascend910B4-1": {
			16384: {Usedmem: 16384, Usedcores: 25},
			32768: {Usedmem: 32768, Usedcores: 50},
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		"Ascend910C": {
			16384: {Usedmem: 16384, Usedcores: 25},
			32768: {Usedmem: 32768, Usedcores: 50},
			65536: {Usedmem: 65536, Usedcores: 100},
		},
		"Ascend310P": {
			3072:  {Usedmem: 3072, Usedcores: 13},
			6144:  {Usedmem: 6144, Usedcores: 25},
			12288: {Usedmem: 12288, Usedcores: 50},
			21527: {Usedmem: 21527, Usedcores: 100},
		},
	}
	initMLUDevice()
}

func initMLUDevice() {
}

func EncodeNodeDevices(dlist []*DeviceInfo, log *log.Helper) string {
	tmp := ""
	for _, val := range dlist {
		tmp += val.ID + "," + strconv.FormatInt(int64(val.Count), 10) + "," + strconv.Itoa(int(val.Devmem)) + "," + strconv.Itoa(int(val.Devcore)) + "," + val.Type + "," + strconv.Itoa(val.Numa) + "," + strconv.FormatBool(val.Health) + OneContainerMultiDeviceSplitSymbol
	}
	log.Infof("Encoded node Devices: %s", tmp)
	return tmp
}

// DecodeNodeDevices decodes the node devices from a string.
func DecodeNodeDevices(str string, log *log.Helper) ([]*DeviceInfo, error) {
	if !strings.Contains(str, OneContainerMultiDeviceSplitSymbol) {
		log.Warn("Node annotations not decode successfully")
		return []*DeviceInfo{}, errors.New("node annotations not decode successfully")
	}
	tmp := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	var retval []*DeviceInfo
	for _, val := range tmp {
		if strings.Contains(val, ",") {
			items := strings.Split(val, ",")
			if len(items) >= 7 || len(items) == 9 {
				count, _ := strconv.ParseInt(items[1], 10, 32)
				devmem, _ := strconv.ParseInt(items[2], 10, 32)
				devcore, _ := strconv.ParseInt(items[3], 10, 32)
				health, _ := strconv.ParseBool(items[6])
				numa, _ := strconv.Atoi(items[5])
				mode := "hami-core"
				index := 0
				if len(items) == 9 {
					index, _ = strconv.Atoi(items[7])
					mode = items[8]
				}
				i := DeviceInfo{
					ID:      items[0],
					AliasId: items[0],
					Count:   int32(count),
					Devmem:  int32(devmem),
					Devcore: int32(devcore),
					Type:    items[4],
					Numa:    numa,
					Health:  health,
					Mode:    mode,
					Index:   uint(index),
				}
				retval = append(retval, &i)
			} else {
				return []*DeviceInfo{}, errors.New("node annotations not decode successfully")
			}
		}
	}
	return retval, nil
}

// DecodeContainerDevices decodes the container devices from a string.
func DecodeContainerDevices(str, priority string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			tmpdev := ContainerDevice{}
			tmpdev.Idx = i
			tmpdev.UUID = tmpstr[0]
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			devcores, _ := strconv.ParseInt(tmpstr[3], 10, 32)
			if devcores == 0 {
				tmpdev.Usedcores = 100
			} else {
				tmpdev.Usedcores = int32(devcores)
			}
			tmpdev.Priority = priority
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func DecodeDCUContainerDevices(str, priority, nodeName string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			cardIdx := strings.Split(tmpstr[0], "-")
			tmpdev.Idx = i
			tmpdev.UUID = fmt.Sprintf("%s-dcu-%s", nodeName, cardIdx[1])
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			devcores, _ := strconv.ParseInt(tmpstr[3], 10, 32)
			if devcores == 0 {
				tmpdev.Usedcores = 100
			} else {
				tmpdev.Usedcores = int32(devcores)
			}
			tmpdev.Priority = priority
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func DecodeNpuContainerDevices(str string, mode AscendAllocationMode) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}
	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			devmem, err := strconv.ParseInt(tmpstr[2], 10, 32)
			if err != nil || devmem < 0 {
				return ContainerDevices{}, fmt.Errorf("invalid Ascend memory field %q", tmpstr[2])
			}
			devcores, err := strconv.ParseInt(tmpstr[3], 10, 32)
			if err != nil || devcores < 0 {
				return ContainerDevices{}, fmt.Errorf("invalid Ascend core field %q", tmpstr[3])
			}
			tmpdev := ContainerDevice{Idx: i, UUID: tmpstr[0], Type: tmpstr[1], Usedmem: int32(devmem)}
			tmpdev.Usedcores, tmpdev.CoreAllocationKnown = ascendCoreAllocation(tmpdev.Type, tmpdev.Usedmem, int32(devcores), mode)

			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func ascendCoreAllocation(deviceType string, memory, annotationCore int32, mode AscendAllocationMode) (int32, bool) {
	switch mode {
	case AscendAllocationModeHamiCore:
		return annotationCore, true
	case AscendAllocationModeTemplate:
		if configs, exists := ascendDeviceConfigs[deviceType]; exists {
			if config, ok := configs[memory]; ok {
				return config.Usedcores, true
			}
		}
		return 0, false
	default:
		return 0, false
	}
}

// DecodeMLUContainerDevices decodes the mlu container devices from a string.
func DecodeMLUContainerDevices(str, nodeName string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	contdev := ContainerDevices{}
	tmpdev := ContainerDevice{}
	if strings.Contains(str, "_") {
		//fmt.Println("cd is ", val)
		tmpstr := strings.Split(str, "_")
		if len(tmpstr) < 3 {
			return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
		}
		tmpdev.Type = "MLU"
		devcores, _ := strconv.ParseInt(tmpstr[1], 10, 32)
		devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
		tmpdev.Usedmem = int32(devmem) * 1024
		index, _ := strconv.ParseInt(tmpstr[5], 10, 32)
		tmpdev.Idx = int(index)
		tmpdev.UUID = fmt.Sprintf("%s-cambricon-mlu-%d", nodeName, index)
		if devcores == 0 {
			tmpdev.Usedcores = 100
		} else {
			tmpdev.Usedcores = int32(devcores)
		}
		contdev = append(contdev, tmpdev)
	}
	return contdev, nil
}

func DecodeMetaxContainerDevices(str string) (ContainerDevices, error) {
	if len(str) == 0 {
		return ContainerDevices{}, nil
	}
	cd := strings.Split(str, OneContainerMultiDeviceSplitSymbol)
	contdev := ContainerDevices{}

	for i, val := range cd {
		if strings.Contains(val, ",") {
			tmpstr := strings.Split(val, ",")
			if len(tmpstr) < 4 {
				return ContainerDevices{}, fmt.Errorf("pod annotation format error; information missing, please do not use nodeName field in task")
			}
			tmpdev := ContainerDevice{}
			tmpdev.Idx = i
			tmpdev.UUID = tmpstr[0]
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			devcores, _ := strconv.ParseInt(tmpstr[3], 10, 32)
			if devcores == 0 {
				tmpdev.Usedcores = 100
			} else {
				tmpdev.Usedcores = int32(devcores)
			}
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
}

func getContainerPriority(ctr corev1.Container) string {
	nvidiaPriority := corev1.ResourceName(NVIDIAPriority)
	if limitPriority, ok := ctr.Resources.Limits[nvidiaPriority]; ok {
		return limitPriority.String()
	}
	if requestPriority, ok := ctr.Resources.Requests[nvidiaPriority]; ok {
		return requestPriority.String()
	}
	return ""
}

func GetContainerPriorities(pod *corev1.Pod) []string {
	priorities := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, ctr := range pod.Spec.InitContainers {
		priorities = append(priorities, getContainerPriority(ctr))
	}
	for _, ctr := range pod.Spec.Containers {
		priorities = append(priorities, getContainerPriority(ctr))
	}
	return priorities
}

func podContainerCount(pod *corev1.Pod) int {
	return len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
}

func DecodePodDevices(pod *corev1.Pod, log *log.Helper, ascendMode AscendAllocationMode) (PodDevices, error) {
	checklist := supportDevicesForPod(pod.Annotations)

	priorities := GetContainerPriorities(pod)

	annos := pod.Annotations
	if len(annos) == 0 {
		return PodDevices{}, nil
	}
	nodeName := annos[AssignedNodeAnnotations]
	pd := make(PodDevices)
	deviceTypes := make([]string, 0, len(checklist))
	for devType := range checklist {
		deviceTypes = append(deviceTypes, devType)
	}
	sort.Strings(deviceTypes)
	for _, devType := range deviceTypes {
		devs := checklist[devType]
		str, ok := annos[devs]
		if !ok {
			continue
		}
		pd[devType] = make(PodSingleDevice, 0)
		switch VendorOf(devType) {
		case AscendGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if i >= podContainerCount(pod) {
					break
				}
				if s == "" {
					pd[devType] = append(pd[devType], ContainerDevices{})
					continue
				}
				cd, err := DecodeNpuContainerDevices(s, ascendMode)
				if err != nil {
					return PodDevices{}, err
				}
				for _, device := range cd {
					if device.Type != devType {
						return PodDevices{}, fmt.Errorf("ascend allocation annotation %s contains device type %q, want %q", devs, device.Type, devType)
					}
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		case CambriconGPUDevice:
			instance := annos[DsmluProfileAndInstance]
			cd, err := DecodeMLUContainerDevices(fmt.Sprintf("%s_%s", str, instance), nodeName)
			if err != nil {
				return PodDevices{}, nil
			}
			if len(cd) == 0 {
				continue
			}
			pd[devType] = append(pd[devType], cd)
		case NvidiaGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if i >= podContainerCount(pod) {
					break
				}
				if s == "" {
					pd[devType] = append(pd[devType], ContainerDevices{})
					continue
				}
				cd, err := DecodeContainerDevices(s, priorities[i])
				if err != nil {
					return PodDevices{}, nil
				}
				pd[devType] = append(pd[devType], cd)
			}
		case HygonGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if i >= podContainerCount(pod) {
					break
				}
				if s == "" {
					pd[devType] = append(pd[devType], ContainerDevices{})
					continue
				}
				cd, err := DecodeDCUContainerDevices(s, priorities[i], nodeName)
				if err != nil {
					return PodDevices{}, nil
				}
				pd[devType] = append(pd[devType], cd)
			}
		case MetaxGPUDevice, MetaxSGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if i >= podContainerCount(pod) {
					break
				}
				if s == "" {
					pd[devType] = append(pd[devType], ContainerDevices{})
					continue
				}
				cd, err := DecodeMetaxContainerDevices(s)
				if err != nil {
					return PodDevices{}, nil
				}
				pd[devType] = append(pd[devType], cd)
			}
		}
	}
	log.Infof("Decoded pod annos: poddevices %v", pd)
	return pd, nil
}

func supportDevicesForPod(annotations map[string]string) map[string]string {
	devices := make(map[string]string, len(SupportDevices))
	for deviceType, annotation := range SupportDevices {
		devices[deviceType] = annotation
	}

	const (
		ascendAllocatedPrefix = "hami.io/Ascend"
		allocatedSuffix       = "-devices-allocated"
	)
	discovered := make([]string, 0)
	for annotation := range annotations {
		if strings.HasPrefix(annotation, ascendAllocatedPrefix) && strings.HasSuffix(annotation, allocatedSuffix) {
			discovered = append(discovered, annotation)
		}
	}
	sort.Strings(discovered)
	for _, annotation := range discovered {
		deviceType := strings.TrimSuffix(strings.TrimPrefix(annotation, "hami.io/"), allocatedSuffix)
		if deviceType == "" {
			continue
		}
		// Replace legacy aliases that point at the same annotation so a concrete
		// producer commonWord is decoded exactly once and remains visible.
		for existingType, existingAnnotation := range devices {
			if existingAnnotation == annotation {
				delete(devices, existingType)
			}
		}
		devices[deviceType] = annotation
	}
	return devices
}

func UnMarshalNodeDevices(str string) ([]*DeviceInfo, error) {
	var dlist []*DeviceInfo
	err := json.Unmarshal([]byte(str), &dlist)
	return dlist, err
}

func MapNewDeviceInfoToDeviceInfo(newDeviceInfo *NewDeviceInfo) *DeviceInfo {
	return &DeviceInfo{
		ID:      newDeviceInfo.ID,
		AliasId: newDeviceInfo.ID,
		Index:   newDeviceInfo.Index,
		Count:   newDeviceInfo.Count,
		Devmem:  newDeviceInfo.Devmem,
		Devcore: newDeviceInfo.Devcore,
		Type:    newDeviceInfo.Type,
		Numa:    newDeviceInfo.Numa,
		Mode:    newDeviceInfo.Mode,
		Health:  newDeviceInfo.Health,
	}
}
