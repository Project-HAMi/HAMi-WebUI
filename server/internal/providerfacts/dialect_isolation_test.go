package providerfacts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/provider/registry"
	"vgpu/internal/resourceview"
)

func TestCorePackagesHaveZeroVendorImportsOrLiterals(t *testing.T) {
	strictTokens := []string{"nvidia", "hygon", "hcu", "dcu", "metax", "ascend", "mlu", "cambricon", "mthreads", "amd.com", "huawei"}
	podfactsTokens := []string{"hygon", "hcu", "dcu", "metax", "ascend", "mlu", "cambricon", "mthreads", "huawei"}

	pkgs := []struct {
		dir            string
		allowedImports map[string]bool
		forbiddenToken []string
	}{
		{dir: ".", allowedImports: map[string]bool{"vgpu/internal/provider": true}, forbiddenToken: strictTokens},
		{dir: "../resourceview", forbiddenToken: strictTokens},
		{dir: "../provider", allowedImports: map[string]bool{"vgpu/internal/provider/util": true}, forbiddenToken: strictTokens},
		{dir: "../podfacts", forbiddenToken: podfactsTokens},
	}

	fset := token.NewFileSet()
	for _, pkg := range pkgs {
		entries, err := os.ReadDir(pkg.dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", pkg.dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(pkg.dir, name)
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			for _, imp := range file.Imports {
				impPath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					continue
				}
				if pkg.allowedImports[impPath] {
					continue
				}
				if strings.HasPrefix(impPath, "vgpu/internal/provider") {
					t.Errorf("%s imports forbidden provider package %q", path, impPath)
				}
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				lower := strings.ToLower(val)
				for _, tok := range pkg.forbiddenToken {
					if strings.Contains(lower, tok) {
						t.Errorf("%s contains forbidden vendor literal %q (%q)", path, tok, val)
					}
				}
				return true
			})
		}
	}
}

type syntheticPoolDialect struct {
	name     string
	resource string
}

func (d syntheticPoolDialect) Name() string        { return d.name }
func (d syntheticPoolDialect) SelectorKey() string { return "" }
func (d syntheticPoolDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	return name == d.resource
}
func (syntheticPoolDialect) OwnsRegistrationAnnotation(string, *devicecatalog.Snapshot) bool {
	return false
}
func (syntheticPoolDialect) ExtractNode(*corev1.Node, *devicecatalog.Snapshot, bool, *log.Helper, time.Time) provider.NodeInventory {
	return provider.NodeInventory{}
}

func TestSyntheticNodePoolOnlyVendorRequiresZeroCoreChanges(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "acme-node", UID: "acme-uid"},
		Status: corev1.NodeStatus{
			Capacity:    corev1.ResourceList{"acme.example/vpu": resource.MustParse("4")},
			Allocatable: corev1.ResourceList{"acme.example/vpu": resource.MustParse("4")},
		},
	}
	dialects := []provider.InventoryDialect{
		syntheticPoolDialect{name: "AcmeVPU", resource: "acme.example/vpu"},
	}
	pods := podfacts.Inventory{Coverage: completeTestCoverage("pod", "shared_pod_informer", 0)}
	snap := Build([]*corev1.Node{node}, nil, pods, nil, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1), dialects)

	if len(snap.Devices) != 0 {
		t.Fatalf("expected 0 devices for pool-only vendor, got %d", len(snap.Devices))
	}
	if len(snap.Nodes) != 1 || len(snap.Nodes[0].Resources) != 1 || snap.Nodes[0].Resources[0].Resource != "acme.example/vpu" {
		t.Fatalf("expected acme.example/vpu node resource pool, got %+v", snap.Nodes)
	}
	if snap.Nodes[0].Resources[0].Capacity.Unit != "resource_units" {
		t.Fatalf("unexpected unit: %s", snap.Nodes[0].Resources[0].Capacity.Unit)
	}
	if snap.DeviceCoverage.Complete {
		t.Fatal("positive unmapped accelerator pool should keep DeviceCoverage incomplete")
	}
}

type syntheticCardDialect struct{}

