package service

import (
	"context"
	"fmt"
	"math"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	view "vgpu/api/observability/v1"
	pb "vgpu/api/v1"
	"vgpu/internal/resourceview"
)

func nativeInventoryNode(name string) resourceview.Node {
	n := testNode(name, nil)
	n.Resources = []resourceview.NodeResource{{Resource: "mthreads.com/vgpu", Capacity: testMetric("8", "resource_units")}}
	n.DeviceCoverage = &view.Coverage{Entity: "device", Complete: false}
	return n
}

func TestNodePagesFollowOverviewInventory(t *testing.T) {
	registered := testNode("a-registered", proto.Bool(true))
	native := nativeInventoryNode("b-native")
	native.InternalIP = "10.0.0.2"
	native.SystemInfo = corev1.NodeSystemInfo{OSImage: "Ubuntu", Architecture: "amd64"}
	cpu := testNode("c-cpu", proto.Bool(true))
	card := testCard(registered, "NVIDIA", "a", "gpu-a", "A100")
	card.Configured = proto.Bool(true)
	card.ComputeCapacityShare = testMetric("1", "share")
	card.MemoryCapacityBytes = testMetric("8Mi", "bytes")
	card.AllocationCoverage = &view.Coverage{Entity: "occupancy", Complete: true, Observed: 1, Total: proto.Int32(1), Sources: []*view.SourceState{{Source: "pod", State: "known"}}}
	card.EffectiveOccupancies = []resourceview.EffectiveOccupancy{{Source: "pod", PodUID: "pod-a", Device: card.Identity,
		ComputeAllocatedShare: testMetric("0.07", "share"), MemoryAllocatedBytes: testMetric("1Mi", "bytes")}}
	// Without a capacity, the second card's large allocation cannot enter the ratio.
	missing := card
	missing.Identity.DeviceID = "b"
	missing.UUID = proto.String("gpu-b")
	missing.ComputeCapacityShare = resourceview.Quantity{}
	missing.MemoryCapacityBytes = resourceview.Quantity{}
	missing.EffectiveOccupancies = []resourceview.EffectiveOccupancy{{Source: "pod", PodUID: "pod-b", Device: missing.Identity, ComputeAllocatedShare: testMetric("99", "share"), MemoryAllocatedBytes: testMetric("100Mi", "bytes")}}
	src := overviewSource{Nodes: []resourceview.Node{registered, native, cpu}, Devices: []resourceview.Device{card, missing}, NodeCoverage: &view.Coverage{Entity: "node", Complete: true}}
	ctx := context.Background()
	nodes := &NodeService{source: &src}
	all := &pb.GetAllNodesReq{UseResourceInventory: true}
	rows, err := nodes.GetAllNodes(ctx, all)
	if err != nil || len(rows.List) != 2 || rows.InventoryCoverage.GetObserved() != 2 || !rows.InventoryCoverage.Complete {
		t.Fatalf("roster = %v, error = %v", rows, err)
	}
	overview, err := (&OverviewService{source: &src}).GetInventory(ctx, &view.InventoryRequest{})
	if err != nil || !proto.Equal(rows.InventoryCoverage, overview.NodeCoverage) || len(overview.Nodes) != len(rows.List) {
		t.Fatalf("roster and overview disagree: %v, %v, %v", rows, overview, err)
	}
	for i, node := range overview.Nodes {
		row := rows.List[i]
		if node.Uid != row.Uid || node.GetPhysicalDevices() != row.GetCardCnt() || (node.PhysicalDevices == nil) != (row.CardCnt == nil) {
			t.Fatalf("node %d: overview %v, list %v", i, node, row)
		}
	}
	r := rows.List[0]
	if r.GetCardCnt() != 2 || r.GetCoreUsed() != 7 || r.GetCoreTotal() != 100 || r.GetMemoryUsed() != 1 || r.GetMemoryTotal() != 8 || math.Abs(r.GetCoreAllocationPercent()-7) > 1e-12 || r.GetMemoryAllocationPercent() != 12.5 {
		t.Fatalf("registered node = %v", r)
	}
	n := rows.List[1]
	if n.Uid != string(native.UID) || n.Ip != native.InternalIP || n.OsImage != "Ubuntu" || n.Architecture != "AMD64" || n.IsReady != nil || n.CardCnt != nil || n.InventoryCoverage == nil || n.InventoryCoverage.Complete || n.CoreTotal != nil || n.CoreAllocationPercent != nil || n.MemoryUsed != nil {
		t.Fatalf("resource-only node = %v", n)
	}
	for _, row := range rows.List {
		detail, err := nodes.GetNode(ctx, &pb.GetNodeReq{Uid: row.Uid, UseResourceInventory: true})
		if err != nil || !proto.Equal(row, detail) {
			t.Fatalf("list entry %v, detail %v, err %v", row, detail, err)
		}
	}
	list, err := nodes.GetAllNodes(ctx, &pb.GetAllNodesReq{UseResourceInventory: true, Filters: &pb.GetAllNodesReq_Filters{Type: "A100"}})
	if err != nil || len(list.List) != 1 || list.List[0].Uid != r.Uid || list.InventoryCoverage.GetObserved() != 1 {
		t.Fatalf("filtered = %v, %v", list, err)
	}

	src.Nodes[1].UID = "recreated-uid"
	for _, uid := range []string{string(cpu.UID), string(native.UID)} {
		_, err = nodes.GetNode(ctx, &pb.GetNodeReq{Uid: uid, UseResourceInventory: true})
		if e := kratoserrors.FromError(err); e.GetCode() != 404 || e.GetReason() != "RESOURCE_NODE_NOT_FOUND" {
			t.Fatalf("detail %s: %v", uid, err)
		}
	}
	src.NodeCoverage.Complete = false
	recovered, err := nodes.GetAllNodes(ctx, all)
	if err != nil || len(recovered.List) != 2 || recovered.InventoryCoverage.GetComplete() {
		t.Fatalf("partial with known rows: %v %v", recovered, err)
	}
	_, err = nodes.GetNode(ctx, &pb.GetNodeReq{Uid: string(native.UID), UseResourceInventory: true})
	if e := kratoserrors.FromError(err); e.GetCode() != 503 || e.GetReason() != "NODE_INVENTORY_UNAVAILABLE" {
		t.Fatalf("partial missing detail: %v", err)
	}
	src.Nodes = nil
	src.Devices = nil
	_, err = nodes.GetAllNodes(ctx, all)
	if e := kratoserrors.FromError(err); e.GetCode() != 503 || e.GetReason() != "NODE_INVENTORY_UNAVAILABLE" {
		t.Fatalf("no evidence: %v", err)
	}
	src.NodeCoverage.Complete = true
	empty, err := nodes.GetAllNodes(ctx, all)
	if err != nil || len(empty.List) != 0 || !empty.InventoryCoverage.GetComplete() {
		t.Fatalf("known empty: %v, %v", empty, err)
	}
}

