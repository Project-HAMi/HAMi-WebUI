// Package providerfacts assembles the new view from Kubernetes facts only.
// It never turns telemetry-only identities into schedulable inventory.
package providerfacts

import (
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func known(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Known, Source: source, Scope: scope}
}
func unknown(source, scope, reason string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Unknown, Source: source, Scope: scope, Reason: reason}
}
func absent(unit, source, scope, reason string) resourceview.Quantity {
	return resourceview.Quantity{Unit: unit, State: unknown(source, scope, reason)}
}
func number(value int64, unit, source, scope string) resourceview.Quantity {
	return resourceview.Quantity{Value: resource.NewQuantity(value, resource.DecimalSI), Unit: unit, State: known(source, scope)}
}
func coverage(entity, source string, observed int) *viewv1.Coverage {
	return &viewv1.Coverage{Entity: entity, Observed: int32(observed), Sources: []*viewv1.SourceState{{Source: source, State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"continuity": "not_tracked"}}}}
}

// Build assembles a detached Kubernetes inventory. Node continuity and allocator
// completeness are checked independently. A nil proof leaves coverage unknown.
func Build(nodes []*corev1.Node, catalog *devicecatalog.Snapshot, pods podfacts.Inventory, selectors map[string]labels.Selector, logger *log.Helper, at time.Time, nodeEvidence *viewv1.Coverage, dialects []provider.InventoryDialect) resourceview.Snapshot {
	out := resourceview.Snapshot{At: at, Nodes: []resourceview.Node{}, Devices: []resourceview.Device{}}
	for _, node := range nodes {
		if node == nil || node.Name == "" || node.UID == "" {
			continue
		}
		fact := nodeFact(node, catalog, dialects)

		for _, dialect := range dialects {
			if dialect == nil {
				continue
			}
			selected := true
			if key := dialect.SelectorKey(); key != "" {
				selector, ok := selectors[key]
				selected = ok && selector != nil && selector.Matches(labels.Set(node.Labels))
			}
			inv := dialect.ExtractNode(node, catalog, selected, logger, at)
			appendDialectInventory(&out, &fact, inv)
		}

		out.Nodes = append(out.Nodes, fact)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].Name < out.Nodes[j].Name })
	sort.Slice(out.Devices, func(i, j int) bool {
		a, b := out.Devices[i].Identity, out.Devices[j].Identity
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.DeviceID < b.DeviceID
	})
	bindingIndex := buildDeviceBindingIndex(out.Devices)
	attachAllocations(&out, pods, bindingIndex)
	if nodeEvidence == nil {
		nodeEvidence = coverage("node", "shared_node_informer", len(out.Nodes))
	}
	applySnapshotCoverage(&out, nodes, catalog, pods, nodeEvidence, bindingIndex, dialects)
	return out
}
func nodeFact(node *corev1.Node, catalog *devicecatalog.Snapshot, dialects []provider.InventoryDialect) resourceview.Node {
	schedulable := !node.Spec.Unschedulable
	f := resourceview.Node{
		UID: node.UID, Name: node.Name, Schedulable: &schedulable, SystemInfo: node.Status.NodeInfo,
		States:  map[string]*viewv1.FieldState{},
		Sources: []*viewv1.SourceState{{Source: "kubernetes_node", State: resourceview.Known}},
	}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			f.InternalIP = address.Address
			break
		}
	}
	f.States["schedulable"] = known("node_spec", "node")
	f.States["ready"] = unknown("node_status", "node", "unverified")
	f.States["createdAt"] = unknown("node_metadata", "node", "unverified")
	if !node.CreationTimestamp.IsZero() {
		created := node.CreationTimestamp.Time
		f.CreatedAt = &created
		f.States["createdAt"] = known("node_metadata", "node")
	}
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue || c.Status == corev1.ConditionFalse {
				ready := c.Status == corev1.ConditionTrue
				f.Ready = &ready
				f.States["ready"] = known("node_status", "node")
			}
			break
		}
	}
	names := map[corev1.ResourceName]bool{}
	for name := range node.Status.Capacity {
		if acceleratorResource(string(name), catalog, dialects) {
			names[name] = true
		}
	}
	for name := range node.Status.Allocatable {
		if acceleratorResource(string(name), catalog, dialects) {
			names[name] = true
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, string(name))
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		r := resourceview.NodeResource{
			Resource:    name,
			Capacity:    absent("resource_units", "node_status", "node", "unverified"),
			Allocatable: absent("resource_units", "node_status", "node", "unverified"),
		}
		if q, exists := node.Status.Capacity[corev1.ResourceName(name)]; exists && q.Sign() >= 0 {
			v := q.DeepCopy()
			r.Capacity = resourceview.Quantity{Value: &v, Unit: "resource_units", State: known("node_status", "node")}
		}
		if q, exists := node.Status.Allocatable[corev1.ResourceName(name)]; exists && q.Sign() >= 0 {
			v := q.DeepCopy()
			r.Allocatable = resourceview.Quantity{Value: &v, Unit: "resource_units", State: known("node_status", "node")}
		}
		f.Resources = append(f.Resources, r)
	}
	return f
}
func acceleratorResource(name string, catalog *devicecatalog.Snapshot, dialects []provider.InventoryDialect) bool {
	if !strings.Contains(name, "/") || strings.Contains(name, corev1.ResourceDefaultNamespacePrefix) {
		return false
	}
	for _, dialect := range dialects {
		if dialect != nil && dialect.OwnsResource(name, catalog) {
			return true
		}
	}
	if !catalog.Loaded() {
		return false
	}
	if catalog.NodeResourceNames[name] {
		return true
	}
	for _, names := range catalog.ResourceNames {
		if name == names.Count || name == names.Memory || name == names.Compute {
			return true
		}
	}
	return false
}
func percentShare(q resourceview.Quantity) resourceview.Quantity {
	q = q.DeepCopy()
	q.Unit = "share"
	if q.Value == nil {
		return q
	}
	original, ok := new(big.Rat).SetString(q.Value.AsDec().String())
	if !ok {
		return absent("share", "allocation", "device", "unverified")
	}
	ratio := new(big.Rat).Quo(original, big.NewRat(100, 1))
	decimal := ratio.FloatString(9)
	exact, _ := new(big.Rat).SetString(decimal)
	if exact == nil || exact.Cmp(ratio) != 0 {
		return absent("share", "allocation", "device", "unverified")
	}
	value, err := resource.ParseQuantity(decimal)
	if err != nil {
		return absent("share", "allocation", "device", "unverified")
	}
	q.Value = &value
	return q
}
func allocationQuantity(values map[string]podfacts.Quantity, key, unit string) resourceview.Quantity {
	if value, ok := values[key]; ok {
		value = value.DeepCopy()
		if key == "compute" {
			if value.Unit != "percent" && value.Value != nil {
				return absent(unit, "allocation", "device", "unverified")
			}
			return percentShare(value)
		}
		if value.Unit != unit && value.Value != nil {
			return absent(unit, "allocation", "device", "unverified")
		}
		value.Unit = unit
		return value
	}
	return absent(unit, "allocation", "device", "unverified")
}

