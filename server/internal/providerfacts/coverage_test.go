package providerfacts

import (
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/resourceview"
)

func completeTestCoverage(entity, source string, count int32) *viewv1.Coverage {
	return &viewv1.Coverage{Entity: entity, Observed: count, Total: &count, Complete: true, Sources: []*viewv1.SourceState{{Source: source, State: resourceview.Known}}}
}
func TestContinuousEmptyHAMiScopeConfirmsZeroButSourceGapsDoNot(t *testing.T) {
	n := testNode()
	cat := testCatalog()
	buildProof := func(p podfacts.Inventory) resourceview.Snapshot {
		return Build([]*corev1.Node{n}, cat, p, map[string]labels.Selector{"NVIDIA": labels.Everything()}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1))
	}
	empty := podfacts.Inventory{Coverage: completeTestCoverage("pod", "shared_pod_informer", 0)}
	got := buildProof(empty)
	if !got.Devices[0].AllocationCoverage.Complete || got.Devices[0].AllocationCoverage.Total == nil || *got.Devices[0].AllocationCoverage.Total != 0 || !got.DeviceCoverage.Complete {
		t.Fatal("proved empty source did not confirm zero")
	}
	for _, tc := range []string{"pod_gap", "bad_allocator", "missing_allocation", "partial_records", "unknown_compute", "foreign_unknown"} {
		t.Run(tc, func(t *testing.T) {
			p := allocationInventory(t, n)
			p.Coverage = completeTestCoverage("pod", "shared_pod_informer", 1)
			switch tc {
			case "pod_gap":
				p.Coverage.Complete = false
				p.Coverage.Total = nil
				p.Coverage.Sources[0].State = resourceview.Unknown
				p.Coverage.Sources[0].Reason = "unverified"
			case "bad_allocator":
				p.Entries[0].Facts.Containers[0].Allocations[0].Source.State = resourceview.Unknown
			case "missing_allocation":
				for i := range p.Entries[0].Facts.Containers {
					p.Entries[0].Facts.Containers[i].Allocations = nil
				}
				p.Entries[0].Facts.Containers[0].Requests = []podfacts.Request{{Resource: "nvidia.com/gpu", Positive: true}}
				p.Entries[0].Occupancies = nil
			case "partial_records":
				p.Entries[0].Facts.Containers[0].Allocations[0].Complete = false
			case "unknown_compute":
				r := &p.Entries[0].Facts.Containers[0].Allocations[0].Records[0]
				r.Quantities["compute"] = absent("percent", "allocation", "container", "unverified")
				p.Entries[0].Occupancies = podfacts.EffectiveOccupancies(p.Entries[0].Facts)
			case "foreign_unknown":
				p.Entries[0].Facts.NodeName = "other"
				for i := range p.Entries[0].Facts.Containers {
					for j := range p.Entries[0].Facts.Containers[i].Allocations {
						p.Entries[0].Facts.Containers[i].Allocations[j].Source.State = resourceview.Unknown
					}
				}
				p.Entries[0].Occupancies = nil
			}
			c := buildProof(p).Devices[0].AllocationCoverage
			want := tc == "unknown_compute" || tc == "foreign_unknown"
			if c.Complete != want {
				t.Fatalf("complete=%v want=%v sources=%+v", c.Complete, want, c.Sources)
			}
		})
	}
}
func TestInventoryContinuityCannotHideMalformedOrUnmappedProviders(t *testing.T) {
	n := testNode()
	cat := testCatalog()
	p := podfacts.Inventory{Coverage: completeTestCoverage("pod", "pods", 0)}
	proof := completeTestCoverage("node", "nodes", 1)
	buildProof := func() resourceview.Snapshot {
		return Build([]*corev1.Node{n}, cat, p, map[string]labels.Selector{"NVIDIA": labels.Everything()}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Time{}, proof)
	}
	n.Annotations["hami.io/node-nvidia-register"] = "broken"
	n.Status.Capacity = nil
	n.Status.Allocatable = nil
	if buildProof().DeviceCoverage.Complete {
		t.Fatal("malformed registration washed by healthy Node watch")
	}
	n.Annotations = nil
	cat.ResourceNames["NVIDIA"] = devicecatalog.DeviceResourceNames{Count: "private.example/accelerator"}
	n.Status.Capacity = corev1.ResourceList{"private.example/accelerator": resource.MustParse("1")}
	if buildProof().DeviceCoverage.Complete {
		t.Fatal("resource with configured accelerator count but no registration claimed complete")
	}
	n.Status.Capacity = nil
	if s := buildProof(); !s.DeviceCoverage.Complete || *s.DeviceCoverage.Total != 0 {
		t.Fatal("complete empty Node inventory failed to confirm no observable devices")
	}
}