func TestNodeInventoryCapacityAndAllocationCoverage(t *testing.T) {
	node := testNode("two-80gib", proto.Bool(true))
	card := func(id string) resourceview.Device {
		d := testCard(node, "NVIDIA", id, "GPU-"+id, "A100-80GB")
		d.Configured = proto.Bool(true)
		d.SlotsCapacity = testMetric("8", "slots")
		d.ComputeCapacityShare = testMetric("1", "share")
		d.MemoryCapacityBytes = testMetric("80Gi", "bytes")
		d.AllocationCoverage = &view.Coverage{Entity: "occupancy", Complete: true, Observed: 1, Total: proto.Int32(1), Sources: []*view.SourceState{{Source: "pod", State: "known"}}}
		d.EffectiveOccupancies = []resourceview.EffectiveOccupancy{{Source: "pod", PodUID: "pod", Device: d.Identity,
			SlotsAllocated: testMetric("2", "slots"), ComputeAllocatedShare: testMetric("0.25", "share"), MemoryAllocatedBytes: testMetric("20Gi", "bytes")}}
		return d
	}
	a := card("a")
	missingAmounts := card("b")
	o := &missingAmounts.EffectiveOccupancies[0]
	o.SlotsAllocated, o.ComputeAllocatedShare, o.MemoryAllocatedBytes = resourceview.Quantity{}, resourceview.Quantity{}, resourceview.Quantity{}
	missingCoverage := card("b")
	missingCoverage.AllocationCoverage = nil
	idle := card("b")
	idle.EffectiveOccupancies = nil
	idle.AllocationCoverage.Observed, idle.AllocationCoverage.Total = 0, proto.Int32(0)
	partial := card("b")
	partial.EffectiveOccupancies[0].ComputeAllocatedShare = testMetric("0.75", "share")
	partial.EffectiveOccupancies[0].MemoryAllocatedBytes = resourceview.Quantity{}
	noCapacity := card("b")
	noCapacity.SlotsCapacity, noCapacity.ComputeCapacityShare, noCapacity.MemoryCapacityBytes = resourceview.Quantity{}, resourceview.Quantity{}, resourceview.Quantity{}
	conflictingMemory := card("b")
	conflictingMemory.MemoryCapacityBytes.State.State = resourceview.Conflict
	zeroCapacity := card("b")
	zeroCapacity.SlotsCapacity, zeroCapacity.ComputeCapacityShare, zeroCapacity.MemoryCapacityBytes = testMetric("0", "slots"), testMetric("0", "share"), testMetric("0", "bytes")
	unconfigured := card("b")
	unconfigured.Configured = proto.Bool(false)
	unknownConfig := card("b")
	unknownConfig.Configured = nil
	alias := card("alias")
	alias.Identity.Provider, alias.UUID = "other", a.UUID
	alias.EffectiveOccupancies[0].Device = alias.Identity
	alias.MemoryCapacityBytes = testMetric("40Gi", "bytes")

	// Amounts are reported in the legacy API units: slots, percentage points and MiB.
	type amounts struct{ slots, core, memory *int32 }
	all := func(slots, core, memory int32) amounts {
		return amounts{proto.Int32(slots), proto.Int32(core), proto.Int32(memory)}
	}
	two, one, used := all(16, 200, 163840), all(8, 100, 81920), all(2, 25, 20480)
	for _, tc := range []struct {
		name                       string
		devices                    []resourceview.Device
		wantTotal, wantUsed        amounts
		wantCorePct, wantMemoryPct *float64
	}{
		{"unknown amounts", []resourceview.Device{a, missingAmounts}, two, used, proto.Float64(25), proto.Float64(25)},
		{"incomplete occupancy", []resourceview.Device{a, missingCoverage}, two, used, proto.Float64(25), proto.Float64(25)},
		{"no known occupancy", []resourceview.Device{missingCoverage}, one, amounts{}, nil, nil},
		{"known idle card", []resourceview.Device{a, idle}, two, used, proto.Float64(12.5), proto.Float64(12.5)},
		{"known zero allocation", []resourceview.Device{idle}, one, all(0, 0, 0), proto.Float64(0), proto.Float64(0)},
		{"only memory allocation missing", []resourceview.Device{a, partial}, two, all(4, 100, 20480), proto.Float64(50), proto.Float64(25)},
		{"missing capacity", []resourceview.Device{a, noCapacity}, one, used, proto.Float64(25), proto.Float64(25)},
		{"conflicting memory capacity", []resourceview.Device{a, conflictingMemory}, all(16, 200, 81920), all(4, 50, 20480), proto.Float64(25), proto.Float64(25)},
		{"no known capacity", []resourceview.Device{noCapacity}, amounts{}, amounts{}, nil, nil},
		{"explicit zero capacity", []resourceview.Device{zeroCapacity}, all(0, 0, 0), amounts{}, nil, nil},
		{"multiple registrations", []resourceview.Device{a, alias}, amounts{}, amounts{}, nil, nil},
		{"unconfigured", []resourceview.Device{unconfigured}, amounts{}, amounts{}, nil, nil},
		{"unknown configuration", []resourceview.Device{unknownConfig}, amounts{}, amounts{}, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := overviewSource{Nodes: []resourceview.Node{node}, Devices: tc.devices, NodeCoverage: &view.Coverage{Entity: "node", Complete: true}}
			row, err := (&NodeService{source: src}).GetNode(context.Background(), &pb.GetNodeReq{Uid: string(node.UID), UseResourceInventory: true})
			if err != nil {
				t.Fatal(err)
			}
			if !equalOptional(row.VgpuTotal, tc.wantTotal.slots) || !equalOptional(row.CoreTotal, tc.wantTotal.core) || !equalOptional(row.MemoryTotal, tc.wantTotal.memory) ||
				!equalOptional(row.VgpuUsed, tc.wantUsed.slots) || !equalOptional(row.CoreUsed, tc.wantUsed.core) || !equalOptional(row.MemoryUsed, tc.wantUsed.memory) ||
				!equalOptional(row.CoreAllocationPercent, tc.wantCorePct) || !equalOptional(row.MemoryAllocationPercent, tc.wantMemoryPct) ||
				row.GetCoreUsedKnown() != (tc.wantUsed.core != nil) {
				t.Fatalf("unexpected node amounts: %v", row)
			}
		})
	}
}

