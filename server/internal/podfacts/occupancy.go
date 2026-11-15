package podfacts

import (
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"vgpu/internal/resourceview"
)

type Occupancy struct {
	NodeUID                          types.UID
	PodUID                           types.UID
	Source, Node, Resource, DeviceID string
	Quantities                       map[string]Quantity
	Complete                         bool
}
type occupancyKey struct {
	source, node, resource, device string
	nodeUID                        types.UID
}

// EffectiveOccupancies computes only explicitly selected Pod-lifetime sources.
// It never calculates DRA persistent-claim occupancy by summing Pod consumers.
func EffectiveOccupancies(f *Facts) []Occupancy {
	result := []Occupancy{}
	if f == nil || f.Terminal {
		return result
	}
	groups := map[occupancyKey]map[string]bool{}
	containers := make([]Container, 0, len(f.Containers))
	for _, c := range f.Containers {
		if c.Kind == Ephemeral || (c.Kind == Init && f.InitUsageReleased) {
			continue
		}
		containers = append(containers, c)
		for _, set := range c.Allocations {
			for _, r := range set.Records {
				k := occupancyKey{set.Source.Source, r.Node, r.Resource, r.DeviceID, r.NodeUID}
				if groups[k] == nil {
					groups[k] = map[string]bool{}
				}
				for field := range r.Quantities {
					groups[k][field] = true
				}
			}
		}
	}
	keys := make([]occupancyKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.source != b.source {
			return a.source < b.source
		}
		if a.node != b.node {
			return a.node < b.node
		}
		if a.resource != b.resource {
			return a.resource < b.resource
		}
		if a.device != b.device {
			return a.device < b.device
		}
		return a.nodeUID < b.nodeUID
	})
	for _, key := range keys {
		row := Occupancy{PodUID: f.Metadata.UID, Source: key.source, Node: key.node, NodeUID: key.nodeUID, Resource: key.resource, DeviceID: key.device, Quantities: map[string]Quantity{}, Complete: len(groups[key]) > 0}
		for field := range groups[key] {
			values := map[string]resource.Quantity{}
			unit := ""
			valid := key.nodeUID != "" && key.node != "" && key.device != "" && key.resource != ""
			for _, c := range containers {
				sum := resource.MustParse("0")
				found := false
				for _, set := range c.Allocations {
					if set.Source.Source != key.source {
						continue
					}
					found = true
					if !set.Complete || set.Source.State != resourceview.Known || set.Accounting != AccountingPodPeak {
						valid = false
					}
					for _, r := range set.Records {
						if r.NodeUID != key.nodeUID || r.Node != key.node || r.Resource != key.resource || r.DeviceID != key.device {
							continue
						}
						q, exists := r.Quantities[field]
						if !exists || q.Value == nil || q.State == nil || q.State.State != resourceview.Known || q.Unit == "" {
							valid = false
							continue
						}
						if unit != "" && unit != q.Unit {
							valid = false
						}
						unit = q.Unit
						sum.Add(*q.Value)
					}
				}
				if !found {
					valid = false
				}
				values[c.Key.Name] = sum
			}
			state := unknown(key.source, "pod")
			if !valid {
				row.Complete = false
				row.Quantities[field] = Quantity{Unit: unit, State: state}
				continue
			}
			// Running native sidecars accumulate in original init order. Each normal
			// init overlaps preceding sidecars, then application containers overlap all.
			sidecars, peak, apps := resource.MustParse("0"), resource.MustParse("0"), resource.MustParse("0")
			init := []Container{}
			for _, c := range containers {
				switch c.Kind {
				case Init, NativeSidecar:
					init = append(init, c)
				case Regular:
					apps.Add(values[c.Key.Name])
				}
			}
			sort.Slice(init, func(i, j int) bool { return init[i].Index < init[j].Index })
			for _, c := range init {
				q := values[c.Key.Name]
				if c.Kind == NativeSidecar {
					sidecars.Add(q)
					if sidecars.Cmp(peak) > 0 {
						peak = sidecars.DeepCopy()
					}
				} else {
					phase := sidecars.DeepCopy()
					phase.Add(q)
					if phase.Cmp(peak) > 0 {
						peak = phase
					}
				}
			}
			apps.Add(sidecars)
			if apps.Cmp(peak) > 0 {
				peak = apps
			}
			row.Quantities[field] = Quantity{Value: &peak, Unit: unit, State: known(key.source, "pod")}
		}
		result = append(result, row)
	}
	return result
}
