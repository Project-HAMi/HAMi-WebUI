package util

import (
	"errors"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

const (
	// OneContainerMultiDeviceSplitSymbol this is when one container use multi device, use : symbol to join device info.
	OneContainerMultiDeviceSplitSymbol = ":"

	// OnePodMultiContainerSplitSymbol this is when one pod having multi container and more than one container use device, use ; symbol to join device info.
	OnePodMultiContainerSplitSymbol = ";"

	NvidiaGPUDevice    = "NVIDIA"
	AscendGPUDevice    = "Ascend"
	HygonGPUDevice     = "DCU"
	CambriconGPUDevice = "MLU"

	DsmluProfileAndInstance = "CAMBRICON_DSMLU_PROFILE_INSTANCE"

	NVIDIAPriority = "nvidia.com/priority"
)

var (
	InRequestDevices map[string]string
	SupportDevices   map[string]string
)

func init() {
	InRequestDevices = make(map[string]string)
	SupportDevices = make(map[string]string)
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
			if len(items) == 7 {
				count, _ := strconv.Atoi(items[1])
				devmem, _ := strconv.Atoi(items[2])
				devcore, _ := strconv.Atoi(items[3])
				health, _ := strconv.ParseBool(items[6])
				numa, _ := strconv.Atoi(items[5])
				i := DeviceInfo{
					ID:      items[0],
					AliasId: items[0],
					Count:   int32(count),
					Devmem:  int32(devmem),
					Devcore: int32(devcore),
					Type:    items[4],
					Numa:    numa,
					Health:  health,
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

func DecodeNpuContainerDevices(str string) (ContainerDevices, error) {
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
			tmpdev.Idx = i
			tmpdev.UUID = tmpstr[0]
			tmpdev.Type = tmpstr[1]
			devmem, _ := strconv.ParseInt(tmpstr[2], 10, 32)
			tmpdev.Usedmem = int32(devmem)
			tmpdev.Usedcores = 100
			if tmpdev.Usedmem == 16384 {
				tmpdev.Usedcores = 25
			} else if tmpdev.Usedmem == 32768 {
				tmpdev.Usedcores = 50
			}
			contdev = append(contdev, tmpdev)
		}
	}
	return contdev, nil
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

func GetContainerPriorities(pod *corev1.Pod) []string {
	var priorities []string

	nvidiaPriority := corev1.ResourceName(NVIDIAPriority)
	for _, ctr := range pod.Spec.Containers {
		priority := ""
		if limitPriority, ok := ctr.Resources.Limits[nvidiaPriority]; ok {
			priority = limitPriority.String()
		} else if requestPriority, ok := ctr.Resources.Requests[nvidiaPriority]; ok {
			priority = requestPriority.String()
		}
		priorities = append(priorities, priority)
	}

	return priorities
}

func DecodePodDevices(pod *corev1.Pod, log *log.Helper) (PodDevices, error) {
	checklist := SupportDevices

	priorities := GetContainerPriorities(pod)

	annos := pod.Annotations
	if len(annos) == 0 {
		return PodDevices{}, nil
	}
	nodeName := annos[AssignedNodeAnnotations]
	pd := make(PodDevices)
	for devType, devs := range checklist {
		str, ok := annos[devs]
		if !ok {
			continue
		}
		pd[devType] = make(PodSingleDevice, 0)
		switch devType {
		case AscendGPUDevice:
			for _, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				cd, err := DecodeNpuContainerDevices(s)
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		case CambriconGPUDevice:
			instance := annos[DsmluProfileAndInstance]
			cd, err := DecodeMLUContainerDevices(fmt.Sprintf("%s_%s", str, instance, nodeName), nodeName)
			if err != nil {
				return PodDevices{}, nil
			}
			if len(cd) == 0 {
				continue
			}
			pd[devType] = append(pd[devType], cd)
		case NvidiaGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if s == "" {
					continue
				}
				cd, err := DecodeContainerDevices(s, priorities[i])
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		case HygonGPUDevice:
			for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
				if s == "" {
					continue
				}
				cd, err := DecodeDCUContainerDevices(s, priorities[i], nodeName)
				if err != nil {
					return PodDevices{}, nil
				}
				if len(cd) == 0 {
					continue
				}
				pd[devType] = append(pd[devType], cd)
			}
		}
	}
	log.Infof("Decoded pod annos: poddevices %v", pd)
	return pd, nil
}
