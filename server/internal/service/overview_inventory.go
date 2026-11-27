package service

import (
	"maps"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/types"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

type overviewInventory struct {
	at                           time.Time
	physical                     resourceview.PhysicalInventory
	nodeCoverage, deviceCoverage *pb.Coverage
}

func observedCount(c *pb.Coverage) *int32 {
	if !c.Complete && c.Observed == 0 {
		return nil
	}
	return proto.Int32(c.Observed)
}

func missingState(source, scope string) *pb.FieldState {
	return &pb.FieldState{State: resourceview.Unknown, Source: source, Scope: scope, Reason: "unverified"}
}

func makeInventory(snapshot resourceview.Snapshot) overviewInventory {
	return overviewInventory{at: snapshot.At, physical: resourceview.GroupPhysicalDevices(snapshot),
		nodeCoverage: snapshot.NodeCoverage, deviceCoverage: snapshot.DeviceCoverage}
}

// Membership and physical identity are shared by the overview and the node
// pages. Callers decide which device fields their response needs to account for.
type inventoryNode struct {
	fact               *resourceview.Node
	ready, schedulable *bool
	states             map[string]*pb.FieldState
	deviceCoverage     *pb.Coverage
	models             map[string]struct{}
	providers          map[string]struct{}
	allocations        *nodeAllocations
	usage              *nodeUsage
}

func (n *inventoryNode) modelNames() []string {
	return slices.Sorted(maps.Keys(n.models))
}

func (n *inventoryNode) providerNames() []string {
	return slices.Sorted(maps.Keys(n.providers))
}

func (inv overviewInventory) nodeRows(uid string, observe func(*inventoryNode, resourceview.PhysicalGroup) string) ([]inventoryNode, *pb.Coverage) {
	coverage := newOverviewCoverage(inv.nodeCoverage, "node", inv.physical.IncompleteNodes)
	capacity := len(inv.physical.AcceleratorNodes)
	if uid != "" {
		capacity = 1
	}
	rows := make([]inventoryNode, 0, capacity)
	selected := make(map[types.UID]int, capacity)
	for i := range inv.physical.AcceleratorNodes {
		n := &inv.physical.AcceleratorNodes[i]
		if uid != "" && string(n.UID) != uid {
			continue
		}
		row := inventoryNode{fact: n, states: map[string]*pb.FieldState{},
			deviceCoverage: newOverviewCoverage(n.DeviceCoverage, "device", inv.physical.UnresolvedOnNode[n.UID])}
		row.ready, row.states["ready"] = nodeFlag(n.Ready, n.States["ready"])
		row.schedulable, row.states["schedulable"] = nodeFlag(n.Schedulable, n.States["schedulable"])
		countRow(coverage, row.states)
		selected[n.UID] = len(rows)
		rows = append(rows, row)
	}
	for _, group := range inv.physical.Groups {
		idx, ok := selected[group.NodeUID]
		if !ok {
			continue
		}
		n := &rows[idx]
		if model := observe(n, group); model != "" {
			if n.models == nil {
				n.models = make(map[string]struct{})
			}
			n.models[model] = struct{}{}
		}
	}
	return rows, coverage
}

func nodeFlag(value *bool, state *pb.FieldState) (*bool, *pb.FieldState) {
	if value != nil && state.GetState() == resourceview.Known {
		return value, state
	}
	if state != nil && state.State != resourceview.Known {
		return nil, state
	}
	return nil, missingState("node_status", "node")
}
