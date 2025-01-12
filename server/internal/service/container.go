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
	biz.ContainerStatusFailed:  1,
	biz.ContainerStatusUnknown: 2,
	biz.ContainerStatusSuccess: 3,
	biz.ContainerStatusClosed:  4,
}

type ContainerService struct {
	pb.UnimplementedContainerServer

	node *biz.NodeUsecase
	pod  *biz.PodUseCase
}

func NewContainerService(node *biz.NodeUsecase, pod *biz.PodUseCase) *ContainerService {
	return &ContainerService{node: node, pod: pod}
}

func (s *ContainerService) GetAllContainers(ctx context.Context, req *pb.GetAllContainersReq) (*pb.ContainersReply, error) {
	filters := req.Filters
	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	var res = &pb.ContainersReply{Items: []*pb.ContainerReply{}}
	for _, container := range containers {
		if filters.Name != "" && !strings.Contains(container.Name, filters.Name) {
			continue
		}
		if filters.NodeName != "" && filters.NodeName != container.NodeName {
			continue
		}
		if filters.Status != "" && filters.Status != container.Status {
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
		containerReply.Status = container.Status
		containerReply.AppName = container.PodName
		containerReply.NodeName = container.NodeName
		containerReply.PodUid = container.PodUID
		containerReply.NodeUid = container.NodeUID
		containerReply.Namespace = container.Namespace
		containerReply.Priority = container.Priority
		for _, containerDevice := range container.ContainerDevices {
			deviceID := containerDevice.UUID
			if device, err := s.node.FindDeviceByAliasId(containerDevice.UUID); err == nil {
				deviceID = device.Id
			}

			if filters.DeviceId != "" && !strings.HasPrefix(deviceID, filters.DeviceId) {
				continue
			}

			containerReply.DeviceIds = append(containerReply.DeviceIds, deviceID)
			containerReply.AllocatedCores = containerReply.AllocatedCores + containerDevice.Usedcores
			containerReply.AllocatedMem = containerReply.AllocatedMem + containerDevice.Usedmem
			containerReply.Type = containerDevice.Type
			containerReply.AllocatedDevices++
		}
		if containerReply.DeviceIds == nil {
			continue
		}
		containerReply.CreateTime = container.CreateTime.Format(time.RFC3339)
		res.Items = append(res.Items, containerReply)
	}
	sort.SliceStable(res.Items, func(i, j int) bool {
		return statusOrder[res.Items[i].Status] < statusOrder[res.Items[j].Status]
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
	ctrReply.Status = container.Status
	ctrReply.AppName = container.PodName
	ctrReply.NodeName = container.NodeName
	ctrReply.PodUid = container.PodUID
	ctrReply.NodeUid = container.NodeUID
	ctrReply.Namespace = container.Namespace
	ctrReply.Priority = container.Priority
	for _, containerDevice := range container.ContainerDevices {
		if req.DeviceId != "" && req.DeviceId != containerDevice.UUID {
			continue
		}
		device, err := s.node.FindDeviceByAliasId(containerDevice.UUID)
		if err != nil {
			ctrReply.DeviceIds = append(ctrReply.DeviceIds, containerDevice.UUID)
		} else {
			ctrReply.DeviceIds = append(ctrReply.DeviceIds, device.Id)
		}
		ctrReply.AllocatedCores = ctrReply.AllocatedCores + containerDevice.Usedcores
		ctrReply.AllocatedMem = ctrReply.AllocatedMem + containerDevice.Usedmem
		ctrReply.Type = containerDevice.Type
		ctrReply.AllocatedDevices++
	}
	ctrReply.CreateTime = container.CreateTime.Format(time.RFC3339)
	return ctrReply, nil
}
