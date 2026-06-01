package service

import (
	"context"
	"sort"
	"strconv"
	"vgpu/internal/biz"

	"github.com/jinzhu/copier"

	pb "vgpu/api/v1"

	"github.com/gookit/goutil/arrutil"
)

type NodeService struct {
	pb.UnimplementedNodeServer

	uc      *biz.NodeUsecase
	pod     *biz.PodUseCase
	summary *biz.SummaryUseCase
	ms      *MonitorService
}

func NewNodeService(uc *biz.NodeUsecase, pod *biz.PodUseCase, summary *biz.SummaryUseCase, ms *MonitorService) *NodeService {
	return &NodeService{uc: uc, pod: pod, summary: summary, ms: ms}
}

func (s *NodeService) GetSummary(ctx context.Context, req *pb.GetSummaryReq) (*pb.DeviceSummaryReply, error) {
	filters := req.Filters
	var res = &pb.DeviceSummaryReply{}
	t, err := s.summary.GetGPUSummary(ctx, filters.DeviceId, filters.NodeUid, filters.Type)
	copier.Copy(&res, &t)
	return res, err
}

func (s *NodeService) GetAllNodes(ctx context.Context, req *pb.GetAllNodesReq) (*pb.NodesReply, error) {
	filters := req.Filters
	nodes, err := s.uc.ListAllNodes(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch containers once (StatisticsByDeviceId used to re-list per device) and
	// pull the per-node core/memory totals in two queries keyed by node, instead
	// of two PromQL per node. This is what made /v1/nodes take 3-4s — and it is
	// also called for the workload page's node filter dropdown.
	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}
	coreByNode := s.queryNodeGauge(ctx, "avg(sum(hami_core_size) by (node, instance)) by (node)")
	memByNode := s.queryNodeGauge(ctx, "avg(sum(hami_memory_size) by (node, instance)) by (node)")

	var res = &pb.NodesReply{List: []*pb.NodeReply{}}
	for _, node := range nodes {
		nodeReply := s.buildNodeReply(node, containers)

		if v, ok := coreByNode[node.Name]; ok {
			nodeReply.CoreTotal = v
		}
		if v, ok := memByNode[node.Name]; ok {
			nodeReply.MemoryTotal = v
		}

		if filters.Ip != "" && filters.Ip != nodeReply.Ip {
			continue
		}
		if filters.Type != "" && !arrutil.InStrings(filters.Type, nodeReply.Type) {
			continue
		}

		result, err := strconv.ParseBool(filters.IsSchedulable)
		if err == nil {
			if result != nodeReply.IsSchedulable {
				continue
			}
		}

		res.List = append(res.List, nodeReply)
	}

	sort.SliceStable(res.List, func(i, j int) bool {
		return res.List[i].Name < res.List[j].Name
	})

	return res, nil
}

func (s *NodeService) GetNode(ctx context.Context, req *pb.GetNodeReq) (*pb.NodeReply, error) {
	node, err := s.uc.GetNode(ctx, req.Uid)
	if err != nil {
		return nil, err
	}

	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}

	return s.buildNodeReply(node, containers), nil
}

// buildNodeReply assembles a node reply from in-memory state only. The caller
// passes a pre-fetched container list so per-device usage stats reuse one scan
// rather than re-listing all containers for each device.
func (s *NodeService) buildNodeReply(node *biz.Node, containers []*biz.Container) *pb.NodeReply {
	nodeReply := &pb.NodeReply{
		Name:                    node.Name,
		Uid:                     node.Uid,
		Ip:                      node.IP,
		IsSchedulable:           node.IsSchedulable,
		IsReady:                 node.IsReady,
		OsImage:                 node.OSImage,
		OperatingSystem:         node.OperatingSystem,
		KernelVersion:           node.KernelVersion,
		ContainerRuntimeVersion: node.ContainerRuntimeVersion,
		KubeletVersion:          node.KubeletVersion,
		KubeProxyVersion:        node.KubeProxyVersion,
		Architecture:            node.Architecture,
		CreationTimestamp:       node.CreationTimestamp,
	}

	for _, device := range node.Devices {
		nodeReply.Type = append(nodeReply.Type, device.Type)
		nodeReply.VgpuTotal += device.Count
		nodeReply.CoreTotal += device.Devcore
		nodeReply.MemoryTotal += device.Devmem
		vGPU, core, memory := biz.ContainersStatisticsInfo(containers, device.AliasId)
		nodeReply.VgpuUsed += vGPU
		nodeReply.CoreUsed += core
		nodeReply.MemoryUsed += memory
	}

	nodeReply.Type = arrutil.Unique(nodeReply.Type)
	nodeReply.CardCnt = int32(len(node.Devices))

	return nodeReply
}

// queryNodeGauge runs a single instant query whose result is grouped by the
// "node" label and returns value-by-node, batching what used to be two PromQL
// per node into one round-trip.
func (s *NodeService) queryNodeGauge(ctx context.Context, query string) map[string]int32 {
	out := map[string]int32{}
	resp, err := s.ms.QueryInstant(ctx, &pb.QueryInstantRequest{Query: query})
	if err != nil {
		return out
	}
	for _, sample := range resp.Data {
		node := sample.Metric["node"]
		if node == "" {
			continue
		}
		out[node] = int32(sample.Value)
	}
	return out
}
