package service

import (
	"context"
	"math"

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
	var summary summaryAccumulator
	nodes, coverage := inv.nodeRows("", func(n *inventoryNode, g resourceview.PhysicalGroup) string {
		sample := g.Observation(deviceObservation)
		d := compactDevice(g, sample)
		reply.Devices = append(reply.Devices, d)
		countRow(reply.DeviceCoverage, nil)
		countRow(n.deviceCoverage, nil)
		summary.add(g, sample, d.UtilizationRatio)
		return d.Model
	})
	reply.NodeCoverage = coverage
	reply.Summary = summary.build()
	reply.Nodes = make([]*pb.InventoryNode, 0, len(nodes))
	for _, n := range nodes {
		reply.Nodes = append(reply.Nodes, &pb.InventoryNode{Name: n.fact.Name, Uid: string(n.fact.UID), Ready: n.ready,
			Schedulable: n.schedulable, Models: n.modelNames(), PhysicalDevices: observedCount(n.deviceCoverage)})
	}
	return reply
}

type summaryAccumulator struct {
	computeAllocatedSum, computeCapacitySum float64
	computeAllocCount                       int
	memoryAllocatedSum, memoryCapacitySum   float64
	memoryAllocCount                        int
	utilSum                                 float64
	utilCount                               int
	memoryUsedSum, memoryPhysicalSum        float64
	memoryUsageCount                        int
}

func validFiniteNonNeg(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func (a *summaryAccumulator) add(group resourceview.PhysicalGroup, sample resourceview.DeviceObservation, utilRatio *float64) {
	if totals, valid := group.Occupancy(); valid && len(group.Members) == 1 {
		member := group.Members[0]
		if totals.Compute.Unknown == 0 {
			alloc := totals.Compute.Sum.AsApproximateFloat64()
			if capVal, ok := member.ComputeCapacityShare.KnownValue(); ok && capVal > 0 && validFiniteNonNeg(alloc) {
				a.computeAllocatedSum += alloc
				a.computeCapacitySum += capVal
				a.computeAllocCount++
			}
		}
		if totals.Memory.Unknown == 0 {
			alloc := totals.Memory.Sum.AsApproximateFloat64()
			if capVal, ok := member.MemoryCapacityBytes.KnownValue(); ok && capVal > 0 && validFiniteNonNeg(alloc) {
				a.memoryAllocatedSum += alloc
				a.memoryCapacitySum += capVal
				a.memoryAllocCount++
			}
		}
	}
	if utilRatio != nil && validFiniteNonNeg(*utilRatio) {
		a.utilSum += *utilRatio
		a.utilCount++
	}
	used, total := sample.MemoryUsedBytes.Value, sample.MemoryPhysicalBytes.Value
	if used != nil && total != nil && total.Sign() > 0 && used.Sign() >= 0 && used.Cmp(*total) <= 0 {
		usedVal, totalVal := float64(used.Value()), float64(total.Value())
		if validFiniteNonNeg(usedVal) && validFiniteNonNeg(totalVal) && totalVal > 0 {
			a.memoryUsedSum += usedVal
			a.memoryPhysicalSum += totalVal
			a.memoryUsageCount++
		}
	}
}

func (a *summaryAccumulator) build() *pb.InventoryResourceSummary {
	if a.computeAllocCount == 0 && a.memoryAllocCount == 0 && a.utilCount == 0 && a.memoryUsageCount == 0 {
		return nil
	}
	out := &pb.InventoryResourceSummary{}
	if a.computeAllocCount > 0 {
		out.ComputeAllocatedRatio = proto.Float64(a.computeAllocatedSum)
		out.ComputeCapacityRatio = proto.Float64(a.computeCapacitySum)
	}
	if a.memoryAllocCount > 0 {
		out.MemoryAllocatedBytes = proto.Float64(a.memoryAllocatedSum)
		out.MemoryCapacityBytes = proto.Float64(a.memoryCapacitySum)
	}
	if a.utilCount > 0 {
		out.UtilizationRatio = proto.Float64(a.utilSum / float64(a.utilCount))
	}
	if a.memoryUsageCount > 0 {
		out.MemoryUsedBytes = proto.Float64(a.memoryUsedSum)
		out.MemoryPhysicalBytes = proto.Float64(a.memoryPhysicalSum)
	}
	return out
}

func compactDevice(group resourceview.PhysicalGroup, sample resourceview.DeviceObservation) *pb.InventoryDevice {
	id := group.Members[0].Identity
	model, _ := group.Model()
	healthy, _ := group.Health()
	d := &pb.InventoryDevice{Node: id.Node, Provider: id.Provider, DeviceId: id.DeviceID, Model: model, Healthy: healthy}
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