func (syntheticCardDialect) Name() string        { return "Acme" }
func (syntheticCardDialect) SelectorKey() string { return "Acme" }
func (syntheticCardDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	return name == "acme.example/gpu"
}
func (syntheticCardDialect) OwnsRegistrationAnnotation(key string, _ *devicecatalog.Snapshot) bool {
	return key == "acme.example/register"
}
func (syntheticCardDialect) ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	if !selected || node == nil {
		return provider.NodeInventory{}
	}
	if _, ok := node.Annotations["acme.example/register"]; !ok {
		return provider.NodeInventory{}
	}
	dev0 := resourceview.Device{
		Identity:             resourceview.DeviceIdentity{NodeUID: node.UID, Node: node.Name, Provider: "Acme", DeviceID: "Acme-0"},
		Physical:             resourceview.PhysicalAssociation{Domain: "acme-board", BoardKey: "slot-0", Priority: 1},
		Kind:                 "physical",
		Vendor:               "acme",
		Model:                "Acme-X1",
		AllocationSource:     "Acme-VPU",
		BindingAliases:       []string{node.Name + "-acme-0"},
		ResourceKeys:         []string{"acme.example/gpu"},
		Resource:             "acme.example/gpu",
		SlotsCapacity:        number(4, "slots", "node_registration", "device"),
		ComputeCapacityShare: number(1, "share", "node_registration", "device"),
		MemoryCapacityBytes:  number(16*1024*1024*1024, "bytes", "node_registration", "device"),
		Sources:              []*viewv1.SourceState{{Source: "hami_registration:Acme", State: resourceview.Known}},
		States:               map[string]*viewv1.FieldState{"kind": provider.KnownState("node_registration", "device")},
	}
	dev1 := dev0
	dev1.Identity.DeviceID = "acme-raw-1"
	dev1.Physical = resourceview.PhysicalAssociation{Domain: "acme-board", BoardKey: "slot-1", Priority: 1}
	dev1.BindingAliases = nil
	dev1.RequireVerifiedRawBinding = true
	if catalog.Loaded() {
		configured := true
		dev0.Configured = &configured
		dev1.Configured = &configured
	}
	return provider.NodeInventory{
		Devices: []resourceview.Device{dev0, dev1},
		Sources: []*viewv1.SourceState{{Source: "hami_registration:Acme", State: resourceview.Known}},
	}
}