func TestNVIDIARegistrationOwnerRequiresRegistration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		registered bool
		advertised bool
		wantCards  int32
	}{
		{"owner with registration", true, true, 1},
		{"registration missing", false, true, 0},
		{"only owner remains", false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := testNode()
			node.Annotations["hami.io/node-nvidia-register-owner"] = "plugin-owner"
			if !tc.registered {
				delete(node.Annotations, nvidia.RegisterAnnos)
			}
			if !tc.advertised {
				node.Status.Capacity, node.Status.Allocatable = nil, nil
			}
			s := Build([]*corev1.Node{node}, testCatalog(), podfacts.Inventory{}, map[string]labels.Selector{"NVIDIA": labels.Everything()},
				log.NewHelper(log.NewStdLogger(io.Discard)), time.Time{}, completeTestCoverage("node", "nodes", 1))
			for _, c := range []*viewv1.Coverage{s.DeviceCoverage, s.Nodes[0].DeviceCoverage} {
				if c.Observed != tc.wantCards || c.Complete != tc.registered || (c.Total != nil) != tc.registered {
					t.Errorf("unexpected device coverage %v", c)
				}
			}
		})
	}
}

func TestAcceleratorCoverageScopesNodeResources(t *testing.T) {
	const core = "cambricon.com/mlu.smlu.vcore"
	const memory = "cambricon.com/mlu.smlu.vmemory"
	const rdma = "rdma/rdma_shared_device_a"
	catalog := testCatalog()
	catalog.Cambricon = &devicecatalog.CambriconConfig{ResourceCountName: "cambricon.com/vmlu", ResourceMemoryName: memory, ResourceCoreName: core}
	catalog.ResourceNames["NVIDIA"] = devicecatalog.DeviceResourceNames{Count: "nvidia.com/gpu", Compute: "nvidia.com/gpucores"}
	catalog.NodeResourceNames = map[string]bool{"iluvatar.ai/MR-V100-vgpu": true}
	for _, tc := range []struct {
		name             string
		registered       bool
		resources        map[corev1.ResourceName]string
		acceleratorNodes int
		completeDevices  bool
	}{
		{"SMLU units alone confirm the node", false, map[corev1.ResourceName]string{core: "200", memory: "192"}, 1, false},
		{"typed plugin key absent from catalog", false, map[corev1.ResourceName]string{"cambricon.com/mlu370.smlu.vcore": "100"}, 1, false},
		{"zero SMLU units", false, map[corev1.ResourceName]string{core: "0", memory: "0"}, 0, true},
		{"RDMA beside registered GPU", true, map[corev1.ResourceName]string{rdma: "1000", "nvidia.com/gpucores": "100"}, 1, true},
		{"SMLU alongside registered GPU", true, map[corev1.ResourceName]string{core: "200"}, 1, false},
		{"configured accelerator without a registration decoder", false, map[corev1.ResourceName]string{"iluvatar.ai/MR-V100-vgpu": "1"}, 1, false},
		{"RDMA and other unrelated resources alone", false, map[corev1.ResourceName]string{rdma: "1000", "storage.example/volume": "5"}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := testNode()
			wantDevices := 1
			if !tc.registered {
				node.Annotations = nil
				node.Status.Capacity = corev1.ResourceList{}
				node.Status.Allocatable = nil
				wantDevices = 0
			}
			for name, value := range tc.resources {
				node.Status.Capacity[name] = resource.MustParse(value)
			}
			s := Build([]*corev1.Node{node}, catalog, podfacts.Inventory{}, map[string]labels.Selector{"NVIDIA": labels.Everything()}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "nodes", 1))
			groups := resourceview.GroupPhysicalDevices(s)
			if len(groups.AcceleratorNodes) != tc.acceleratorNodes || !s.NodeCoverage.Complete {
				t.Fatalf("accelerator nodes=%d, node coverage=%v", len(groups.AcceleratorNodes), s.NodeCoverage)
			}
			if len(groups.Groups) != wantDevices || len(s.Devices) != wantDevices {
				t.Fatalf("physical cards=%d, registration rows=%d, want %d", len(groups.Groups), len(s.Devices), wantDevices)
			}
			for _, c := range []*viewv1.Coverage{s.DeviceCoverage, s.Nodes[0].DeviceCoverage} {
				if c.Complete != tc.completeDevices || (c.Total != nil) != tc.completeDevices || c.Observed != int32(wantDevices) {
					t.Errorf("device coverage=%v, want observed=%d complete=%v", c, wantDevices, tc.completeDevices)
				}
			}
		})
	}
}