func TestNodeAllocationAmountsRespectLegacyUnits(t *testing.T) {
	for _, tt := range []struct {
		used, capacity         string
		multiply, divide       int64
		wantUsed, wantCapacity *int32
	}{
		{"0.29", "1", 100, 1, proto.Int32(29), proto.Int32(100)},
		{"2.375", "8", 100, 1, nil, proto.Int32(800)},
		{"0", "1Mi", 1, 1024 * 1024, proto.Int32(0), proto.Int32(1)},
		{"128Ki", "1Mi", 1, 1024 * 1024, nil, proto.Int32(1)},
		{"0", "2147483648", 1, 1, proto.Int32(0), nil},
	} {
		var p nodeAllocation
		p.add(testMetric(tt.capacity, "test"), resourceview.OccupancyTotal{Sum: resource.MustParse(tt.used)}, true)
		used, total := p.amounts(tt.multiply, tt.divide)
		if !equalOptional(used, tt.wantUsed) || !equalOptional(total, tt.wantCapacity) || p.percent() == nil {
			t.Errorf("%s/%s: got %v %v", tt.used, tt.capacity, used, total)
		}
	}
}

func TestNodeAllocationPercentRoundsOnlyAfterDividing(t *testing.T) {
	for _, tt := range []struct {
		used, total string
		want        float64
	}{
		// 92 GiB of 640 GiB with different decimal scales.
		{"98784247808.00000", "640Gi", 14.375},
		{"0.07", "1", 7},
		{"2.375", "8", 29.6875},
		{"4", "3", 400.0 / 3},
	} {
		var p nodeAllocation
		p.add(testMetric(tt.total, "test"), resourceview.OccupancyTotal{Sum: resource.MustParse(tt.used)}, true)
		percent := p.percent()
		if percent == nil {
			t.Fatal("missing allocation percentage")
		}
		if *percent != tt.want {
			t.Errorf("percentage for %s/%s = %.17g, want %.17g", tt.used, tt.total, *percent, tt.want)
		}
	}
}

