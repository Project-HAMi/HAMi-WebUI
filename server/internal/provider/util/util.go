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

	NvidiaGPUDevice    = "NVIDIA"
	AscendGPUDevice    = "Ascend"
	HygonGPUDevice     = "DCU"
	HygonHCUDevice     = "HCU"
	CambriconGPUDevice = "MLU"
	MetaxGPUDevice     = "Metax-GPU"
	MetaxSGPUDevice    = "Metax-SGPU"

	NVIDIAPriority = "nvidia.com/priority"
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
			if len(items) >= 7 || len(items) == 9 {
				count, _ := strconv.ParseInt(items[1], 10, 32)
				devmem, _ := strconv.ParseInt(items[2], 10, 32)
				devcore, _ := strconv.ParseInt(items[3], 10, 32)
				health, _ := strconv.ParseBool(items[6])
				numa, _ := strconv.Atoi(items[5])
				mode := ""
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

// deviceSegment is one "id,type,memory,cores" allocation; HAMi may append optional fields after these.
type deviceSegment struct {
	id, devType string
	mem, cores  int32
}

func parseDeviceSegment(val string) (deviceSegment, error) {
	fields := strings.Split(val, ",")
	if len(fields) < 4 {
		return deviceSegment{}, fmt.Errorf("allocation %q has fewer than 4 fields", val)
	}
	if fields[0] == "" || fields[1] == "" {
		return deviceSegment{}, fmt.Errorf("allocation %q has no device id or type", val)
	}
	mem, err := parseNonNegativeInt32(fields[2])
	if err != nil {
		return deviceSegment{}, fmt.Errorf("allocation %q memory: %w", val, err)
	}
	cores, err := parseNonNegativeInt32(fields[3])
	if err != nil {
		return deviceSegment{}, fmt.Errorf("allocation %q cores: %w", val, err)
	}
	return deviceSegment{id: fields[0], devType: fields[1], mem: mem, cores: cores}, nil
}

func parseNonNegativeInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative value %d", v)
	}
	return int32(v), nil
}

// coresOrWholeCard keeps the current reading of a zero core request as the whole card.
func coresOrWholeCard(cores int32) int32 {
	if cores == 0 {
		return 100
	}
	return cores
}

// decodeSegments reads one container's allocations; only the empty segment after a trailing separator is skipped.
func decodeSegments(str string, build func(i int, seg deviceSegment) (ContainerDevice, error)) (ContainerDevices, error) {
	contdev := ContainerDevices{}
	for i, val := range strings.Split(str, OneContainerMultiDeviceSplitSymbol) {
		if val == "" {
			continue
		}
		seg, err := parseDeviceSegment(val)
		if err != nil {
			return ContainerDevices{}, err
		}
		dev, err := build(i, seg)
		if err != nil {
			return ContainerDevices{}, err
		}
		contdev = append(contdev, dev)
	}
	return contdev, nil
}

// DecodeContainerDevices decodes the container devices from a string.
func DecodeContainerDevices(str, priority string) (ContainerDevices, error) {
	return decodeSegments(str, func(i int, seg deviceSegment) (ContainerDevice, error) {
		return ContainerDevice{Idx: i, UUID: seg.id, Type: seg.devType, Usedmem: seg.mem, Usedcores: coresOrWholeCard(seg.cores), Priority: priority}, nil
	})
}

func DecodeDCUContainerDevices(str, priority, nodeName string) (ContainerDevices, error) {
	return decodeSegments(str, func(i int, seg deviceSegment) (ContainerDevice, error) {
		parts := strings.Split(seg.id, "-")
		if len(parts) < 2 || parts[1] == "" {
			return ContainerDevice{}, fmt.Errorf("invalid DCU device id %q", seg.id)
		}
		return ContainerDevice{
			Idx: i, UUID: fmt.Sprintf("%s-dcu-%s", nodeName, parts[1]), Type: seg.devType,
			Usedmem: seg.mem, Usedcores: coresOrWholeCard(seg.cores), Priority: priority,
		}, nil
	})
}

