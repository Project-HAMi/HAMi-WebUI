package providerfacts

import (
	"strings"

	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func proven(c *viewv1.Coverage) bool {
	return c != nil && c.Complete && resourceview.ValidateCoverage(c) == nil
}
func copySource(s *viewv1.SourceState) *viewv1.SourceState {
	return proto.Clone(s).(*viewv1.SourceState)
}
func addCoverageSources(c *viewv1.Coverage, other *viewv1.Coverage) {
	if other == nil {
		return
	}
	seen := map[string]bool{}
	for _, s := range c.Sources {
		seen[s.Source] = true
	}
	for _, s := range other.Sources {
		if s != nil && !seen[s.Source] {
			c.Sources = append(c.Sources, copySource(s))
			seen[s.Source] = true
		}
	}
}
func completeCoverage(c *viewv1.Coverage, complete bool) {
	c.Complete = false
	c.Total = nil
	for _, s := range c.Sources {
		if s.State != resourceview.Known && s.State != resourceview.NotApplicable {
			complete = false
		}
	}
	if complete {
		total := c.Observed
		c.Total = &total
		c.Complete = true
	}
}
func deviceAllocationSource(d resourceview.Device) string {
	src := d.AllocationSource
	if src == "" {
		src = d.Identity.Provider
	}
	if src == "" {
		return ""
	}
	return "hami_annotation:" + src
}

// On a mixed node, a request proved to belong exclusively to another
// registered source cannot invalidate this source's allocation coverage.
// Unmapped keys, including opaque claims, still need conservative treatment.
type nodeAllocationSources map[string]map[string]bool // annotation source -> confirmed resource keys

func (s nodeAllocationSources) add(d resourceview.Device) {
	source := deviceAllocationSource(d)
	if source == "" {
		return
	}
	if s[source] == nil {
		s[source] = map[string]bool{}
	}
	key := d.Resource
	if key == "" {
		return
	}
	keys := s[source]
	keys[key] = true
	for _, rk := range d.ResourceKeys {
		if rk != "" {
			keys[rk] = true
		}
	}
}

func (s nodeAllocationSources) canRequest(source, resource string) bool {
	if s[source][resource] {
		return true
	}
	for _, keys := range s {
		if keys[resource] {
			return false
		}
	}
	return true // opaque or missing resource key
}
func applySnapshotCoverage(out *resourceview.Snapshot, nodes []*corev1.Node, catalog *devicecatalog.Snapshot, pods podfacts.Inventory, nodeEvidence *viewv1.Coverage, bindingIndex map[deviceKey]deviceBinding, dialects []provider.InventoryDialect) {
	// Inventory and allocation checks only need the affected node and allocation source.
	nodesByUID := make(map[types.UID]*corev1.Node, len(nodes))
	for _, node := range nodes {
		if node != nil {
			nodesByUID[node.UID] = node
		}
	}
	devicesByNode := make(map[types.UID][]int)
	type scope struct {
		node   types.UID
		source string
	}
	sourcesByNode := make(map[types.UID]nodeAllocationSources)
	for i, d := range out.Devices {
		id := d.Identity
		if sourcesByNode[id.NodeUID] == nil {
			sourcesByNode[id.NodeUID] = nodeAllocationSources{}
		}
		sourcesByNode[id.NodeUID].add(d)
		devicesByNode[id.NodeUID] = append(devicesByNode[id.NodeUID], i)
	}
	podsByNode := make(map[string][]*podfacts.Facts)
	for _, entry := range pods.Entries {
		f := entry.Facts
		if f == nil || f.Terminal {
			continue
		}
		if f.NodeName != "" {
			podsByNode[f.NodeName] = append(podsByNode[f.NodeName], f)
		}
		if assigned := f.Metadata.Annotations["hami.io/vgpu-node"]; assigned != "" && assigned != f.NodeName {
			podsByNode[assigned] = append(podsByNode[assigned], f)
		}
	}
	// All cards from one allocator on a node share the same Pod evidence.
	// Validate those records once per source, not once per physical card.
	allocationIssues := make(map[scope]string, len(out.Devices))
	out.NodeCoverage = proto.Clone(nodeEvidence).(*viewv1.Coverage)
	for i := range out.Devices {
		d := &out.Devices[i]
		evidence := &viewv1.Coverage{Entity: "device", Observed: 1}
		addCoverageSources(evidence, nodeEvidence)
		for _, source := range d.Coverage.Sources {
			if strings.HasPrefix(source.Source, "hami_registration:") {
				evidence.Sources = append(evidence.Sources, copySource(source))
			}
		}
		completeCoverage(evidence, proven(nodeEvidence))
		d.Coverage = evidence
		key := scope{d.Identity.NodeUID, deviceAllocationSource(*d)}
		issue, checked := allocationIssues[key]
		if !checked {
			issue = allocationSourceIssue(*d, bindingIndex, sourcesByNode[key.node], podsByNode[d.Identity.Node])
			allocationIssues[key] = issue
		}
		d.AllocationCoverage = allocationCoverage(*d, issue, pods.Coverage, nodeEvidence)
	}
	globalComplete := proven(nodeEvidence)
	global := &viewv1.Coverage{Entity: "device", Observed: int32(len(out.Devices))}
	addCoverageSources(global, nodeEvidence)
	for i := range out.Nodes {
		n := &out.Nodes[i]
		c := &viewv1.Coverage{Entity: "device"}
		addCoverageSources(c, nodeEvidence)
		scopeComplete := proven(nodeEvidence)
		observedSources := map[string]bool{}
		for _, source := range n.Sources {
			if strings.HasPrefix(source.Source, "hami_registration:") {
				observedSources[source.Source] = true
				if source.State != resourceview.Known {
					scopeComplete = false
				}
			}
		}
		for _, idx := range devicesByNode[n.UID] {
			d := &out.Devices[idx]
			c.Observed++
			scopeComplete = scopeComplete && proven(d.Coverage)
		}
		mapped := map[string]bool{}
		for _, resources := range sourcesByNode[n.UID] {
			for key := range resources {
				mapped[key] = true
			}
		}
		// Only advertised accelerator resources can imply missing cards.
		// Other Kubernetes extended resources may represent networking or storage.
		for _, pool := range n.Resources {
			if ((pool.Capacity.Value != nil && pool.Capacity.Value.Sign() > 0) || (pool.Allocatable.Value != nil && pool.Allocatable.Value.Sign() > 0)) && !mapped[pool.Resource] {
				scopeComplete = false
			}
		}
		if node := nodesByUID[n.UID]; node != nil {
			for key, raw := range node.Annotations {
				if raw == "" {
					continue
				}
				recognized := false
				for _, dialect := range dialects {
					if dialect == nil || !dialect.OwnsRegistrationAnnotation(key, catalog) {
						continue
					}
					recognized = true
					if !observedSources["hami_registration:"+dialect.Name()] {
						scopeComplete = false
					}
				}
				if strings.HasPrefix(key, "hami.io/") && strings.Contains(key, "register") && !recognized {
					scopeComplete = false
				}
			}
		}
		if !scopeComplete {
			c.Sources = append(c.Sources, &viewv1.SourceState{Source: "registered_device_inventory", State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"scope": "node", "node": n.Name, "completeness": "unmapped_or_unavailable_source"}})
		}
		completeCoverage(c, scopeComplete)
		n.DeviceCoverage = c
		globalComplete = globalComplete && c.Complete
	}
	if !globalComplete {
		global.Sources = append(global.Sources, &viewv1.SourceState{Source: "registered_device_inventory", State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"completeness": "unmapped_or_unavailable_source"}})
	}
	completeCoverage(global, globalComplete)
	out.DeviceCoverage = global
}
func allocationSourceIssue(d resourceview.Device, bindingIndex map[deviceKey]deviceBinding, sources nodeAllocationSources, pods []*podfacts.Facts) string {
	sourceName := deviceAllocationSource(d)
	reason := ""
	for _, f := range pods {
		matchedRecord, possibleRequest, explainedElsewhere := false, false, false
		checkRequest := func(request podfacts.Request) bool {
			if !request.Positive && (request.State == nil || request.State.State == resourceview.Known) {
				return false
			}
			if sources.canRequest(sourceName, request.Resource) {
				possibleRequest = true
				return true
			}
			explainedElsewhere = true
			return false
		}
		for _, request := range f.Requests {
			checkRequest(request)
		}
		for _, container := range f.Containers {
			if len(container.Claims) > 0 {
				reason = "unsupported_claim_source"
			}
			if f.InitUsageReleased && container.Kind == podfacts.Init {
				continue
			}
			needsRecord := false
			for _, request := range container.Requests {
				if checkRequest(request) {
					needsRecord = true
				}
			}
			for _, set := range container.Allocations {
				if set.Source == nil {
					reason = "unavailable_allocation_source"
					continue
				}
				if set.Source.Source != sourceName {
					if _, known := sources[set.Source.Source]; known {
						explainedElsewhere = true
					} else if set.Source.State != resourceview.Known {
						reason = "unavailable_allocation_source"
					}
					continue
				}
				if set.Source.State != resourceview.Known {
					reason = "unavailable_allocation_source"
				}
				if !set.Complete {
					reason = "partial_allocation_records"
				}
				if len(set.Records) > 0 {
					matchedRecord = true
					needsRecord = false
				}
				for _, record := range set.Records {
					if record.NodeUID != d.Identity.NodeUID || record.Node != d.Identity.Node {
						reason = "unmatched_allocation_identity"
						continue
					}
					if _, matched := lookupDeviceBinding(bindingIndex, sourceName, record); !matched {
						reason = "unmatched_allocation_identity"
					}
				}
			}
			// An empty slot is valid for a CPU container. A container requesting
			// this allocator needs its own record, even when a sibling has one.
			if needsRecord && reason == "" {
				reason = "candidate_allocation_source_missing"
			}
		}
		unresolved := f.Candidate == podfacts.CandidateUnresolved && !explainedElsewhere
		if assigned := f.Metadata.Annotations["hami.io/vgpu-node"]; (matchedRecord || possibleRequest || unresolved) && assigned != "" && f.NodeName != "" && assigned != f.NodeName {
			reason = "allocation_node_conflict"
		}
		if !matchedRecord && (possibleRequest || unresolved) && reason == "" {
			reason = "candidate_allocation_source_missing"
		}
	}
	return reason
}

func allocationCoverage(d resourceview.Device, reason string, podEvidence, nodeEvidence *viewv1.Coverage) *viewv1.Coverage {
	c := &viewv1.Coverage{Entity: "occupancy", Observed: int32(len(d.EffectiveOccupancies))}
	addCoverageSources(c, nodeEvidence)
	addCoverageSources(c, podEvidence)
	sourceName := deviceAllocationSource(d)
	state := &viewv1.SourceState{Source: sourceName, State: resourceview.Known, Params: map[string]string{"scope": "hami_allocation_annotations", "node": d.Identity.Node}}
	if sourceName == "" {
		state.Source = "allocation_source"
	}
	c.Sources = append(c.Sources, state)
	complete := proven(nodeEvidence) && proven(podEvidence) && proven(d.Coverage)
	if sourceName == "" || d.Resource == "" || d.Configured == nil || !*d.Configured {
		reason = "resource_mapping_unverified"
	}
	if reason != "" {
		state.State = resourceview.Unknown
		state.Reason = "unverified"
		state.Params["completeness"] = reason
		complete = false
	}
	completeCoverage(c, complete)
	return c
}
