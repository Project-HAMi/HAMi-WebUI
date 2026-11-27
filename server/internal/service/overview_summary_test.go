package service

import (
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func configuredQuantity(value string) resourceview.Quantity {
	q := resource.MustParse(value)
	return resourceview.Quantity{
		Value: &q,
		State: &pb.FieldState{State: resourceview.Known, Source: "test", Scope: "device"},
	}
}

func TestCompactInventorySummaryMatchesExporterSemantics(t *testing.T) {
	src := overviewSource{
		At:             time.Unix(600, 0).UTC(),
		NodeCoverage:   &pb.Coverage{Complete: true},
		DeviceCoverage: &pb.Coverage{Complete: true},
	}
	node := testNode("worker-1", proto.Bool(true))
	src.Nodes = []resourceview.Node{node}

	n1 := int32(1)
	allocCov := &pb.Coverage{
		Entity:   "occupancy",
		Complete: true,
		Observed: n1,
		Total:    &n1,
		Sources:  []*pb.SourceState{{Source: "pod", State: resourceview.Known}},
	}

	d1 := testCard(node, "NVIDIA", "GPU-1", "GPU-1", "NVIDIA A100")
	d1.Configured = proto.Bool(true)
	d1.ComputeCapacityShare = configuredQuantity("1")
	d1.MemoryCapacityBytes = configuredQuantity("80Gi")
	d1.AllocationCoverage = allocCov
	d1.EffectiveOccupancies = []resourceview.EffectiveOccupancy{{
		Source:                "pod",
		PodUID:                "pod-1",
		Device:                d1.Identity,
		SlotsAllocated:        configuredQuantity("1"),
		ComputeAllocatedShare: configuredQuantity("0.4"),
		MemoryAllocatedBytes:  configuredQuantity("20Gi"),
		Complete:              true,
	}}
	d1.UtilizationRatio = testMetric("0.4", "ratio")
	d1.MemoryUsedBytes = testMetric("20Gi", "bytes")
	d1.MemoryPhysicalBytes = testMetric("80Gi", "bytes")

	// Device with unknown compute occupancy & over-physical memory reading must be excluded from those pairs,
	// while still contributing valid memory allocation and compute utilization.
	d2 := testCard(node, "NVIDIA", "GPU-2", "GPU-2", "NVIDIA A100")
	d2.Configured = proto.Bool(true)
	d2.ComputeCapacityShare = configuredQuantity("1")
	d2.MemoryCapacityBytes = configuredQuantity("80Gi")
	d2.AllocationCoverage = allocCov
	d2.EffectiveOccupancies = []resourceview.EffectiveOccupancy{{
		Source:                "pod",
		PodUID:                "pod-2",
		Device:                d2.Identity,
		SlotsAllocated:        configuredQuantity("1"),
		ComputeAllocatedShare: resourceview.Quantity{},
		MemoryAllocatedBytes:  configuredQuantity("10Gi"),
		Complete:              true,
	}}
	d2.UtilizationRatio = testMetric("0.8", "ratio")
	d2.MemoryUsedBytes = testMetric("85Gi", "bytes")
	d2.MemoryPhysicalBytes = testMetric("80Gi", "bytes")

	src.Devices = []resourceview.Device{d1, d2}
	reply := makeInventory(resourceview.Snapshot(src)).compactInventory()
	if reply.Summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if reply.Summary.GetComputeAllocatedRatio() != 0.4 || reply.Summary.GetComputeCapacityRatio() != 1 {
		t.Fatalf("compute alloc summary = (%v, %v), want (0.4, 1)", reply.Summary.ComputeAllocatedRatio, reply.Summary.ComputeCapacityRatio)
	}
	const giB = float64(1024 * 1024 * 1024)
	if reply.Summary.GetMemoryAllocatedBytes() != 30*giB || reply.Summary.GetMemoryCapacityBytes() != 160*giB {
		t.Fatalf("memory alloc summary = (%v, %v), want (%v, %v)", reply.Summary.MemoryAllocatedBytes, reply.Summary.MemoryCapacityBytes, 30*giB, 160*giB)
	}
	if math.Abs(reply.Summary.GetUtilizationRatio()-0.6) > 1e-9 {
		t.Fatalf("utilization summary = %v, want 0.6", reply.Summary.GetUtilizationRatio())
	}
	if reply.Summary.GetMemoryUsedBytes() != 20*giB || reply.Summary.GetMemoryPhysicalBytes() != 80*giB {
		t.Fatalf("memory usage summary = (%v, %v), want (%v, %v)", reply.Summary.MemoryUsedBytes, reply.Summary.MemoryPhysicalBytes, 20*giB, 80*giB)
	}
}

func TestCompactInventoryOmitsEmptySummary(t *testing.T) {
	src := overviewSource{
		At:             time.Unix(600, 0).UTC(),
		NodeCoverage:   &pb.Coverage{Complete: true},
		DeviceCoverage: &pb.Coverage{Complete: true},
	}
	reply := makeInventory(resourceview.Snapshot(src)).compactInventory()
	if reply.Summary != nil {
		t.Fatalf("expected nil summary for empty fleet, got %+v", reply.Summary)
	}
}