func equalOptional[T comparable](a, b *T) bool { return a == b || (a != nil && b != nil && *a == *b) }

func TestNodeInventoryUsageAndProviders(t *testing.T) {
	node := testNode("hygon-node", proto.Bool(true))
	d1 := testCard(node, "DCU", "0", "DCU-0", "K100")
	d1.Configured = proto.Bool(true)
	d1.UtilizationRatio = testMetric("0.4", "ratio")
	d1.MemoryUsedBytes = testMetric("4Gi", "bytes")
	d1.MemoryPhysicalBytes = testMetric("16Gi", "bytes")

	// Unconfigured physical card still contributes observed utilization, memory usage, and canonical provider.
	d2 := testCard(node, "HCU", "1", "HCU-1", "K100")
	d2.Configured = proto.Bool(false)
	d2.UtilizationRatio = testMetric("0.8", "ratio")
	d2.MemoryUsedBytes = testMetric("12Gi", "bytes")
	d2.MemoryPhysicalBytes = testMetric("16Gi", "bytes")

	// Conflicting sample is omitted from usage averages cleanly.
	d3 := testCard(node, "NVIDIA", "2", "GPU-2", "A100")
	d3.UtilizationRatio = resourceview.Quantity{State: &view.FieldState{State: resourceview.Conflict}}
	d3.MemoryUsedBytes = testMetric("20Gi", "bytes")
	d3.MemoryPhysicalBytes = testMetric("16Gi", "bytes")

	src := overviewSource{
		Nodes:        []resourceview.Node{node},
		Devices:      []resourceview.Device{d1, d2, d3},
		NodeCoverage: &view.Coverage{Entity: "node", Complete: true},
	}
	reply, err := (&NodeService{source: &src}).GetAllNodes(context.Background(), &pb.GetAllNodesReq{UseResourceInventory: true})
	if err != nil || len(reply.List) != 1 {
		t.Fatalf("GetAllNodes = %v, err = %v", reply, err)
	}
	row := reply.List[0]
	wantProviders := []string{"Hygon", "NVIDIA"}
	if len(row.Providers) != len(wantProviders) || row.Providers[0] != wantProviders[0] || row.Providers[1] != wantProviders[1] {
		t.Fatalf("Providers = %v, want %v", row.Providers, wantProviders)
	}
	if row.ComputeUtilizationPercent == nil || math.Abs(*row.ComputeUtilizationPercent-60) > 1e-12 {
		t.Fatalf("ComputeUtilizationPercent = %v, want 60", row.ComputeUtilizationPercent)
	}
	if row.MemoryUsagePercent == nil || math.Abs(*row.MemoryUsagePercent-50) > 1e-12 {
		t.Fatalf("MemoryUsagePercent = %v, want 50", row.MemoryUsagePercent)
	}
}

