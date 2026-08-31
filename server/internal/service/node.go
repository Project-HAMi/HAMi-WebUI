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
}

func NewNodeService(uc *biz.NodeUsecase, pod *biz.PodUseCase, summary *biz.SummaryUseCase) *NodeService {
	return &NodeService{uc: uc, pod: pod, summary: summary}
}

func (s *NodeService) GetSummary(ctx context.Context, req *pb.GetSummaryReq) (*pb.DeviceSummaryReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetSummaryReq_Filters{}
	}
	var res = &pb.DeviceSummaryReply{}
	t, err := s.summary.GetGPUSummary(ctx, filters.DeviceId, filters.NodeUid, filters.Type)
	copier.Copy(&res, &t)
	res.CoreUsedKnown = &t.CoreUsedKnown
	return res, err
}

func (s *NodeService) GetAllNodes(ctx context.Context, req *pb.GetAllNodesReq) (*pb.NodesReply, error) {
	filters := req.GetFilters()
	if filters == nil {
		filters = &pb.GetAllNodesReq_Filters{}
	}
	nodes, err := s.uc.ListAllNodes(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch containers once (StatisticsByDeviceId used to re-list per device).
	// Memory capacity already comes from DeviceInfo.Devmem, HAMi's registered
	// schedulable value. Compute capacity is the deterministic physical baseline
	// assembled from the same inventory below.
	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}

	var res = &pb.NodesReply{List: []*pb.NodeReply{}}
	for _, node := range nodes {
		nodeReply := s.buildNodeReply(node, containers)

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
	coreUsedKnown := true
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
		nodeReply.CoreTotal += biz.PhysicalCoreBaselinePerDevice
		nodeReply.MemoryTotal += device.Devmem
		vGPU, core, memory, coreKnown := biz.ContainersStatisticsInfo(containers, device.AliasId)
		nodeReply.VgpuUsed += vGPU
		nodeReply.CoreUsed += core
		if !coreKnown {
			coreUsedKnown = false
		}
		nodeReply.MemoryUsed += memory
	}
	nodeReply.CoreUsedKnown = &coreUsedKnown

	nodeReply.Type = arrutil.Unique(nodeReply.Type)
	nodeReply.CardCnt = int32(len(node.Devices))

	return nodeReply
}
