package service

import (
	"context"
	"sort"
	"strings"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

type CardService struct {
	pb.UnimplementedCardServer

	node *biz.NodeUsecase
	pod  *biz.PodUseCase
}

func NewCardService(node *biz.NodeUsecase, pod *biz.PodUseCase) *CardService {
	return &CardService{node: node, pod: pod}
}

func (s *CardService) GetAllGPUs(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllGpusReq_Filters{}
	}
	deviceInfos, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch the container list once. StatisticsByDeviceId re-lists all containers
	// on every call, so calling it per device turned this into an O(devices)
	// re-scan; at large scale (100+ cards) that alone was seconds of work.
	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}

	var res = &pb.GPUsReply{List: []*pb.GPUReply{}}
	for _, device := range deviceInfos {
		gpu := &pb.GPUReply{}
		nodeName := strings.Trim(filters.NodeName, " ")
		if nodeName != "" && nodeName != device.NodeName {
			continue
		}
		deviceType := strings.Trim(filters.Type, " ")
		if deviceType != "" && deviceType != device.Type {
			continue
		}
		deviceUid := strings.Trim(filters.Uid, " ")
		if deviceUid != "" && deviceUid != device.Id {
			continue
		}
		gpu.Uuid = device.Id
		gpu.NodeName = device.NodeName
		gpu.Type = device.Type
		gpu.VgpuTotal = device.Count
		gpu.CoreTotal = biz.PhysicalCoreBaselinePerDevice
		gpu.MemoryTotal = device.Devmem
		gpu.NodeUid = device.NodeUid
		gpu.Health = device.Health
		gpu.Mode = device.Mode

		vGPU, core, memory, coreKnown := biz.ContainersStatisticsInfo(containers, device.AliasId)
		gpu.VgpuUsed = vGPU
		gpu.CoreUsed = core
		gpu.CoreUsedKnown = &coreKnown
		gpu.MemoryUsed = memory

		res.List = append(res.List, gpu)
	}

	sort.SliceStable(res.List, func(i, j int) bool {
		return res.List[i].Uuid < res.List[j].Uuid
	})
	return res, nil
}

func (s *CardService) GetAllGPUTypes(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	deviceInfos, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	var res = &pb.GPUsReply{List: []*pb.GPUReply{}}
	seenTypes := make(map[string]struct{})

	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllGpusReq_Filters{}
	}
	provider := strings.Trim(filters.Provider, " ")
	for _, device := range deviceInfos {
		if provider != "" && provider != device.Provider {
			continue
		}

		if _, exists := seenTypes[device.Type]; !exists {
			seenTypes[device.Type] = struct{}{}
			gpu := &pb.GPUReply{}
			gpu.Type = device.Type
			res.List = append(res.List, gpu)
		}
	}

	return res, nil
}

func (s *CardService) GetGPU(ctx context.Context, req *pb.GetGpuReq) (*pb.GPUReply, error) {
	devices, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}
	gpu := &pb.GPUReply{}
	for _, device := range devices {
		deviceUid := strings.Trim(req.Uid, " ")
		if deviceUid == "" || deviceUid != device.Id {
			continue
		}
		gpu.Uuid = device.Id
		gpu.NodeName = device.NodeName
		gpu.Type = device.Type
		gpu.VgpuTotal = device.Count
		gpu.CoreTotal = biz.PhysicalCoreBaselinePerDevice
		gpu.MemoryTotal = device.Devmem
		gpu.NodeUid = device.NodeUid
		gpu.Health = device.Health
		gpu.Mode = device.Mode

		vGPU, core, memory, coreKnown, err := s.pod.StatisticsByDeviceId(ctx, device.AliasId)
		if err == nil {
			gpu.VgpuUsed = vGPU
			gpu.CoreUsed = core
			gpu.CoreUsedKnown = &coreKnown
			gpu.MemoryUsed = memory
		}
		return gpu, nil
	}
	return gpu, nil
}
