package data

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
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
	"vgpu/internal/providerfacts"
	"vgpu/internal/resourceview"
)

func TestPodFactAllocationsHasZeroVendorImportsOrLiterals(t *testing.T) {
	forbiddenTokens := []string{"nvidia", "hygon", "hcu", "dcu", "metax", "ascend", "mlu", "cambricon", "mthreads", "amd.com", "huawei"}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "pod_fact_allocations.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse pod_fact_allocations.go: %v", err)
	}
	for _, imp := range file.Imports {
		impPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if impPath != "vgpu/internal/provider" && strings.HasPrefix(impPath, "vgpu/internal/provider") {
			t.Errorf("pod_fact_allocations.go imports forbidden provider package %q", impPath)
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
		for _, tok := range forbiddenTokens {
			if strings.Contains(lower, tok) {
				t.Errorf("pod_fact_allocations.go contains forbidden vendor literal %q (%q)", tok, val)
			}
		}
		return true
	})
}

type syntheticAcmeDialect struct{}

func (syntheticAcmeDialect) Name() string        { return "Acme" }
func (syntheticAcmeDialect) SelectorKey() string { return "Acme" }

func (syntheticAcmeDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	return name == "acme.example/vpu"
}

func (syntheticAcmeDialect) OwnsRegistrationAnnotation(key string, _ *devicecatalog.Snapshot) bool {
	return key == "acme.example/register"
}

func (syntheticAcmeDialect) ExtractNode(node *corev1.Node, _ *devicecatalog.Snapshot, selected bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	if !selected || node == nil || node.Annotations["acme.example/register"] == "" {
		return provider.NodeInventory{}
	}
	configured := true
	dev := resourceview.Device{
		Identity:         resourceview.DeviceIdentity{NodeUID: node.UID, Node: node.Name, Provider: "Acme", DeviceID: "Acme-0"},
		Kind:             "physical",
		Vendor:           "acme",
		Model:            "Acme-X1",
		AllocationSource: "Acme-VPU",
		Resource:         "acme.example/vpu",
		ResourceKeys:     []string{"acme.example/vpu"},
		Configured:       &configured,
		SlotsCapacity:    resourceview.Quantity{Value: resource.NewQuantity(4, resource.DecimalSI), Unit: "slots", State: provider.KnownState("node_registration", "device")},
		ComputeCapacityShare: resourceview.Quantity{
			Value: resource.NewQuantity(1, resource.DecimalSI), Unit: "share", State: provider.KnownState("node_registration", "device"),
		},
		MemoryCapacityBytes: resourceview.Quantity{
			Value: resource.NewQuantity(8192*1024*1024, resource.BinarySI), Unit: "bytes", State: provider.KnownState("node_registration", "device"),
		},
		Sources: []*viewv1.SourceState{{Source: "hami_registration:Acme", State: resourceview.Known}},
		States:  map[string]*viewv1.FieldState{"kind": provider.KnownState("node_registration", "device")},
	}
	return provider.NodeInventory{
		Devices: []resourceview.Device{dev},
		Sources: []*viewv1.SourceState{{Source: "hami_registration:Acme", State: resourceview.Known}},
	}
}

func (syntheticAcmeDialect) CatalogResourceNames(_ *devicecatalog.Snapshot) []corev1.ResourceName {
	return []corev1.ResourceName{"acme.example/vpu"}
}

func (syntheticAcmeDialect) ExtractPodAllocations(ctx provider.PodAllocationContext) provider.PodAllocationResult {
	return provider.DecodeStandardPodAllocation(ctx, provider.StandardAllocationSpec{
		Kind:          "Acme-VPU",
		AnnotationKey: "hami.io/acme-vpu-devices-allocated",
		CountResource: "acme.example/vpu",
	})
}

