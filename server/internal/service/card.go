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
	ms   *MonitorService
}

func NewCardService(node *biz.NodeUsecase, pod *biz.PodUseCase, ms *MonitorService) *CardService {
	return &CardService{node: node, pod: pod, ms: ms}
}

func (s *CardService) GetAllGPUs(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	filters := req.Filters
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

	// Pull hami_core_size / hami_memory_size for ALL devices in two queries keyed
	// by device_uuid, instead of two PromQL per device. The old per-device fanout
	// meant ~2*N serial instant queries against Prometheus / VictoriaMetrics
	// (300+ at large scale), which made the card list spin for several seconds.
	coreSizeByUUID := s.queryGaugeByLabel(ctx, "avg(hami_core_size) by (device_uuid)", "device_uuid")
	memSizeByUUID := s.queryGaugeByLabel(ctx, "avg(hami_memory_size) by (device_uuid)", "device_uuid")

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
		gpu.CoreTotal = device.Devcore
		gpu.MemoryTotal = device.Devmem
		gpu.NodeUid = device.NodeUid
		gpu.Health = device.Health
		gpu.Mode = device.Mode

		vGPU, core, memory := biz.ContainersStatisticsInfo(containers, device.AliasId)
		gpu.VgpuUsed = vGPU
		gpu.CoreUsed = core
		gpu.MemoryUsed = memory

		if v, ok := coreSizeByUUID[device.Id]; ok {
			gpu.CoreTotal = v
		}
		if v, ok := memSizeByUUID[device.Id]; ok {
			gpu.MemoryTotal = v
		}
		res.List = append(res.List, gpu)
	}

	sort.SliceStable(res.List, func(i, j int) bool {
		return res.List[i].Uuid < res.List[j].Uuid
	})
	return res, nil
}

// queryGaugeByLabel runs a single instant query and returns the result values
// keyed by the given label, so callers can batch what used to be per-entity
// lookups into one round-trip to Prometheus / VictoriaMetrics.
func (s *CardService) queryGaugeByLabel(ctx context.Context, query, label string) map[string]int32 {
	out := map[string]int32{}
	resp, err := s.ms.QueryInstant(ctx, &pb.QueryInstantRequest{Query: query})
	if err != nil {
		return out
	}
	for _, sample := range resp.Data {
		key := sample.Metric[label]
		if key == "" {
			continue
		}
		out[key] = int32(sample.Value)
	}
	return out
}

func (s *CardService) GetAllGPUTypes(ctx context.Context, req *pb.GetAllGpusReq) (*pb.GPUsReply, error) {
	deviceInfos, err := s.node.ListAllDevices(ctx)
	if err != nil {
		return nil, err
	}

	var res = &pb.GPUsReply{List: []*pb.GPUReply{}}
	seenTypes := make(map[string]struct{})

	filters := req.Filters
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
		gpu.CoreTotal = device.Devcore
		gpu.MemoryTotal = device.Devmem
		gpu.NodeUid = device.NodeUid
		gpu.Health = device.Health
		gpu.Mode = device.Mode

		vGPU, core, memory, err := s.pod.StatisticsByDeviceId(ctx, device.AliasId)
		if err == nil {
			gpu.VgpuUsed = vGPU
			gpu.CoreUsed = core
			gpu.MemoryUsed = memory
		}
		return gpu, nil
	}
	return gpu, nil
}
