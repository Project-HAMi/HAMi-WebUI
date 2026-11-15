package resourceview

import (
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

type OccupancyTotal struct {
	Sum     resource.Quantity
	Unknown int
}

func (total *OccupancyTotal) add(q Quantity) {
	if _, ok := q.KnownValue(); ok {
		total.Sum.Add(*q.Value)
	} else {
		total.Unknown++
	}
}

type OccupancyTotals struct{ Slots, Compute, Memory OccupancyTotal }

// Occupancy sums the effective allocations of distinct Pods on one configured
// physical card. Multiple scheduling authorities cannot be combined.
func (g PhysicalGroup) Occupancy() (OccupancyTotals, bool) {
	if len(g.Members) != 1 {
		return OccupancyTotals{}, false
	}
	device := &g.Members[0]
	coverage := device.AllocationCoverage
	if device.Configured == nil || !*device.Configured || coverage == nil || !coverage.Complete || ValidateCoverage(coverage) != nil || int(coverage.Observed) != len(device.EffectiveOccupancies) {
		return OccupancyTotals{}, false
	}
	var totals OccupancyTotals
	type key struct {
		source string
		uid    types.UID
	}
	seen := make(map[key]struct{}, len(device.EffectiveOccupancies))
	for _, o := range device.EffectiveOccupancies {
		k := key{o.Source, o.PodUID}
		if o.Source == "" || o.PodUID == "" || o.Device != device.Identity {
			return OccupancyTotals{}, false
		}
		if _, duplicate := seen[k]; duplicate {
			return OccupancyTotals{}, false
		}
		seen[k] = struct{}{}
		totals.Slots.add(o.SlotsAllocated)
		totals.Compute.add(o.ComputeAllocatedShare)
		totals.Memory.add(o.MemoryAllocatedBytes)
	}
	return totals, true
}