func TestSyntheticAllocationDialectRequiresZeroCoreChanges(t *testing.T) {
	catalog := &mutableCatalog{current: &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}}
	dialects := []provider.AllocationDialect{syntheticAcmeDialect{}}

	// 1. Allocated Pod decoded cleanly through synthetic dialect.
	allocated := factTestPod()
	allocated.Spec.InitContainers = nil
	allocated.Spec.EphemeralContainers = nil
	allocated.Annotations = map[string]string{
		provider.AssignedNodeAnnotation:      "node-a",
		"hami.io/acme-vpu-devices-allocated": "Acme-0,Acme-VPU,2048,50:;",
	}
	repo := factRepo(t, catalog, allocated)
	repo.allocationDialects = dialects
	entry := factEntry(t, repo, allocated)
	if entry.Facts.Candidate != podfacts.CandidateAccelerator {
		t.Fatalf("expected CandidateAccelerator, got %s", entry.Facts.Candidate)
	}
	if set := entry.Facts.Containers[0].Allocations[0]; !set.Complete || set.Source.State != resourceview.Known || set.Source.Source != "hami_annotation:Acme-VPU" {
		t.Fatalf("unexpected allocation source state: %+v", set)
	}
	if len(entry.Occupancies) != 1 || entry.Occupancies[0].Quantities["memory"].Value.Value() != 2048*1024*1024 {
		t.Fatalf("unexpected occupancies: %+v", entry.Occupancies)
	}

	// 2. End-to-end join against providerfacts.Build with synthetic InventoryDialect.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", Annotations: map[string]string{"acme.example/register": "present"}},
		Status:     corev1.NodeStatus{Capacity: corev1.ResourceList{"acme.example/vpu": resource.MustParse("4")}},
	}
	podsSnap := repo.facts.Snapshot()
	total := int32(1)
	podsSnap.Coverage = &viewv1.Coverage{Entity: "pod", Observed: 1, Total: &total, Complete: true, Sources: []*viewv1.SourceState{{Source: "shared_pod_informer", State: resourceview.Known}}}
	nodeCov := &viewv1.Coverage{Entity: "node", Observed: 1, Total: &total, Complete: true, Sources: []*viewv1.SourceState{{Source: "shared_node_informer", State: resourceview.Known}}}
	clusterSnap := providerfacts.Build(
		[]*corev1.Node{node},
		catalog.Snapshot(),
		podsSnap,
		map[string]labels.Selector{"Acme": labels.Everything()},
		log.NewHelper(log.NewStdLogger(io.Discard)),
		time.Unix(100, 0),
		nodeCov,
		[]provider.InventoryDialect{syntheticAcmeDialect{}},
	)
	if !clusterSnap.DeviceCoverage.Complete || len(clusterSnap.VerifiedBindings) != 1 || len(clusterSnap.Devices[0].EffectiveOccupancies) != 1 {
		t.Fatalf("end-to-end synthetic join failed: devices=%+v bindings=%+v cov=%+v", clusterSnap.Devices, clusterSnap.VerifiedBindings, clusterSnap.DeviceCoverage)
	}

	// 3. Requested-only Pod classified via dialect CatalogResourceNames without annotations.
	requested := factTestPod()
	requested.UID = "uid-req"
	requested.Annotations = nil
	requested.Spec.Containers[0].Resources.Limits = corev1.ResourceList{"acme.example/vpu": resource.MustParse("1")}
	if err := repo.podIndexer.Add(requested); err != nil {
		t.Fatal(err)
	}
	reqEntry := factEntry(t, repo, requested)
	if reqEntry.Facts.Candidate != podfacts.CandidateAccelerator || len(reqEntry.Occupancies) != 0 {
		t.Fatalf("expected requested-only CandidateAccelerator with 0 occupancies, got %+v", reqEntry)
	}

	// 4. Unrecognized allocator annotation still triggers version_unsupported.
	unknown := factTestPod()
	unknown.UID = "uid-unk"
	unknown.Annotations = map[string]string{"hami.io/other-devices-allocated": "card,OTHER,4096,0:;"}
	if err := repo.podIndexer.Add(unknown); err != nil {
		t.Fatal(err)
	}
	unkEntry := factEntry(t, repo, unknown)
	if unkEntry.Facts.Candidate != podfacts.CandidateUnresolved || unkEntry.Facts.Containers[0].Allocations[0].Source.Reason != "version_unsupported" {
		t.Fatalf("expected version_unsupported for unrecognized key, got %+v", unkEntry.Facts)
	}
}
