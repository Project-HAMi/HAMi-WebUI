package service

import (
	"context"
	"sort"
	"strings"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

var statusOrder = map[string]int{
	biz.ContainerStatusError:       1,
	biz.ContainerStatusFailed:      1,
	biz.ContainerStatusNotReady:    1,
	biz.ContainerStatusUnknown:     1,
	biz.ContainerStatusWaiting:     3,
	biz.ContainerStatusTerminating: 4,
	biz.ContainerStatusSuccess:     5,
	biz.ContainerStatusClosed:      6,
}

func normalizedContainerStatus(status string) string {
	if _, known := statusOrder[status]; known {
		return status
	}
	return biz.ContainerStatusUnknown
}

func containerStatusDetailReply(detail *biz.ContainerStatusDetail) *pb.ContainerStatusDetail {
	if detail == nil {
		return nil
	}
	return &pb.ContainerStatusDetail{
		ContainerState:         detail.ContainerState,
		Reason:                 detail.Reason,
		Message:                detail.Message,
		Ready:                  detail.Ready,
		RestartCount:           detail.RestartCount,
		ExitCode:               detail.ExitCode,
		PodPhase:               detail.PodPhase,
		PodReady:               detail.PodReady,
		PodReadyReason:         detail.PodReadyReason,
		PodReadyMessage:        detail.PodReadyMessage,
		RestartPending:         detail.RestartPending,
		LastTerminationReason:  detail.LastTerminationReason,
		LastExitCode:           detail.LastExitCode,
		LastTerminationMessage: detail.LastTerminationMessage,
		PodReason:              detail.PodReason,
		PodMessage:             detail.PodMessage,
	}
}

type ContainerService struct {
	pb.UnimplementedContainerServer

	node *biz.NodeUsecase
	pod  *biz.PodUseCase
}

func NewContainerService(node *biz.NodeUsecase, pod *biz.PodUseCase) *ContainerService {
	return &ContainerService{node: node, pod: pod}
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

// matchesWorkloadName searches both names shown in the task-list row. A Pod
// can contain several containers, so neither name should be hidden from the
// user-facing filter.
func matchesWorkloadName(podName, containerName, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(podName, filter) || strings.Contains(containerName, filter)
}

// The abnormal filter groups actionable conditions without changing the
// detailed status returned by the API or the existing exact-status filters.
func matchesWorkloadStatus(status, filter string) bool {
	if filter == "abnormal" {
		// Unconfirmed states, such as after node loss, count as abnormal.
		return status == biz.ContainerStatusError || status == biz.ContainerStatusFailed ||
			status == biz.ContainerStatusNotReady || status == biz.ContainerStatusUnknown
	}
	// "all" is also the key of the unfiltered count in status_counts.
	return filter == "" || filter == "all" || filter == status
}

func (s *ContainerService) GetAllContainers(ctx context.Context, req *pb.GetAllContainersReq) (*pb.ContainersReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllContainersReq_Filters{}
	}
	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	var res = &pb.ContainersReply{Items: []*pb.ContainerReply{}}
	for _, container := range containers {
		status := normalizedContainerStatus(container.Status)
		if !matchesWorkloadName(container.PodName, container.Name, filters.Name) {
			continue
		}
		if filters.NodeName != "" && filters.NodeName != container.NodeName {
			continue
		}
		if !matchesWorkloadStatus(status, filters.Status) {
			continue
		}
		if filters.NodeUid != "" && filters.NodeUid != container.NodeUID {
			continue
		}
		priority := strings.Trim(filters.Priority, " ")
		if priority != "" {
			if (priority == "0" && container.Priority == "1") ||
				(priority == "1" && container.Priority != "1") {
				continue
			}
		}
		containerReply := &pb.ContainerReply{}
		containerReply.Name = container.Name
		containerReply.Status = status
		containerReply.StatusDetail = containerStatusDetailReply(container.StatusDetail)
		containerReply.AppName = container.PodName
		containerReply.Images = uniqueNonEmpty([]string{container.Image})
		containerReply.NodeName = container.NodeName
		containerReply.PodUid = container.PodUID
		containerReply.NodeUid = container.NodeUID
		containerReply.Namespace = container.Namespace
		containerReply.Priority = container.Priority
		allocatedCoresKnown := true
		var selected biz.ContainerDevices
		var vendors []string
		for _, containerDevice := range container.ContainerDevices {
			deviceID, vendor := containerDevice.UUID, containerDevice.Vendor
			if device, err := s.node.FindDeviceByAliasId(containerDevice.UUID); err == nil {
				deviceID, vendor = device.Id, device.Provider
			}

			if deviceID == "" {
				continue
			}

			if filters.DeviceId != "" && !strings.HasPrefix(deviceID, filters.DeviceId) {
				continue
			}

			selected = append(selected, containerDevice)
			vendors = append(vendors, vendor)
			containerReply.DeviceIds = append(containerReply.DeviceIds, deviceID)
			containerReply.AllocatedCores = containerReply.AllocatedCores + containerDevice.Usedcores
			if containerDevice.CoreAllocationUnknown {
				allocatedCoresKnown = false
			}
			containerReply.AllocatedMem = containerReply.AllocatedMem + containerDevice.Usedmem
			containerReply.Type = containerDevice.Type
			containerReply.AllocatedDevices++
		}
		if containerReply.DeviceIds == nil {
			continue
		}
		containerReply.AllocatedCoresKnown = &allocatedCoresKnown
		containerReply.AllocationShape, containerReply.Template, containerReply.AllocatedCoresReason = describeAllocation(selected)
		containerReply.AllocationShapeReason = shapeReason(selected, containerReply.AllocationShape)
		containerReply.Vendor = sharedVendor(vendors)
		containerReply.Devices = containerDevices(selected, containerReply.DeviceIds)
		containerReply.CreateTime = container.CreateTime.Format(time.RFC3339)
		res.Items = append(res.Items, containerReply)
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		left, right := res.Items[i], res.Items[j]
		if statusOrder[left.Status] != statusOrder[right.Status] {
			return statusOrder[left.Status] < statusOrder[right.Status]
		}
		// Repository map iteration is unordered. Break visible status-group ties
		// by workload identity, so changing an abnormal condition or refreshing
		// does not arbitrarily reorder rows with the same displayed status.
		if left.Namespace != right.Namespace {
			return left.Namespace < right.Namespace
		}
		if left.AppName != right.AppName {
			return left.AppName < right.AppName
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.PodUid < right.PodUid
	})
	return res, nil
}

func (s *ContainerService) GetContainer(ctx context.Context, req *pb.GetContainerReq) (*pb.ContainerReply, error) {
	container, _ := s.pod.FindOneContainer(ctx, req.PodUid, req.Name)
	if container == nil {
		return &pb.ContainerReply{}, nil
	}
	ctrReply := &pb.ContainerReply{}
	ctrReply.Name = container.Name
	ctrReply.Status = normalizedContainerStatus(container.Status)
	ctrReply.StatusDetail = containerStatusDetailReply(container.StatusDetail)
	ctrReply.AppName = container.PodName
	ctrReply.NodeName = container.NodeName
	ctrReply.PodUid = container.PodUID
	ctrReply.NodeUid = container.NodeUID
	ctrReply.Namespace = container.Namespace
	ctrReply.Priority = container.Priority
	allocatedCoresKnown := true
	allContainers, err := s.pod.ListAllContainers(ctx)
	if err == nil {
		images := make([]string, 0)
		for _, item := range allContainers {
			if item.PodUID == container.PodUID {
				images = append(images, item.Image)
			}
		}
		ctrReply.Images = uniqueNonEmpty(images)
	} else {
		ctrReply.Images = uniqueNonEmpty([]string{container.Image})
	}
	var selected biz.ContainerDevices
	var vendors []string
	for _, containerDevice := range container.ContainerDevices {
		if req.DeviceId != "" && req.DeviceId != containerDevice.UUID {
			continue
		}
		selected = append(selected, containerDevice)
		device, err := s.node.FindDeviceByAliasId(containerDevice.UUID)
		if err != nil {
			ctrReply.DeviceIds = append(ctrReply.DeviceIds, containerDevice.UUID)
			vendors = append(vendors, containerDevice.Vendor)
		} else {
			ctrReply.DeviceIds = append(ctrReply.DeviceIds, device.Id)
			vendors = append(vendors, device.Provider)
		}
		ctrReply.AllocatedCores = ctrReply.AllocatedCores + containerDevice.Usedcores
		if containerDevice.CoreAllocationUnknown {
			allocatedCoresKnown = false
		}
		ctrReply.AllocatedMem = ctrReply.AllocatedMem + containerDevice.Usedmem
		ctrReply.Type = containerDevice.Type
		ctrReply.AllocatedDevices++
	}
	ctrReply.AllocatedCoresKnown = &allocatedCoresKnown
	ctrReply.AllocationShape, ctrReply.Template, ctrReply.AllocatedCoresReason = describeAllocation(selected)
	ctrReply.AllocationShapeReason = shapeReason(selected, ctrReply.AllocationShape)
	ctrReply.Vendor = sharedVendor(vendors)
	ctrReply.Devices = containerDevices(selected, ctrReply.DeviceIds)
	ctrReply.CreateTime = container.CreateTime.Format(time.RFC3339)
	return ctrReply, nil
}

// containerDevices reports each allocated device, so a page can place the
// allocation on the device it shares with other workloads.
func containerDevices(devices biz.ContainerDevices, ids []string) []*pb.ContainerDevice {
	result := make([]*pb.ContainerDevice, 0, len(devices))
	for i, device := range devices {
		id := device.UUID
		if i < len(ids) {
			id = ids[i]
		}
		known := !device.CoreAllocationUnknown
		item := &pb.ContainerDevice{
			Id:                    id,
			Type:                  device.Type,
			AllocatedCores:        device.Usedcores,
			AllocatedCoresKnown:   &known,
			AllocatedMem:          device.Usedmem,
			AllocationShape:       device.Shape,
			AllocationShapeReason: device.ShapeReason,
			Template:              device.Template,
		}
		if device.MigPlacement.Size > 0 {
			start, size := device.MigPlacement.Start, device.MigPlacement.Size
			item.MigStart, item.MigSize = &start, &size
		}
		result = append(result, item)
	}
	return result
}

func sharedVendor(vendors []string) string {
	if len(vendors) == 0 {
		return ""
	}
	for _, vendor := range vendors {
		if vendor != vendors[0] {
			return ""
		}
	}
	return vendors[0]
}

// describeAllocation reports a shape only when every device agrees.
// shapeReason explains an unknown shape the devices agree on.
func shapeReason(devices biz.ContainerDevices, shape string) string {
	if shape != biz.SplitShapeUnknown {
		return ""
	}
	for _, device := range devices {
		if device.ShapeReason != "" {
			return device.ShapeReason
		}
	}
	return ""
}

func describeAllocation(devices biz.ContainerDevices) (shape, template, reason string) {
	for i, device := range devices {
		if i == 0 {
			shape, template = device.Shape, device.Template
		}
		if device.Shape != shape {
			shape = ""
		}
		if device.Template != template {
			template = ""
		}
		if reason == "" && device.CoreAllocationUnknown {
			reason = device.CoreReason
		}
	}
	return shape, template, reason
}