func DecodeMetaxContainerDevices(str string) (ContainerDevices, error) {
	return decodeSegments(str, func(i int, seg deviceSegment) (ContainerDevice, error) {
		return ContainerDevice{Idx: i, UUID: seg.id, Type: seg.devType, Usedmem: seg.mem, Usedcores: coresOrWholeCard(seg.cores)}, nil
	})
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

// DecodeError reports one device type whose allocation annotation could not be read.
type DecodeError struct {
	DeviceType string
	Err        error
}

func (e *DecodeError) Error() string { return e.DeviceType + ": " + e.Err.Error() }

func (e *DecodeError) Unwrap() error { return e.Err }

// DecodePodDevices skips Ascend, which the ascend provider decodes against the device configuration.
// A device type that fails to decode is left out and reported, so the pod's other allocations still count.
func DecodePodDevices(pod *corev1.Pod, log *log.Helper) (PodDevices, []*DecodeError) {
	checklist := supportDevicesForPod(pod.Annotations)
	annos := pod.Annotations
	if len(annos) == 0 {
		return PodDevices{}, nil
	}
	pd := make(PodDevices)
	var failures []*DecodeError
	deviceTypes := make([]string, 0, len(checklist))
	for devType := range checklist {
		deviceTypes = append(deviceTypes, devType)
	}
	sort.Strings(deviceTypes)
	for _, devType := range deviceTypes {
		str, ok := annos[checklist[devType]]
		if !ok {
			continue
		}
		devices, err := decodeDeviceType(pod, devType, str)
		if err != nil {
			failures = append(failures, &DecodeError{DeviceType: devType, Err: err})
			continue
		}
		pd[devType] = devices
	}
	log.Infof("Decoded pod annos: poddevices %v", pd)
	return pd, failures
}

func decodeDeviceType(pod *corev1.Pod, devType, str string) (PodSingleDevice, error) {
	annos := pod.Annotations
	nodeName := annos[AssignedNodeAnnotations]
	devices := make(PodSingleDevice, 0)
	var decode func(s, priority string) (ContainerDevices, error)
	switch VendorOf(devType) {
	case NvidiaGPUDevice, HygonHCUDevice, CambriconGPUDevice:
		decode = DecodeContainerDevices
	case HygonGPUDevice:
		decode = func(s, priority string) (ContainerDevices, error) {
			return DecodeDCUContainerDevices(s, priority, nodeName)
		}
	case MetaxGPUDevice, MetaxSGPUDevice:
		decode = func(s, _ string) (ContainerDevices, error) { return DecodeMetaxContainerDevices(s) }
	default:
		return devices, nil
	}
	priorities := GetContainerPriorities(pod)
	for i, s := range strings.Split(str, OnePodMultiContainerSplitSymbol) {
		if i >= podContainerCount(pod) {
			break
		}
		if s == "" {
			devices = append(devices, ContainerDevices{})
			continue
		}
		cd, err := decode(s, priorities[i])
		if err != nil {
			return nil, err
		}
		devices = append(devices, cd)
	}
	return devices, nil
}

func supportDevicesForPod(map[string]string) map[string]string {
	devices := make(map[string]string, len(SupportDevices))
	for deviceType, annotation := range SupportDevices {
		devices[deviceType] = annotation
	}
	return devices
}

func UnMarshalNodeDevices(str string) ([]*DeviceInfo, error) {
	var dlist []*DeviceInfo
	err := json.Unmarshal([]byte(str), &dlist)
	return dlist, err
}

// MapNewDeviceInfoToDeviceInfo leaves MigProfiles to ParseMigProfiles.
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

func ParseMigProfiles(raw json.RawMessage) ([]MigProfile, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var profiles []MigProfile
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}