type deviceKey struct {
	nodeUID          types.UID
	node, source, id string
}

type deviceBinding struct {
	index     int
	allowRaw  bool
	ambiguous bool
}

func buildDeviceBindingIndex(devices []resourceview.Device) map[deviceKey]deviceBinding {
	index := make(map[deviceKey]deviceBinding, len(devices))
	for i := range devices {
		id := devices[i].Identity
		source := deviceAllocationSource(devices[i])
		if id.NodeUID == "" || id.Node == "" || source == "" || id.DeviceID == "" {
			continue
		}
		entry := deviceBinding{index: i, allowRaw: devices[i].RequireVerifiedRawBinding}
		record := func(alias string) {
			if alias == "" {
				return
			}
			k := deviceKey{id.NodeUID, id.Node, source, alias}
			if existing, exists := index[k]; exists {
				if existing.index != i {
					existing.ambiguous = true
					index[k] = existing
				}
				return
			}
			index[k] = entry
		}
		record(id.DeviceID)
		for _, alias := range devices[i].BindingAliases {
			record(alias)
		}
	}
	return index
}

func lookupDeviceBinding(index map[deviceKey]deviceBinding, source string, record podfacts.Allocation) (int, bool) {
	if record.NodeUID == "" {
		return -1, false
	}
	if record.DeviceID != "" {
		b, ok := index[deviceKey{record.NodeUID, record.Node, source, record.DeviceID}]
		return b.index, ok && !b.ambiguous
	}
	if record.RawDeviceID != "" {
		if b, ok := index[deviceKey{record.NodeUID, record.Node, source, record.RawDeviceID}]; ok && !b.ambiguous && b.allowRaw {
			return b.index, true
		}
	}
	return -1, false
}