func TestSyntheticCardVendorRequiresZeroCoreChanges(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "node-acme",
			UID:         "uid-acme",
			Annotations: map[string]string{"acme.example/register": "present"},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{"acme.example/gpu": resource.MustParse("8")},
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "acme-pod", Namespace: "work", UID: "pod-acme", ResourceVersion: "1"},
		Spec:       corev1.PodSpec{NodeName: node.Name, Containers: []corev1.Container{{Name: "app"}}},
	}
	alloc := podfacts.AllocationSnapshot{
		Accounting:      podfacts.AccountingPodPeak,
		PodUID:          pod.UID,
		ResourceVersion: pod.ResourceVersion,
		Complete:        true,
		Source:          &viewv1.SourceState{Source: "hami_annotation:Acme-VPU", State: resourceview.Known},
		Records: []podfacts.Allocation{
			{
				Container: "app", RecordID: "r0", Node: node.Name, NodeUID: node.UID,
				Resource: "Acme-VPU", DeviceID: "node-acme-acme-0",
				Quantities: map[string]podfacts.Quantity{"memory": number(1024*1024, "bytes", "hami_annotation", "container"), "compute": number(50, "percent", "hami_annotation", "container"), "slots": number(1, "slots", "hami_annotation", "container")},
			},
			{
				Container: "app", RecordID: "r1", Node: node.Name, NodeUID: node.UID,
				Resource: "Acme-VPU", DeviceID: "", RawDeviceID: "acme-raw-1",
				Quantities: map[string]podfacts.Quantity{"memory": number(2048*1024, "bytes", "hami_annotation", "container"), "compute": number(50, "percent", "hami_annotation", "container"), "slots": number(1, "slots", "hami_annotation", "container")},
			},
		},
	}
	facts, err := podfacts.Extract(pod, podfacts.Evidence{Allocations: []podfacts.AllocationSnapshot{alloc}}, podfacts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ledger := podfacts.NewLedger()
	ledger.Put(facts, func() bool { return true })
	pods := ledger.Snapshot()
	pods.Coverage = completeTestCoverage("pod", "shared_pod_informer", 1)

	catalog := &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}
	snap := Build([]*corev1.Node{node}, catalog, pods, map[string]labels.Selector{"Acme": labels.Everything()}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1), []provider.InventoryDialect{syntheticCardDialect{}})

	if len(snap.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(snap.Devices))
	}
	if len(snap.VerifiedBindings) != 2 {
		t.Fatalf("expected 2 VerifiedBindings, got %+v", snap.VerifiedBindings)
	}
	if pods.Entries[0].Facts.Containers[0].Allocations[0].Records[1].DeviceID != "" {
		t.Fatal("raw binding fallback mutated pod inventory ledger")
	}
	for i, d := range snap.Devices {
		if len(d.EffectiveOccupancies) != 1 {
			t.Fatalf("device %d missing occupancy: %+v", i, d.EffectiveOccupancies)
		}
		if !d.AllocationCoverage.Complete {
			t.Fatalf("device %d AllocationCoverage incomplete: %+v", i, d.AllocationCoverage)
		}
	}
	if !snap.DeviceCoverage.Complete {
		t.Fatalf("expected complete DeviceCoverage, got %+v", snap.DeviceCoverage)
	}
	if groups := resourceview.GroupPhysicalDevices(snap); len(groups.Groups) != 2 {
		t.Fatalf("expected 2 physical groups, got %d", len(groups.Groups))
	}
}

func TestNonAcceleratorVendorPrefixedResourceIsIgnored(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "rdma-node", UID: "rdma-uid"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{"nvidia.com/mlx5_0": resource.MustParse("1")},
		},
	}
	pods := podfacts.Inventory{Coverage: completeTestCoverage("pod", "shared_pod_informer", 0)}
	snap := Build([]*corev1.Node{node}, testCatalog(), pods, nil, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1), registry.DefaultInventoryDialects())
	if len(snap.Nodes) != 1 || len(snap.Nodes[0].Resources) != 0 {
		t.Fatalf("expected non-accelerator RDMA resource ignored, got %+v", snap.Nodes)
	}
	if !snap.DeviceCoverage.Complete {
		t.Fatalf("expected complete DeviceCoverage with non-accelerator resource, got %+v", snap.DeviceCoverage)
	}
}

