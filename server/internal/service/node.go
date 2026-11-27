package service

import (
	"context"
	"sort"
	"strconv"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/gookit/goutil/arrutil"
	"github.com/jinzhu/copier"
	"google.golang.org/protobuf/proto"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

type NodeService struct {
	pb.UnimplementedNodeServer

	uc      *biz.NodeUsecase
	pod     *biz.PodUseCase
	summary *biz.SummaryUseCase
	source  resourceview.SnapshotSource
}

func NewNodeService(uc *biz.NodeUsecase, pod *biz.PodUseCase, summary *biz.SummaryUseCase, source *resourceview.ObservedSnapshotSource) *NodeService {
	var snapSource resourceview.SnapshotSource
	if source != nil {
		snapSource = source
	}
	return &NodeService{uc: uc, pod: pod, summary: summary, source: snapSource}
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
	if req.GetUseResourceInventory() {
		return s.inventoryNodeRows(ctx, "", req.GetFilters())
	}
	filters := req.GetFilters()
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
	reservations := biz.AggregateDeviceReservations(containers)

	var res = &pb.NodesReply{List: []*pb.NodeReply{}}
	for _, node := range nodes {
		nodeReply := s.buildNodeReply(node, reservations)

		if !matchesNodeFilters(nodeReply, filters) {
			continue
		}
		res.List = append(res.List, nodeReply)
	}

	sort.SliceStable(res.List, func(i, j int) bool {
		return res.List[i].Name < res.List[j].Name
	})

	return res, nil
}

func matchesNodeFilters(node *pb.NodeReply, filters *pb.GetAllNodesReq_Filters) bool {
	if ip := filters.GetIp(); ip != "" && ip != node.Ip {
		return false
	}
	if model := filters.GetType(); model != "" && !arrutil.InStrings(model, node.Type) {
		return false
	}
	schedulable, err := strconv.ParseBool(filters.GetIsSchedulable())
	return err != nil || (node.IsSchedulable != nil && *node.IsSchedulable == schedulable)
}

func (s *NodeService) GetNode(ctx context.Context, req *pb.GetNodeReq) (*pb.NodeReply, error) {
	if req.GetUseResourceInventory() {
		if req.GetUid() == "" {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", "Missing node UID").WithMetadata(map[string]string{"param": "uid"})
		}
		nodes, err := s.inventoryNodeRows(ctx, req.GetUid(), nil)
		if err != nil {
			return nil, err
		}
		if len(nodes.List) == 0 {
			return nil, kratoserrors.NotFound("RESOURCE_NODE_NOT_FOUND", "Accelerator node was not found")
		}
		return nodes.List[0], nil
	}
	node, err := s.uc.GetNode(ctx, req.Uid)
	if err != nil {
		return nil, err
	}

	containers, err := s.pod.ListAllContainers(ctx)
	if err != nil {
		return nil, err
	}

	return s.buildNodeReply(node, biz.AggregateDeviceReservations(containers)), nil
}

// buildNodeReply assembles a node reply from in-memory state only. The caller
// passes pre-aggregated per-device reservations so per-device lookups stay O(1)
// rather than rescanning all containers for every device.
func (s *NodeService) buildNodeReply(node *biz.Node, reservations map[string]biz.DeviceReservation) *pb.NodeReply {
	coreUsedKnown := true
	nodeReply := &pb.NodeReply{
		Name:                    node.Name,
		Uid:                     node.Uid,
		Ip:                      node.IP,
		IsSchedulable:           proto.Bool(node.IsSchedulable),
		IsReady:                 proto.Bool(node.IsReady),
		OsImage:                 node.OSImage,
		OperatingSystem:         node.OperatingSystem,
		KernelVersion:           node.KernelVersion,
		ContainerRuntimeVersion: node.ContainerRuntimeVersion,
		KubeletVersion:          node.KubeletVersion,
		KubeProxyVersion:        node.KubeProxyVersion,
		Architecture:            node.Architecture,
		CreationTimestamp:       node.CreationTimestamp,
	}

	var vgpuTotal, coreTotal, memoryTotal, vgpuUsed, coreUsed, memoryUsed int32
	for _, device := range node.Devices {
		nodeReply.Type = append(nodeReply.Type, device.Type)
		if !device.Unconfigured {
			vgpuTotal += device.Count
			coreTotal += biz.PhysicalCoreBaselinePerDevice
			memoryTotal += device.Devmem
		}
		reservation := reservations[device.AliasId]
		vgpuUsed += reservation.Slots
		coreUsed += reservation.Cores
		if reservation.CoreUnknown {
			coreUsedKnown = false
		}
		memoryUsed += reservation.MemoryMiB
	}
	nodeReply.CoreUsedKnown = &coreUsedKnown

	nodeReply.Type = arrutil.Unique(nodeReply.Type)
	nodeReply.CardCnt = proto.Int32(int32(len(node.Devices)))
	nodeReply.VgpuTotal, nodeReply.CoreTotal, nodeReply.MemoryTotal = &vgpuTotal, &coreTotal, &memoryTotal
	nodeReply.VgpuUsed, nodeReply.CoreUsed, nodeReply.MemoryUsed = &vgpuUsed, &coreUsed, &memoryUsed

	return nodeReply
}