const resolvedDeviceIndexPrefix = "\x00idx:"

func attachAllocations(out *resourceview.Snapshot, pods podfacts.Inventory, index map[deviceKey]deviceBinding) {
	for _, entry := range pods.Entries {
		f := entry.Facts
		if f == nil || f.Terminal {
			continue
		}
		occupancies := entry.Occupancies
		type resolvedRecord struct {
			c, s, r int
			idx     int
		}
		var resolved []resolvedRecord
		needsRewrite := false
		for ci, c := range f.Containers {
			for si, set := range c.Allocations {
				if set.Source == nil || set.Source.State != resourceview.Known {
					continue
				}
				for ri, record := range set.Records {
					idx, exists := lookupDeviceBinding(index, set.Source.Source, record)
					if !exists {
						continue
					}
					canonicalID := out.Devices[idx].Identity.DeviceID
					out.VerifiedBindings = append(out.VerifiedBindings, resourceview.VerifiedDeviceBinding{
						PodUID:      f.Metadata.UID,
						Container:   c.Key.Name,
						Source:      set.Source.Source,
						RecordID:    record.RecordID,
						RawDeviceID: record.RawDeviceID,
						Device:      out.Devices[idx].Identity,
					})
					resolved = append(resolved, resolvedRecord{ci, si, ri, idx})
					if record.DeviceID != canonicalID || index[deviceKey{record.NodeUID, record.Node, set.Source.Source, canonicalID}].ambiguous {
						needsRewrite = true
					}
				}
			}
		}
		if needsRewrite {
			f = podfacts.Clone(f)
			for _, item := range resolved {
				f.Containers[item.c].Allocations[item.s].Records[item.r].DeviceID = resolvedDeviceIndexPrefix + strconv.Itoa(item.idx)
			}
			occupancies = podfacts.EffectiveOccupancies(f)
		}

		for _, occupancy := range occupancies {
			if occupancy.NodeUID == "" {
				continue
			}
			idx := -1
			exists := false
			if rawIdx, ok := strings.CutPrefix(occupancy.DeviceID, resolvedDeviceIndexPrefix); ok {
				if parsed, err := strconv.Atoi(rawIdx); err == nil && parsed >= 0 && parsed < len(out.Devices) {
					idx, exists = parsed, true
				}
			} else if b, ok := index[deviceKey{occupancy.NodeUID, occupancy.Node, occupancy.Source, occupancy.DeviceID}]; ok && !b.ambiguous {
				idx, exists = b.index, true
			}
			if !exists {
				continue
			}
			device := &out.Devices[idx]
			device.EffectiveOccupancies = append(device.EffectiveOccupancies, resourceview.EffectiveOccupancy{
				PodUID: f.Metadata.UID, Source: occupancy.Source, Device: device.Identity,
				Resource: device.Resource, Complete: occupancy.Complete,
				SlotsAllocated:        allocationQuantity(occupancy.Quantities, "slots", "slots"),
				ComputeAllocatedShare: allocationQuantity(occupancy.Quantities, "compute", "share"),
				MemoryAllocatedBytes:  allocationQuantity(occupancy.Quantities, "memory", "bytes"),
			})
		}
	}
}

func appendDialectInventory(out *resourceview.Snapshot, nodeFact *resourceview.Node, inv provider.NodeInventory) {
	for _, source := range inv.Sources {
		if source != nil {
			nodeFact.Sources = append(nodeFact.Sources, copySource(source))
		}
	}
	invCoverage := &viewv1.Coverage{Sources: inv.Sources}
	for _, device := range inv.Devices {
		if device.Coverage == nil {
			device.Coverage = coverage("device", "shared_node_informer", 1)
		} else {
			device.Coverage = proto.Clone(device.Coverage).(*viewv1.Coverage)
		}
		addCoverageSources(device.Coverage, invCoverage)
		out.Devices = append(out.Devices, device)
	}
}