func TestDialectEdgeCasesAmbiguousAliasPeakMergeCoverageCloneAndResourceUnion(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "edge-node", UID: "edge-uid"},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				"acme.example/gpu":         resource.MustParse("8"),
				"acme.example/extra-cores": resource.MustParse("100"),
			},
		},
	}

	sharedCov := &viewv1.Coverage{Entity: "device", Observed: 1, Sources: []*viewv1.SourceState{{Source: "shared_node_informer", State: resourceview.Known}}}
	devA := resourceview.Device{
		Identity:             resourceview.DeviceIdentity{NodeUID: node.UID, Node: node.Name, Provider: "Acme", DeviceID: "card-a"},
		Kind:                 "physical",
		Vendor:               "acme",
		Resource:             "acme.example/gpu",
		ResourceKeys:         []string{"acme.example/gpu"},
		AllocationSource:     "Acme",
		BindingAliases:       []string{"alias-a"},
		SlotsCapacity:        number(4, "slots", "node_registration", "device"),
		ComputeCapacityShare: number(1, "share", "node_registration", "device"),
		MemoryCapacityBytes:  number(1024*1024, "bytes", "node_registration", "device"),
		Coverage:             sharedCov,
		States:               map[string]*viewv1.FieldState{"kind": provider.KnownState("node_registration", "device")},
	}
	devB := devA
	devB.Identity.DeviceID = "card-b"
	devB.BindingAliases = []string{"card-c"} // collides with card-c's canonical ID
	devB.ResourceKeys = []string{"acme.example/gpu", "acme.example/extra-cores"}
	devC := devA
	devC.Identity.DeviceID = "card-c"
	devC.BindingAliases = []string{"card-c-alt"}
	configured := true
	devA.Configured, devB.Configured, devC.Configured = &configured, &configured, &configured

	covDialect := &staticInventoryDialect{
		name:     "Acme",
		resource: "acme.example/gpu",
		extraRes: "acme.example/extra-cores",
		inv: provider.NodeInventory{
			Devices: []resourceview.Device{devA, devB, devC},
			Sources: []*viewv1.SourceState{{Source: "hami_registration:Acme", State: resourceview.Known}},
		},
	}

	// Pod mixing alias-a (init) and card-a (app) should merge into 1 occupancy, while
	// colliding "card-c" allocation must be rejected as ambiguous.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns", UID: "pod-edge", ResourceVersion: "1"},
		Spec: corev1.PodSpec{
			NodeName:       node.Name,
			InitContainers: []corev1.Container{{Name: "init"}},
			Containers:     []corev1.Container{{Name: "app"}},
		},
	}
	alloc := podfacts.AllocationSnapshot{
		Accounting: podfacts.AccountingPodPeak, PodUID: pod.UID, ResourceVersion: pod.ResourceVersion, Complete: true,
		Source: &viewv1.SourceState{Source: "hami_annotation:Acme", State: resourceview.Known},
		Records: []podfacts.Allocation{
			{Container: "init", RecordID: "r-init", Node: node.Name, NodeUID: node.UID, Resource: "Acme", DeviceID: "alias-a", Quantities: map[string]podfacts.Quantity{"memory": number(2*1024*1024, "bytes", "hami_annotation", "container"), "slots": number(1, "slots", "hami_annotation", "container")}},
			{Container: "app", RecordID: "r-app", Node: node.Name, NodeUID: node.UID, Resource: "Acme", DeviceID: "card-a", Quantities: map[string]podfacts.Quantity{"memory": number(3*1024*1024, "bytes", "hami_annotation", "container"), "slots": number(1, "slots", "hami_annotation", "container")}},
		},
	}
	facts, err := podfacts.Extract(pod, podfacts.Evidence{Allocations: []podfacts.AllocationSnapshot{alloc}}, podfacts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ledger := podfacts.NewLedger()
	ledger.Put(facts, func() bool { return true })
	pods := ledger.Snapshot()
	pods.Coverage = completeTestCoverage("pod", "shared_pod_informer", 1)

	dialects := []provider.InventoryDialect{covDialect}
	snap1 := Build([]*corev1.Node{node}, &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}, pods, nil, nil, time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1), dialects)
	snap2 := Build([]*corev1.Node{node}, &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}, pods, nil, nil, time.Unix(101, 0), completeTestCoverage("node", "shared_node_informer", 1), dialects)

	if len(sharedCov.Sources) != 1 {
		t.Fatalf("dialect Coverage was mutated across Build calls: %+v", sharedCov.Sources)
	}
	if !snap1.DeviceCoverage.Complete || !snap2.DeviceCoverage.Complete {
		t.Fatalf("later card ResourceKeys union or Coverage clone broke DeviceCoverage: %+v", snap1.DeviceCoverage)
	}
	if len(snap1.Devices[0].EffectiveOccupancies) != 1 || snap1.Devices[0].EffectiveOccupancies[0].MemoryAllocatedBytes.Value.Value() != 3*1024*1024 {
		t.Fatalf("expected init+app canonicalized to single peak occupancy (3Mi), got %+v", snap1.Devices[0].EffectiveOccupancies)
	}

	// Ambiguous canonical/alias collision on "card-c" must not bind to card-b or card-c.
	podColliding := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "ns", UID: "pod-col", ResourceVersion: "1"},
		Spec:       corev1.PodSpec{NodeName: node.Name, Containers: []corev1.Container{{Name: "app"}}},
	}
	allocCol := podfacts.AllocationSnapshot{
		Accounting: podfacts.AccountingPodPeak, PodUID: podColliding.UID, ResourceVersion: podColliding.ResourceVersion, Complete: true,
		Source:  &viewv1.SourceState{Source: "hami_annotation:Acme", State: resourceview.Known},
		Records: []podfacts.Allocation{{Container: "app", RecordID: "r0", Node: node.Name, NodeUID: node.UID, Resource: "Acme", DeviceID: "card-c"}},
	}
	factsCol, err := podfacts.Extract(podColliding, podfacts.Evidence{Allocations: []podfacts.AllocationSnapshot{allocCol}}, podfacts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ledger.Put(factsCol, func() bool { return true })
	podsCol := ledger.Snapshot()
	podsCol.Coverage = completeTestCoverage("pod", "shared_pod_informer", 2)
	snapCol := Build([]*corev1.Node{node}, &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}, podsCol, nil, nil, time.Unix(102, 0), completeTestCoverage("node", "shared_node_informer", 1), dialects)
	for _, b := range snapCol.VerifiedBindings {
		if b.PodUID == podColliding.UID {
			t.Fatalf("ambiguous ID bound to device: %+v", b)
		}
	}
	if len(snapCol.Devices[1].EffectiveOccupancies) != 0 || len(snapCol.Devices[2].EffectiveOccupancies) != 0 {
		t.Fatalf("ambiguous ID attached occupancy to card-b or card-c")
	}

	// Unique alias "card-c-alt" on card-c must bind and attach occupancy even when canonical "card-c" collides.
	podAlt := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "ns", UID: "pod-alt", ResourceVersion: "1"},
		Spec:       corev1.PodSpec{NodeName: node.Name, Containers: []corev1.Container{{Name: "app"}}},
	}
	allocAlt := podfacts.AllocationSnapshot{
		Accounting: podfacts.AccountingPodPeak, PodUID: podAlt.UID, ResourceVersion: podAlt.ResourceVersion, Complete: true,
		Source:  &viewv1.SourceState{Source: "hami_annotation:Acme", State: resourceview.Known},
		Records: []podfacts.Allocation{{Container: "app", RecordID: "r0", Node: node.Name, NodeUID: node.UID, Resource: "Acme", DeviceID: "card-c-alt", Quantities: map[string]podfacts.Quantity{"memory": number(4*1024*1024, "bytes", "hami_annotation", "container")}}},
	}
	factsAlt, err := podfacts.Extract(podAlt, podfacts.Evidence{Allocations: []podfacts.AllocationSnapshot{allocAlt}}, podfacts.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ledgerAlt := podfacts.NewLedger()
	ledgerAlt.Put(factsAlt, func() bool { return true })
	podsAlt := ledgerAlt.Snapshot()
	podsAlt.Coverage = completeTestCoverage("pod", "shared_pod_informer", 1)
	snapAlt := Build([]*corev1.Node{node}, &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}, podsAlt, nil, nil, time.Unix(103, 0), completeTestCoverage("node", "shared_node_informer", 1), dialects)
	if len(snapAlt.Devices[2].EffectiveOccupancies) != 1 || snapAlt.Devices[2].EffectiveOccupancies[0].MemoryAllocatedBytes.Value.Value() != 4*1024*1024 {
		t.Fatalf("unique alias under canonical collision lost occupancy: %+v", snapAlt.Devices[2].EffectiveOccupancies)
	}
}

type staticInventoryDialect struct {
	name, resource, extraRes string
	inv                      provider.NodeInventory
}

func (d *staticInventoryDialect) Name() string        { return d.name }
func (d *staticInventoryDialect) SelectorKey() string { return "" }
func (d *staticInventoryDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	return name == d.resource || name == d.extraRes
}
func (d *staticInventoryDialect) OwnsRegistrationAnnotation(string, *devicecatalog.Snapshot) bool {
	return false
}
func (d *staticInventoryDialect) ExtractNode(*corev1.Node, *devicecatalog.Snapshot, bool, *log.Helper, time.Time) provider.NodeInventory {
	return d.inv
}