func TestMixedNodeScopesAllocationEvidenceToItsSource(t *testing.T) {
	node := testNode()
	const model = "Ascend910B3"
	node.Annotations["hami.io/node-register-"+model] = `[{"id":"npu-0","type":"Ascend910B3","count":8,"devmem":65536,"devcore":20,"health":true}]`
	node.Status.Capacity["huawei.com/npu"] = resource.MustParse("8")
	catalog := testCatalog()
	catalog.ResourceNames["NVIDIA"] = devicecatalog.DeviceResourceNames{Count: "nvidia.com/gpu", Compute: "private.example/gpu-cores"}
	catalog.Ascend = map[string]devicecatalog.AscendModel{model: {CommonWord: model, ResourceName: "huawei.com/npu", AICore: 20}}
	selectors := map[string]labels.Selector{"NVIDIA": labels.Everything(), "Ascend": labels.Everything()}

	for _, tc := range []struct {
		name                                  string
		request                               string
		removeRecords, badNVIDIA, opaqueClaim bool
		nvidiaComplete, ascendComplete        bool
	}{
		{"valid NVIDIA allocation", "nvidia.com/gpu", false, false, false, true, true},
		{"missing NVIDIA allocation", "private.example/gpu-cores", true, false, false, false, true},
		{"invalid NVIDIA allocation", "nvidia.com/gpu", false, true, false, false, true},
		{"opaque request", "other.example/unknown", true, false, false, false, false},
		{"opaque claim", "nvidia.com/gpu", false, false, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pods := allocationInventory(t, node)
			pods.Coverage = completeTestCoverage("pod", "shared_pod_informer", 1)
			f := pods.Entries[0].Facts
			f.Containers[0].Requests = []podfacts.Request{{Resource: tc.request, Positive: true}}
			if tc.opaqueClaim {
				f.Containers[0].Claims = []podfacts.ClaimReference{{LocalName: "resource"}}
			}
			for i := range f.Containers {
				if tc.removeRecords {
					f.Containers[i].Allocations = nil
				}
				if tc.badNVIDIA {
					f.Containers[i].Allocations[0].Source.State = resourceview.Unknown
				}
			}
			pods.Entries[0].Occupancies = podfacts.EffectiveOccupancies(f)
			got := Build([]*corev1.Node{node}, catalog, pods, selectors, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), completeTestCoverage("node", "shared_node_informer", 1))
			if len(got.Devices) != 2 {
				t.Fatalf("got %d devices", len(got.Devices))
			}
			for _, d := range got.Devices {
				want := tc.nvidiaComplete
				if d.Identity.Provider == "Ascend" {
					want = tc.ascendComplete
				}
				if d.AllocationCoverage.Complete != want {
					t.Errorf("%s: complete=%v, want %v (%v)", d.Identity.Provider, d.AllocationCoverage.Complete, want, d.AllocationCoverage.Sources)
				}
			}
		})
	}
}
