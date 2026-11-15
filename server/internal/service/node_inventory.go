package service

import (
	"context"
	"math"
	"math/big"
	"strings"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	pb "vgpu/api/v1"
	"vgpu/internal/resourceview"
)

// Only the node pages opt in. Membership, hardware counts and allocations all
// come from one Kubernetes snapshot, including nodes without a legacy provider.
func (s *NodeService) inventoryNodeRows(ctx context.Context, uid string, filters *pb.GetAllNodesReq_Filters) (*pb.NodesReply, error) {
	snapshot, err := s.source.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	inv := makeInventory(snapshot)
	nodes, coverage := inv.nodeRows(uid, observeNodeAllocation)
	reply := &pb.NodesReply{InventoryCoverage: coverage}
	filtered := filters.GetIp() != "" || filters.GetType() != "" || filters.GetIsSchedulable() != ""
	if filtered {
		reply.InventoryCoverage = newOverviewCoverage(coverage, "node", false)
	}
	for _, node := range nodes {
		n := node.fact
		row := &pb.NodeReply{
			Uid:                     string(n.UID),
			Name:                    n.Name,
			IsReady:                 node.ready,
			IsSchedulable:           node.schedulable,
			CardCnt:                 observedCount(node.deviceCoverage),
			Type:                    node.modelNames(),
			InventoryCoverage:       node.deviceCoverage,
			Ip:                      n.InternalIP,
			OsImage:                 n.SystemInfo.OSImage,
			OperatingSystem:         n.SystemInfo.OperatingSystem,
			Architecture:            strings.ToUpper(n.SystemInfo.Architecture),
			KernelVersion:           n.SystemInfo.KernelVersion,
			KubeletVersion:          n.SystemInfo.KubeletVersion,
			KubeProxyVersion:        n.SystemInfo.KubeProxyVersion,
			ContainerRuntimeVersion: n.SystemInfo.ContainerRuntimeVersion,
		}
		if n.CreatedAt != nil {
			row.CreationTimestamp = n.CreatedAt.Format("2006-01-02 15:04:05")
		}
		if !matchesNodeFilters(row, filters) {
			continue
		}
		if node.allocations != nil {
			node.allocations.fill(row)
		}
		reply.List = append(reply.List, row)
		if filtered {
			countRow(reply.InventoryCoverage, node.states)
		}
	}
	if len(reply.List) == 0 && !reply.InventoryCoverage.Complete {
		return nil, kratoserrors.ServiceUnavailable("NODE_INVENTORY_UNAVAILABLE", "Accelerator node inventory is incomplete")
	}
	return reply, nil
}

func observeNodeAllocation(n *inventoryNode, group resourceview.PhysicalGroup) string {
	// No telemetry field histograms are required by the node pages.
	countRow(n.deviceCoverage, nil)
	model, _ := group.Model()
	// Schedulable capacity belongs to a single configured registration.
	if len(group.Members) != 1 {
		return model
	}
	device := &group.Members[0]
	if device.Configured == nil || !*device.Configured {
		return model
	}
	if n.allocations == nil {
		n.allocations = &nodeAllocations{}
	}
	used, covered := group.Occupancy()
	n.allocations.slots.add(device.SlotsCapacity, used.Slots, covered)
	n.allocations.compute.add(device.ComputeCapacityShare, used.Compute, covered)
	n.allocations.memory.add(device.MemoryCapacityBytes, used.Memory, covered)
	return model
}

type nodeAllocations struct{ slots, compute, memory nodeAllocation }
type nodeAllocation struct {
	totalCapacity, pairedCapacity, used resource.Quantity
	hasCapacity                         bool
}

func (p *nodeAllocation) add(capacity resourceview.Quantity, used resourceview.OccupancyTotal, covered bool) {
	if _, known := capacity.KnownValue(); !known {
		return
	}
	p.hasCapacity = true
	p.totalCapacity.Add(*capacity.Value)
	// The allocation percentage uses the same positive, paired capacities as the overview.
	if covered && used.Unknown == 0 && capacity.Value.Sign() > 0 {
		p.used.Add(used.Sum)
		p.pairedCapacity.Add(*capacity.Value)
	}
}

// Legacy callers express compute in integer percentage points and memory in
// MiB. A fractional or overflowing result cannot be represented in those fields.
func wholeInt32(value resource.Quantity, multiply, divide int64) *int32 {
	v := value.DeepCopy()
	v.Mul(multiply)
	if v.CmpInt64(math.MaxInt32*divide) > 0 {
		return nil
	}
	n := v.Value() / divide
	if v.CmpInt64(n*divide) != 0 {
		return nil
	}
	return proto.Int32(int32(n))
}

func (p nodeAllocation) amounts(multiply, divide int64) (*int32, *int32) {
	if !p.hasCapacity {
		return nil, nil
	}
	total := wholeInt32(p.totalCapacity, multiply, divide)
	if p.pairedCapacity.IsZero() {
		return nil, total
	}
	return wholeInt32(p.used, multiply, divide), total
}

func (p nodeAllocation) percent() *float64 {
	if p.pairedCapacity.IsZero() {
		return nil
	}
	// Keep the two accumulated decimal quantities exact through division.
	// Converting them separately can push 92 GiB / 640 GiB below 14.375%.
	var used, capacity big.Rat
	used.SetString(p.used.AsDec().String())
	capacity.SetString(p.pairedCapacity.AsDec().String())
	used.Mul(&used, big.NewRat(100, 1))
	percent, _ := used.Quo(&used, &capacity).Float64()
	return &percent
}

func (a *nodeAllocations) fill(row *pb.NodeReply) {
	row.VgpuUsed, row.VgpuTotal = a.slots.amounts(1, 1)
	row.CoreUsed, row.CoreTotal = a.compute.amounts(100, 1)
	row.MemoryUsed, row.MemoryTotal = a.memory.amounts(1, 1024*1024)
	row.CoreAllocationPercent, row.MemoryAllocationPercent = a.compute.percent(), a.memory.percent()
	if row.CoreUsed != nil {
		row.CoreUsedKnown = proto.Bool(true)
	}
}