func TestNodeInventoryUnresolvedDeviceMetadata(t *testing.T) {
	node := testNode("ascend-910c-000", proto.Bool(true))
	node.DeviceCoverage = &view.Coverage{Entity: "device", Complete: false}

	module := testCard(node, "Ascend", "C-0", "", "Ascend910C")
	module.Kind = "unknown"
	module.UUID = nil
	module.States["kind"] = &view.FieldState{State: resourceview.Unknown, Source: "node_registration", Scope: "device", Reason: "unverified"}
	module.States["model"] = &view.FieldState{State: resourceview.Known, Source: "node_registration", Scope: "device"}

	unverifiedModel := testCard(node, "Ascend", "C-1", "", "UnverifiedModel")
	unverifiedModel.Kind = "unknown"
	unverifiedModel.UUID = nil
	unverifiedModel.States["kind"] = &view.FieldState{State: resourceview.Unknown}
	unverifiedModel.States["model"] = &view.FieldState{State: resourceview.Unknown}

	childInstance := testCard(node, "Other", "I-0", "", "ChildInstance")
	childInstance.Kind = "instance"
	parent := module.Identity
	childInstance.ParentIdentity = &parent
	childInstance.States["kind"] = &view.FieldState{State: resourceview.Known}
	childInstance.States["parentId"] = &view.FieldState{State: resourceview.Known}
	childInstance.States["model"] = &view.FieldState{State: resourceview.Known}

	src := overviewSource{
		Nodes:        []resourceview.Node{node},
		Devices:      []resourceview.Device{module, unverifiedModel, childInstance},
		NodeCoverage: &view.Coverage{Entity: "node", Complete: true},
	}
	ctx := context.Background()
	nodes := &NodeService{source: &src}

	all, err := nodes.GetAllNodes(ctx, &pb.GetAllNodesReq{UseResourceInventory: true})
	if err != nil || len(all.List) != 1 {
		t.Fatalf("GetAllNodes = %v, err = %v", all, err)
	}
	row := all.List[0]
	if row.CardCnt != nil || row.InventoryCoverage == nil || row.InventoryCoverage.GetComplete() {
		t.Fatalf("expected uncounted unresolved device coverage, got CardCnt=%v coverage=%v", row.CardCnt, row.InventoryCoverage)
	}
	if len(row.Type) != 1 || row.Type[0] != "Ascend910C" {
		t.Fatalf("Type = %v, want [Ascend910C]", row.Type)
	}
	if len(row.Providers) != 1 || row.Providers[0] != "Ascend" {
		t.Fatalf("Providers = %v, want [Ascend]", row.Providers)
	}

	detail, err := nodes.GetNode(ctx, &pb.GetNodeReq{Uid: string(node.UID), UseResourceInventory: true})
	if err != nil || !proto.Equal(row, detail) {
		t.Fatalf("GetNode = %v, err = %v, want %v", detail, err, row)
	}

	filtered, err := nodes.GetAllNodes(ctx, &pb.GetAllNodesReq{
		UseResourceInventory: true,
		Filters:              &pb.GetAllNodesReq_Filters{Type: "Ascend910C"},
	})
	if err != nil || len(filtered.List) != 1 || filtered.List[0].Uid != row.Uid {
		t.Fatalf("filtered by Ascend910C = %v, err = %v", filtered, err)
	}

	overview, err := (&OverviewService{source: &src}).GetInventory(ctx, &view.InventoryRequest{})
	if err != nil || len(overview.Nodes) != 1 || overview.Nodes[0].PhysicalDevices != nil || len(overview.Devices) != 0 {
		t.Fatalf("overview leaked unresolved physical devices: %v, err = %v", overview, err)
	}
}

func BenchmarkInventoryNodeList(b *testing.B) {
	for _, n := range []int{501, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			snapshot := overviewSource{NodeCoverage: &view.Coverage{Complete: true}}
			for i := 0; i < n; i++ {
				node := testNode(fmt.Sprintf("n-%05d", i), proto.Bool(true))
				snapshot.Nodes = append(snapshot.Nodes, node)
				snapshot.Devices = append(snapshot.Devices, testCard(node, "NVIDIA", "card", "gpu", "A100"))
			}
			s := &NodeService{source: snapshot}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r, err := s.GetAllNodes(context.Background(), &pb.GetAllNodesReq{UseResourceInventory: true, Filters: &pb.GetAllNodesReq_Filters{}})
				if err != nil || len(r.List) != n {
					b.Fatalf("got %d nodes, err %v", len(r.List), err)
				}
				b.ReportMetric(float64(proto.Size(r)), "proto-bytes/op")
			}
		})
	}
}
