package service

import (
	"context"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

type OverviewService struct {
	source resourceview.SnapshotSource
}

func NewOverviewService(source *resourceview.ObservedSnapshotSource) *OverviewService {
	return &OverviewService{source: source}
}

func (s *OverviewService) GetInventory(ctx context.Context, _ *pb.InventoryRequest) (*pb.InventoryReply, error) {
	snapshot, err := s.source.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return makeInventory(snapshot).compactInventory(), nil
}

// The homepage joins cards to node incarnations. Both sets must come from
// one source read, even when successive requests go to different replicas.
func (inv overviewInventory) compactInventory() *pb.InventoryReply {
	reply := &pb.InventoryReply{
		SnapshotAt:     timestamppb.New(inv.at),
		DeviceCoverage: newOverviewCoverage(inv.deviceCoverage, "device", inv.physical.IncompleteDevices),
		Devices:        make([]*pb.InventoryDevice, 0, len(inv.physical.Groups)),
	}
	nodes, coverage := inv.nodeRows("", func(n *inventoryNode, g resourceview.PhysicalGroup) string {
		d := compactDevice(g)
		reply.Devices = append(reply.Devices, d)
		countRow(reply.DeviceCoverage, nil)
		countRow(n.deviceCoverage, nil)
		return d.Model
	})
	reply.NodeCoverage = coverage
	reply.Nodes = make([]*pb.InventoryNode, 0, len(nodes))
	for _, n := range nodes {
		reply.Nodes = append(reply.Nodes, &pb.InventoryNode{Name: n.fact.Name, Uid: string(n.fact.UID), Ready: n.ready,
			Schedulable: n.schedulable, Models: n.modelNames(), PhysicalDevices: observedCount(n.deviceCoverage)})
	}
	return reply
}

func compactDevice(group resourceview.PhysicalGroup) *pb.InventoryDevice {
	id := group.Members[0].Identity
	model, _ := group.Model()
	healthy, _ := group.Health()
	d := &pb.InventoryDevice{Node: id.Node, Provider: id.Provider, DeviceId: id.DeviceID, Model: model, Healthy: healthy}
	sample := group.Observation(deviceObservation)
	if ratio, ok := sample.UtilizationRatio.KnownValue(); ok {
		d.UtilizationRatio = &ratio
	}
	// Observation returns both byte counts only if the same sample supplied
	// both. Compare the exact integers before converting to a display ratio.
	used, total := sample.MemoryUsedBytes.Value, sample.MemoryPhysicalBytes.Value
	if used != nil && total != nil && total.Sign() > 0 && used.Cmp(*total) <= 0 {
		d.MemoryUsageRatio = proto.Float64(float64(used.Value()) / float64(total.Value()))
	}
	return d
}

func deviceObservation(d resourceview.Device) resourceview.DeviceObservation {
	return resourceview.DeviceObservation{MemoryUsedBytes: d.MemoryUsedBytes, MemoryPhysicalBytes: d.MemoryPhysicalBytes, UtilizationRatio: d.UtilizationRatio}
}
