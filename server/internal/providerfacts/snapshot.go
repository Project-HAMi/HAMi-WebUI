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
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/hygon"
	"vgpu/internal/provider/metax"
	"vgpu/internal/provider/mlu"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/provider/util"
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
func Build(nodes []*corev1.Node, catalog *devicecatalog.Snapshot, pods podfacts.Inventory, selectors map[string]labels.Selector, logger *log.Helper, at time.Time, nodeEvidence *viewv1.Coverage) resourceview.Snapshot {
	out := resourceview.Snapshot{At: at, Nodes: []resourceview.Node{}, Devices: []resourceview.Device{}}
	for _, node := range nodes {
		if node == nil || node.Name == "" || node.UID == "" {
			continue
		}
		fact := nodeFact(node, catalog)

		selected := func(provider string) bool {
			selector, ok := selectors[provider]
			return ok && selector.Matches(labels.Set(node.Labels))
		}
		if selected("NVIDIA") {
			if raw, present := node.Annotations[nvidia.RegisterAnnos]; present {
				records, err := nvidia.RegistrationFacts(raw, logger)
				appendRegistrationFacts(&out, &fact, node, catalog, "NVIDIA", "nvidia", "NVIDIA", records, err)
			}
		}
		if selected("HCU") {
			if raw, present := node.Annotations[hygon.HCURegisterAnnos]; present {
				records, err := hygon.RegistrationFacts(raw, node.Name, true)
				appendRegistrationFacts(&out, &fact, node, catalog, "HCU", "hygon", "HCU", records, err)
			}
		}
		if selected("DCU") {
			if raw, present := node.Annotations[hygon.RegisterAnnos]; present {
				records, err := hygon.RegistrationFacts(raw, node.Name, false)
				appendRegistrationFacts(&out, &fact, node, catalog, "DCU", "hygon", "DCU", records, err)
			}
		}
		if selected("Metax") {
			if raw, present := node.Annotations[metax.MetaxSDeviceAnno]; present {
				records, err := metax.SGPURegistrationFacts(raw)
				appendRegistrationFacts(&out, &fact, node, catalog, "Metax", "metax", "Metax-SGPU", records, err)
			}
		}
		if selected("Ascend") {
			present := false
			for key := range node.Annotations {
				if strings.HasPrefix(key, "hami.io/node-register-Ascend") {
					present = true
				}
			}
			for _, word := range catalog.AscendCommonWords() {
				if _, ok := node.Annotations["hami.io/node-register-"+word]; ok {
					present = true
				}
			}
			if present {
				records, err := ascend.RegistrationFacts(node, catalog)
				appendRegistrationFacts(&out, &fact, node, catalog, "Ascend", "huawei", "Ascend", records, err)
			}
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
	applySnapshotCoverage(&out, nodes, catalog, pods, nodeEvidence, bindingIndex)
	return out
}
func nodeFact(node *corev1.Node, catalog *devicecatalog.Snapshot) resourceview.Node {
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
		if acceleratorResource(string(name), catalog) {
			names[name] = true
		}
	}
	for name := range node.Status.Allocatable {
		if acceleratorResource(string(name), catalog) {
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
func acceleratorResource(name string, catalog *devicecatalog.Snapshot) bool {
	if !strings.Contains(name, "/") || strings.Contains(name, corev1.ResourceDefaultNamespacePrefix) {
		return false
	}
	switch name {
	case "mthreads.com/gpu", "mthreads.com/vgpu", "mthreads.com/sgpu-core", "mthreads.com/sgpu-memory", "nvidia.com/gpu", "nvidia.com/gpu.shared", "amd.com/gpu":
		return true
	}
	if strings.HasPrefix(name, "nvidia.com/mig-") || strings.HasPrefix(name, "amd.com/gpu-") {
		return true
	}
	if strings.HasPrefix(name, mlu.CambriconResourcePrefix) && (strings.HasSuffix(name, mlu.CambriconCoreResource) || strings.HasSuffix(name, mlu.CambriconMemResource)) {
		return true
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
	if model := catalog.Cambricon; model != nil && (name == model.ResourceCountName || name == model.ResourceMemoryName || name == model.ResourceCoreName) {
		return true
	}
	_, _, configured := catalog.AscendResource(name)
	return configured
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

func buildDeviceBindingIndex(devices []resourceview.Device) map[deviceKey]int {
	index := make(map[deviceKey]int, len(devices))
	for i, device := range devices {
		id := device.Identity
		if id.NodeUID == "" || id.Node == "" || id.DeviceID == "" {
			continue
		}
		source := deviceAllocationSource(device)
		if source == "" {
			continue
		}
		index[deviceKey{id.NodeUID, id.Node, source, id.DeviceID}] = i
		if id.Provider == "DCU" {
			if raw, ok := strings.CutPrefix(id.DeviceID, "DCU-"); ok {
				if minor, err := strconv.Atoi(raw); err == nil && minor >= 0 {
					index[deviceKey{id.NodeUID, id.Node, source, id.Node + "-dcu-" + strconv.Itoa(minor)}] = i
				}
			}
		}
	}
	return index
}

func recordLookupDeviceID(source string, record podfacts.Allocation) string {
	if record.DeviceID == "" && source == "hami_annotation:Metax-SGPU" {
		return record.RawDeviceID
	}
	return record.DeviceID
}

func attachAllocations(out *resourceview.Snapshot, pods podfacts.Inventory, index map[deviceKey]int) {
	for _, entry := range pods.Entries {
		f := entry.Facts
		if f == nil || f.Terminal {
			continue
		}
		occupancies := entry.Occupancies
		matchedMetaX := false
		for ci, c := range f.Containers {
			for si, set := range c.Allocations {
				if set.Source == nil || set.Source.State != resourceview.Known {
					continue
				}
				for ri, record := range set.Records {
					lookupID := recordLookupDeviceID(set.Source.Source, record)
					if record.NodeUID == "" || lookupID == "" {
						continue
					}
					idx, exists := index[deviceKey{record.NodeUID, record.Node, set.Source.Source, lookupID}]
					if !exists {
						continue
					}
					out.VerifiedBindings = append(out.VerifiedBindings, resourceview.VerifiedDeviceBinding{
						PodUID:      f.Metadata.UID,
						Container:   c.Key.Name,
						Source:      set.Source.Source,
						RecordID:    record.RecordID,
						RawDeviceID: record.RawDeviceID,
						Device:      out.Devices[idx].Identity,
					})
					if set.Source.Source == "hami_annotation:Metax-SGPU" && record.DeviceID == "" {
						if !matchedMetaX {
							f = podfacts.Clone(f)
							matchedMetaX = true
						}
						f.Containers[ci].Allocations[si].Records[ri].DeviceID = record.RawDeviceID
					}
				}
			}
		}
		if matchedMetaX {
			occupancies = podfacts.EffectiveOccupancies(f)
		}

		for _, occupancy := range occupancies {
			i, exists := index[deviceKey{occupancy.NodeUID, occupancy.Node, occupancy.Source, occupancy.DeviceID}]
			if !exists || occupancy.NodeUID == "" {
				continue
			}
			device := &out.Devices[i]
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

func appendRegistrationFacts(out *resourceview.Snapshot, nodeFact *resourceview.Node, node *corev1.Node, catalog *devicecatalog.Snapshot, provider, vendor, configKey string, records []util.RegistrationDevice, err error) {
	source := &viewv1.SourceState{Source: "hami_registration:" + provider, State: resourceview.Known}
	if err != nil {
		source.State, source.Reason = resourceview.Unknown, "unverified"
	}
	nodeFact.Sources = append(nodeFact.Sources, source)
	for _, record := range records {
		device := registrationDeviceFact(node, catalog, provider, vendor, configKey, record)
		device.Coverage.Sources = append(device.Coverage.Sources, proto.Clone(source).(*viewv1.SourceState))
		out.Devices = append(out.Devices, device)
	}
}

func registrationDeviceFact(node *corev1.Node, catalog *devicecatalog.Snapshot, provider, vendor, configKey string, registered util.RegistrationDevice) resourceview.Device {
	device := registered.Device
	id := resourceview.DeviceIdentity{Provider: provider, Node: node.Name, NodeUID: node.UID, DeviceID: device.ID}
	row := resourceview.Device{Identity: id, Kind: "physical", Vendor: vendor, Model: device.Type, Mode: device.Mode, States: map[string]*viewv1.FieldState{}, Sources: []*viewv1.SourceState{{Source: "hami_registration:" + provider, State: resourceview.Known}}, Coverage: coverage("device", "shared_node_informer", 1)}
	if provider == "NVIDIA" || provider == "Metax" {
		uuid := device.ID
		row.UUID = &uuid
	}
	if registered.Known["health"] {
		health := device.Health
		row.Healthy = &health
	}
	row.States["kind"] = known("node_registration", "device")
	row.States["model"], row.States["mode"], row.States["health"], row.States["uuid"] = known("node_registration", "device"), known("node_registration", "device"), known("node_registration", "device"), known("node_registration", "device")
	if !registered.Known["mode"] || (provider == "NVIDIA" && device.Mode != nvidia.ModeHamiCore && device.Mode != nvidia.ModeMig) {
		row.Mode = ""
		row.States["mode"] = unknown("node_registration", "device", "unverified")
		row.States["mode"].Params = map[string]string{"raw": registered.RawMode}
	}
	if !registered.Known["health"] {
		row.States["health"] = unknown("node_registration", "device", "unverified")
	}
	if !registered.Known["type"] {
		row.States["model"] = unknown("node_registration", "device", "unverified")
	}
	if row.UUID == nil {
		row.States["uuid"] = unknown("node_registration", "device", "unverified")
	}
	if provider == "HCU" {
		row.Identity.DeviceID = device.AliasId
	}
	if provider == "HCU" || provider == "DCU" {
		serial, _ := hygon.BoardSerial(provider, row.Identity.DeviceID)
		priority := 0
		if provider == "HCU" {
			priority = 1
		}
		row.Physical = resourceview.PhysicalAssociation{Domain: "hygon-board", BoardKey: serial, Priority: priority}
	}
	row.States["configured"] = unknown("device_config", "device", "missing_config")
	row.States["resource"] = unknown("device_config", "device", "missing_config")
	if catalog.Loaded() {
		names, present := catalog.ResourceNames[configKey]
		if provider == "HCU" && !present {
			// Both DCU and HCU use HAMi's explicit hygon configuration. The
			// exact Node resource check below still has to confirm this mapping.
			names, present = catalog.ResourceNames["DCU"]
		}
		configured := present
		if provider == "Ascend" {
			model, exists := catalog.AscendModel(device.Type)
			names.Count = model.ResourceName
			configured = exists
			present = true
		}
		if present {
			row.Configured = &configured
			row.States["configured"] = known("device_config", "device")
			if names.Count != "" {
				_, capacityPresent := node.Status.Capacity[corev1.ResourceName(names.Count)]
				_, allocatablePresent := node.Status.Allocatable[corev1.ResourceName(names.Count)]
				if capacityPresent || allocatablePresent {
					row.Resource = names.Count
					row.States["resource"] = known("device_config_and_node_status", "device")
				} else {
					row.States["resource"] = unknown("device_config_and_node_status", "device", "unverified")
					row.States["resource"].Params = map[string]string{"expectedResource": names.Count}
				}
			}
		}
	}
	row.SlotsCapacity = number(int64(device.Count), "slots", "node_registration", "device")
	row.ComputeCapacityShare = percentShare(number(int64(device.Devcore), "percent", "node_registration", "device"))
	row.MemoryCapacityBytes = number(int64(device.Devmem)*1024*1024, "bytes", "node_registration", "device")
	if !registered.Known["count"] {
		row.SlotsCapacity = absent("slots", "node_registration", "device", "unverified")
	}
	if !registered.Known["devcore"] {
		row.ComputeCapacityShare = absent("share", "node_registration", "device", "unverified")
	}
	if !registered.Known["devmem"] {
		row.MemoryCapacityBytes = absent("bytes", "node_registration", "device", "unverified")
	}
	if provider == "Ascend" {
		// registerHAMi reports a percentage only in hami-core mode. Template
		// mode reports physical AI Core count; normalize against the exact model.
		if device.Mode != ascend.VNPUModeHamiCore {
			row.ComputeCapacityShare = absent("share", "node_registration_and_device_config", "device", "unverified")
			if model, exists := catalog.AscendModel(device.Type); exists && device.Mode == ascend.ModeTemplate && registered.Known["devcore"] && model.AICore > 0 {
				ratio := new(big.Rat).SetFrac64(int64(device.Devcore), int64(model.AICore))
				decimal := ratio.FloatString(9)
				if exact, ok := new(big.Rat).SetString(decimal); ok && exact.Cmp(ratio) == 0 {
					value := resource.MustParse(decimal)
					row.ComputeCapacityShare = resourceview.Quantity{Value: &value, Unit: "share", State: known("node_registration_and_device_config", "device")}
				}
			}
		}
		if device.Type == "Ascend910C" {
			// A 910C registration is a die/module. Source-reviewed paired
			// allocation does not establish a unique enclosing physical card.
			row.Kind = "unknown"
			row.States["kind"] = unknown("node_registration", "device", "unverified")
			row.States["kind"].Params = map[string]string{"identityLevel": "npu_module", "missing": "physical_card_association"}
		}
	}
	row.MemoryPhysicalBytes = absent("bytes", "telemetry", "device", "unverified")
	row.MemoryUsedBytes = absent("bytes", "telemetry", "device", "unverified")
	row.UtilizationRatio = absent("ratio", "telemetry", "device", "unverified")
	return row
}
