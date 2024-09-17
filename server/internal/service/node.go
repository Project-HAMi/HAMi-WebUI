package service

import (
	"context"
	"fmt"
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

	var res = &pb.NodesReply{List: []*pb.NodeReply{}}
	for _, node := range nodes {
		nodeReply, err := s.buildNodeReply(ctx, node)
		if err != nil {
			return nil, err
		}

		coreTotal, memoryTotal, err := s.queryNodeMetrics(ctx, node.Name)
		if err == nil {
			nodeReply.CoreTotal = coreTotal
			nodeReply.MemoryTotal = memoryTotal
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

	return s.buildNodeReply(ctx, node)
}

func (s *NodeService) buildNodeReply(ctx context.Context, node *biz.Node) (*pb.NodeReply, error) {
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
		vGPU, core, memory, err := s.pod.StatisticsByDeviceId(ctx, device.AliasId)
		if err == nil {
			nodeReply.VgpuUsed += vGPU
			nodeReply.CoreUsed += core
			nodeReply.MemoryUsed += memory
		}
	}

	nodeReply.Type = arrutil.Unique(nodeReply.Type)
	nodeReply.CardCnt = int32(len(node.Devices))

	return nodeReply, nil
}

func (s *NodeService) queryNodeMetrics(ctx context.Context, nodeName string) (int32, int32, error) {
	coreTotal, memoryTotal := int32(0), int32(0)

	resp, err := s.ms.QueryInstant(ctx, &pb.QueryInstantRequest{Query: fmt.Sprintf("avg(hami_core_size{node=~\"%s\"})", nodeName)})
	if err == nil && len(resp.Data) > 0 {
		coreTotal = int32(resp.Data[0].Value)
	}

	resp, err = s.ms.QueryInstant(ctx, &pb.QueryInstantRequest{Query: fmt.Sprintf("avg(hami_memory_size{node=~\"%s\"})", nodeName)})
	if err == nil && len(resp.Data) > 0 {
		memoryTotal = int32(resp.Data[0].Value)
	}

	return coreTotal, memoryTotal, nil
}
