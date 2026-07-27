package service

import (
	"context"
	"sort"
	"strconv"
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

	// Pull size/usage gauges for ALL devices in a few queries keyed by
	// device_uuid, instead of per-device PromQL fanout.
	coreSizeByUUID := s.queryGaugeByLabel(ctx, "avg(hami_core_size) by (device_uuid)", "device_uuid")
	memSizeByUUID := s.queryGaugeByLabel(ctx, "avg(hami_memory_size) by (device_uuid)", "device_uuid")
	coreUsageByUUID := s.queryGaugeByLabel(ctx, "avg(hami_core_util_avg) by (device_uuid)", "device_uuid")
	memUsageByUUID := s.queryGaugeByLabel(ctx, "avg(hami_memory_used) by (device_uuid)", "device_uuid")
	nodeIPByName := s.nodeIPByName(ctx)

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
		if want, err := strconv.ParseBool(strings.TrimSpace(filters.Health)); err == nil {
			if want != device.Health {
				continue
			}
		}
		gpu.Uuid = device.Id
		gpu.NodeName = device.NodeName
		gpu.NodeIp = nodeIPByName[device.NodeName]
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
			gpu.CoreTotal = int32(v)
		}
		if v, ok := memSizeByUUID[device.Id]; ok {
			gpu.MemoryTotal = int32(v)
		}
		if v, ok := coreUsageByUUID[device.Id]; ok {
			gpu.CoreUsage = v
		}
		if v, ok := memUsageByUUID[device.Id]; ok {
			gpu.MemoryUsage = int32(v)
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
func (s *CardService) queryGaugeByLabel(ctx context.Context, query, label string) map[string]float32 {
	out := map[string]float32{}
	resp, err := s.ms.QueryInstant(ctx, &pb.QueryInstantRequest{Query: query})
	if err != nil {
		return out
	}
	for _, sample := range resp.Data {
		key := sample.Metric[label]
		if key == "" {
			continue
		}
		out[key] = sample.Value
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
		if node, err := s.node.GetNode(ctx, device.NodeUid); err == nil {
			gpu.NodeIp = node.IP
		}

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

func (s *CardService) nodeIPByName(ctx context.Context) map[string]string {
	out := map[string]string{}
	nodes, err := s.node.ListAllNodes(ctx)
	if err != nil {
		return out
	}
	for _, node := range nodes {
		if node != nil && node.Name != "" && node.IP != "" {
			out[node.Name] = node.IP
		}
	}
	return out
}
