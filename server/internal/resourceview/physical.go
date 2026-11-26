package resourceview

import (
	"cmp"
	"slices"

	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
)

// PhysicalGroup represents one physical device on one incarnation of a Node.
// The first member is its deterministic link/metric identity. Different
// providers share a group only with a proven hardware identity.
type PhysicalGroup struct {
	NodeUID types.UID
	UUID    string
	Members []Device
}

type PhysicalInventory struct {
	Groups                             []PhysicalGroup
	AcceleratorNodes                   []Node
	UnresolvedOnNode                   map[types.UID]bool
	IncompleteDevices, IncompleteNodes bool
}

type physicalKey struct {
	nodeUID                                 types.UID
	uuid, domain, boardKey, provider, device string
}

// GroupPhysicalDevices applies the same identity rules to the public inventory
// and exported metrics. Native accelerator resource units establish a node's
// membership but never create physical devices.
func GroupPhysicalDevices(snapshot Snapshot) PhysicalInventory {
	nodes := make(map[types.UID]Node, len(snapshot.Nodes))
	nodeUIDs := map[string]types.UID{}
	invalidUID := map[types.UID]bool{}
	missingNode := false
	for _, n := range snapshot.Nodes {
		if n.Name == "" || n.UID == "" {
			missingNode = true
			continue
		}
		if old, ok := nodeUIDs[n.Name]; ok {
			invalidUID[old], invalidUID[n.UID] = true, true
		}
		if _, ok := nodes[n.UID]; ok {
			invalidUID[n.UID] = true
		}
		nodeUIDs[n.Name], nodes[n.UID] = n.UID, n
	}

	for uid := range invalidUID {
		delete(nodes, uid)
		missingNode = true
	}

	registered := map[types.UID]bool{}
	missingOnNode := map[types.UID]bool{}
	missingDevice := false
	// The source ID must identify one device even when several providers can
	// prove the same physical UUID. Conflicting UUIDs for an ID are ambiguous.
	keys := map[DeviceIdentity]physicalKey{}
	conflicting := map[DeviceIdentity]bool{}
	for _, d := range snapshot.Devices {
		id := d.Identity
		n, ok := nodes[id.NodeUID]
		if !ok || n.Name != id.Node {
			missingDevice = true
			continue
		}
		registered[id.NodeUID] = true
		if d.Kind != "physical" || d.States["kind"].GetState() != Known {
			continue
		}
		if _, err := DeviceID(id.Provider, id.Node, id.DeviceID); err != nil {
			continue
		}
		key, exists := keys[id]
		if !exists {
			if d.Physical.Domain != "" && d.Physical.BoardKey != "" {
				key = physicalKey{nodeUID: id.NodeUID, domain: d.Physical.Domain, boardKey: d.Physical.BoardKey}
			} else {
				key = physicalKey{nodeUID: id.NodeUID, provider: id.Provider, device: id.DeviceID}
			}
		}
		if d.UUID != nil && *d.UUID != "" && d.States["uuid"].GetState() == Known {
			if key.uuid != "" && key.uuid != *d.UUID {
				conflicting[id] = true
			}
			key = physicalKey{nodeUID: id.NodeUID, uuid: *d.UUID}
		}
		keys[id] = key
	}
	for id := range conflicting {
		delete(keys, id)
	}

	maxDomainPriority := map[types.UID]map[string]int{}
	for _, d := range snapshot.Devices {
		id := d.Identity
		if _, ok := keys[id]; !ok || d.Physical.Domain == "" {
			continue
		}
		perNode := maxDomainPriority[id.NodeUID]
		if perNode == nil {
			perNode = map[string]int{}
			maxDomainPriority[id.NodeUID] = perNode
		}
		if prev, seen := perNode[d.Physical.Domain]; !seen || d.Physical.Priority > prev {
			perNode[d.Physical.Domain] = d.Physical.Priority
		}
	}

	groups := map[physicalKey][]Device{}
	for _, d := range snapshot.Devices {
		if key, ok := keys[d.Identity]; ok && d.Kind == "physical" && d.States["kind"].GetState() == Known {
			// A slot-only registration cannot prove an additional physical board
			// when a higher-priority generation also registers boards on the node.
			if d.Physical.Domain != "" && d.Physical.BoardKey == "" && key.uuid == "" {
				if maxP, hasDomain := maxDomainPriority[d.Identity.NodeUID][d.Physical.Domain]; hasDomain && maxP > d.Physical.Priority {
					missingOnNode[d.Identity.NodeUID] = true
					continue
				}
			}
			groups[key] = append(groups[key], d)
			continue
		}
		if d.ParentIdentity != nil && d.States["parentId"].GetState() == Known {
			if _, ok := keys[*d.ParentIdentity]; ok {
				continue
			}
		}
		missingOnNode[d.Identity.NodeUID] = true
	}

	out := PhysicalInventory{UnresolvedOnNode: missingOnNode, IncompleteNodes: missingNode, IncompleteDevices: missingDevice || len(missingOnNode) > 0}
	for key, members := range groups {
		slices.SortFunc(members, func(a, b Device) int {
			return cmp.Or(cmp.Compare(b.Physical.Priority, a.Physical.Priority), compareDevices(a, b))
		})
		out.Groups = append(out.Groups, PhysicalGroup{NodeUID: key.nodeUID, UUID: key.uuid, Members: members})
	}
	slices.SortFunc(out.Groups, func(a, b PhysicalGroup) int { return compareDevices(a.Members[0], b.Members[0]) })
	for _, n := range nodes {
		if registered[n.UID] || hasAcceleratorResource(n) {
			out.AcceleratorNodes = append(out.AcceleratorNodes, n)
		}
	}
	slices.SortFunc(out.AcceleratorNodes, func(a, b Node) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func compareDevices(a, b Device) int {
	return cmp.Or(cmp.Compare(a.Identity.Node, b.Identity.Node), cmp.Compare(a.Identity.Provider, b.Identity.Provider), cmp.Compare(a.Identity.DeviceID, b.Identity.DeviceID))
}

func hasAcceleratorResource(node Node) bool {
	for _, r := range node.Resources {
		for _, q := range []Quantity{r.Capacity, r.Allocatable} {
			if q.State.GetState() == Known && q.Value != nil && q.Value.Sign() > 0 {
				return true
			}
		}
	}
	return false
}

// When multiple generations register the same physical board, prefer the highest-priority
// registration for display and telemetry while retaining all members for allocations.
func (g PhysicalGroup) reportingMembers() (primary, fallback []Device) {
	if len(g.Members) < 2 {
		return g.Members, nil
	}
	maxPriority := g.Members[0].Physical.Priority
	for _, member := range g.Members[1:] {
		if member.Physical.Priority > maxPriority {
			maxPriority = member.Physical.Priority
		}
	}
	for _, member := range g.Members {
		if member.Physical.Priority == maxPriority {
			primary = append(primary, member)
		} else {
			fallback = append(fallback, member)
		}
	}
	if len(fallback) == 0 {
		return g.Members, nil
	}
	return primary, fallback
}

// Model resolves the model across registrations for the same physical device.
func (group PhysicalGroup) Model() (string, *viewv1.FieldState) {
	primary, _ := group.reportingMembers()
	model, state := agreedField(primary, func(d *Device) (*string, *viewv1.FieldState) {
		if d.Model == "" {
			return nil, d.States["model"]
		}
		return &d.Model, d.States["model"]
	})
	if model == nil {
		return "", state
	}
	return *model, state
}

// Health reports the agreed health status across the device's registrations.
func (group PhysicalGroup) Health() (*bool, *viewv1.FieldState) {
	primary, _ := group.reportingMembers()
	return agreedField(primary, func(d *Device) (*bool, *viewv1.FieldState) {
		return d.Healthy, d.States["health"]
	})
}

func agreedField[T comparable](aliases []Device, read func(*Device) (*T, *viewv1.FieldState)) (*T, *viewv1.FieldState) {
	var value *T
	var known, unavailable *viewv1.FieldState
	for i := range aliases {
		v, s := read(&aliases[i])
		if s.GetState() == Conflict {
			return nil, s
		}
		if v == nil || s.GetState() != Known {
			if unavailable == nil && s != nil && s.State != Known {
				unavailable = s
			}
			continue
		}
		if value != nil && *value != *v {
			return nil, &viewv1.FieldState{State: Conflict, Source: "physical_identity", Scope: "device", Reason: "identity_conflict"}
		}
		if value == nil {
			value, known = v, s
		}
	}
	if value != nil {
		return value, known
	}
	if unavailable != nil {
		return nil, unavailable
	}
	return nil, &viewv1.FieldState{State: Unknown, Source: "inventory", Scope: "device", Reason: "unverified"}
}
