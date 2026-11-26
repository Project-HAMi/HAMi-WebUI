package providerfacts

import (
	"io"
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
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/hygon"
	"vgpu/internal/provider/metax"
	"vgpu/internal/provider/registry"
	"vgpu/internal/resourceview"
)

func vendorBuild(n *corev1.Node, c *devicecatalog.Snapshot, p podfacts.Inventory, provider string) resourceview.Snapshot {
	return Build([]*corev1.Node{n}, c, p, map[string]labels.Selector{provider: labels.Everything()}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), nil, registry.DefaultInventoryDialects())
}
func TestRegisteredVendorIdentityUnitsAndAllocationAssociation(t *testing.T) {
	for _, tc := range []struct{ provider, key, raw, id, allocationID, source, configKey, countResource string }{
		{"HCU", hygon.HCURegisterAnnos, "HCU-serial,8,32768,150,HCU-K100,0,true:", "HCU-serial", "HCU-serial", "HCU", "DCU", "hygon.com/hcunum"},
		{"DCU", hygon.RegisterAnnos, "DCU-0,8,32768,150,DCU-Z100,0,true:", "DCU-0", "node-a-dcu-0", "DCU", "DCU", "hygon.com/dcunum"},
		{"Metax", metax.MetaxSDeviceAnno, `[{"uuid":"metax-uuid","model":"C500","totalDevCount":8,"totalVRam":32768,"totalCompute":150,"healthy":true}]`, "metax-uuid", "metax-uuid", "Metax-SGPU", "Metax-SGPU", "metax-tech.com/sgpu"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			n := testNode()
			n.Annotations = map[string]string{tc.key: tc.raw}
			n.Status.Capacity = corev1.ResourceList{corev1.ResourceName(tc.countResource): resource.MustParse("8")}
			c := testCatalog()
			c.ResourceNames = map[string]devicecatalog.DeviceResourceNames{tc.configKey: {Count: tc.countResource}}
			p := allocationInventory(t, n)
			for ci := range p.Entries[0].Facts.Containers {
				for si := range p.Entries[0].Facts.Containers[ci].Allocations {
					set := &p.Entries[0].Facts.Containers[ci].Allocations[si]
					set.Source.Source = "hami_annotation:" + tc.source
					for ri := range set.Records {
						r := &set.Records[ri]
						r.DeviceID = tc.allocationID
						r.Resource = tc.source
						if tc.provider == "Metax" {
							r.DeviceID = ""
							r.RawDeviceID = tc.allocationID
						}
					}
				}
			}
			p.Entries[0].Occupancies = podfacts.EffectiveOccupancies(p.Entries[0].Facts)
			s := vendorBuild(n, c, p, tc.provider)
			if len(s.Devices) != 1 {
				t.Fatalf("devices=%d", len(s.Devices))
			}
			d := s.Devices[0]
			if d.Identity.DeviceID != tc.id || d.Kind != "physical" || d.ComputeCapacityShare.Value.Cmp(resource.MustParse("1.5")) != 0 || d.SlotsCapacity.Value.Value() != 8 || d.MemoryCapacityBytes.Value.Value() != 32768*1024*1024 {
				t.Fatalf("identity/unit: %+v", d)
			}
			if d.Resource != tc.countResource || d.Mode != "" || d.States["mode"].State != resourceview.Unknown {
				t.Fatal("resource lost or mode invented")
			}
			if len(d.EffectiveOccupancies) != 1 || d.EffectiveOccupancies[0].MemoryAllocatedBytes.Value.Value() != 5*1024*1024 {
				t.Fatalf("association/peak: %+v", d.EffectiveOccupancies)
			}
			if tc.provider == "Metax" && p.Entries[0].Facts.Containers[0].Allocations[0].Records[0].DeviceID != "" {
				t.Fatal("inventory association mutated ledger")
			}
			if len(s.VerifiedBindings) != 3 {
				t.Fatalf("expected 3 VerifiedDeviceBindings, got %+v", s.VerifiedBindings)
			}
			for _, b := range s.VerifiedBindings {
				if b.PodUID != p.Entries[0].Facts.Metadata.UID || b.Source != "hami_annotation:"+tc.source ||
					b.Device.NodeUID != n.UID || b.Device.DeviceID != tc.id ||
					(tc.provider == "Metax" && b.RawDeviceID != tc.allocationID) {
					t.Fatalf("unexpected VerifiedDeviceBinding: %+v", b)
				}
			}
			n.UID = "replacement"
			replaced := vendorBuild(n, c, p, tc.provider)
			if len(replaced.Devices[0].EffectiveOccupancies) != 0 || len(replaced.VerifiedBindings) != 0 {
				t.Fatal("UID replacement inherited occupancy or verified binding")
			}
		})
	}
}
func TestAscendPhysicalIdentityAndAIcoreUnitsAreIndependent(t *testing.T) {
	for _, word := range []string{"Ascend310P", "Ascend910A", "Ascend910B2", "Ascend910B3", "Ascend910B4", "Ascend910B4-1", "Ascend910C"} {
		t.Run(word, func(t *testing.T) {
			n := testNode()
			n.Annotations = map[string]string{"hami.io/node-register-" + word: `[{"id":"npu-die","type":"` + word + `","count":8,"devmem":65536,"devcore":20,"health":true}]`, "hami-vnpu-core": "false"}
			c := testCatalog()
			c.Ascend = map[string]devicecatalog.AscendModel{word: {CommonWord: word, AICore: 20, ResourceName: "huawei.com/npu"}}
			n.Status.Capacity = corev1.ResourceList{"huawei.com/npu": resource.MustParse("8")}
			s := vendorBuild(n, c, podfacts.Inventory{}, "Ascend")
			if len(s.Devices) != 1 {
				t.Fatal("missing Ascend identity")
			}
			d := s.Devices[0]
			if d.ComputeCapacityShare.Value == nil || d.ComputeCapacityShare.Value.Cmp(resource.MustParse("1")) != 0 || d.Mode != ascend.ModeTemplate {
				t.Fatalf("AIcore confused with percentage: %+v", d.ComputeCapacityShare)
			}
			if word == "Ascend910C" {
				if d.Kind != "unknown" || d.States["kind"].State != resourceview.Unknown {
					t.Fatal("NPU modules became physical cards")
				}
			} else if d.Kind != "physical" {
				t.Fatal("proven physical card denominator lost")
			}
			delete(n.Annotations, "hami-vnpu-core")
			d = vendorBuild(n, c, podfacts.Inventory{}, "Ascend").Devices[0]
			if d.ComputeCapacityShare.Value != nil || d.Mode != "" {
				t.Fatal("configuration default invented registration units/mode")
			}
			n.Annotations["hami-vnpu-core"] = "false"
			c.Ascend[word] = devicecatalog.AscendModel{CommonWord: word, AICore: 3}
			if vendorBuild(n, c, podfacts.Inventory{}, "Ascend").Devices[0].ComputeCapacityShare.Value != nil {
				t.Fatal("inexact fraction rounded into known value")
			}
		})
	}
}
func TestVendorMissingFieldsPreserveIdentityAndUnknown(t *testing.T) {
	n := testNode()
	n.Annotations = map[string]string{metax.MetaxSDeviceAnno: `[{"uuid":"uuid","model":"C500"}]`}
	d := vendorBuild(n, testCatalog(), podfacts.Inventory{}, "Metax").Devices[0]
	if d.SlotsCapacity.Value != nil || d.MemoryCapacityBytes.Value != nil || d.ComputeCapacityShare.Value != nil || d.Healthy != nil {
		t.Fatal("omitted fields became known zero/false")
	}
	n.Annotations = map[string]string{"hami.io/node-register-Ascend910B3": `[{"id":"same","type":"Ascend910B3"}]`, "hami.io/node-register-Ascend910B4": `[{"id":"same","type":"Ascend910B4"}]`}
	s := vendorBuild(n, testCatalog(), podfacts.Inventory{}, "Ascend")
	if len(s.Devices) != 0 || s.Nodes[0].Sources[1].State != resourceview.Unknown {
		t.Fatal("ambiguous model identities selected arbitrarily")
	}
}

func TestDCUOpaqueRegistrationIDDoesNotRequireTelemetryConversion(t *testing.T) {
	n := testNode()
	n.Annotations = map[string]string{hygon.RegisterAnnos: "DCU-opaque-serial,4,32768,100,Z100,0,true,0,hami-core:"}
	s := vendorBuild(n, testCatalog(), podfacts.Inventory{}, "DCU")
	if len(s.Devices) != 1 || s.Devices[0].Identity.DeviceID != "DCU-opaque-serial" || s.Devices[0].UUID != nil {
		t.Fatal("opaque registration lost or became fake telemetry UUID")
	}
	c := testCatalog()
	c.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"DCU": {Count: "hygon.com/dcunum"}}
	n.Status.Capacity = corev1.ResourceList{"hygon.com/dcunum": resource.MustParse("4")}
	p := allocationInventory(t, n)
	p.Entries[0].Facts.Containers[0].Allocations[0].Source.Source = "hami_annotation:DCU"
	p.Entries[0].Facts.Containers[0].Allocations[0].Records[0].DeviceID = "node-a-dcu-opaque-serial"
	p.Entries[0].Occupancies = podfacts.EffectiveOccupancies(p.Entries[0].Facts)
	mismatched := vendorBuild(n, c, p, "DCU")
	if len(mismatched.VerifiedBindings) != 0 || mismatched.Devices[0].AllocationCoverage.Complete {
		t.Fatalf("fabricated opaque DCU alias matched or claimed complete: bindings=%+v cov=%+v", mismatched.VerifiedBindings, mismatched.Devices[0].AllocationCoverage)
	}
}

func TestAscendVerifiedBindingMatchesCommonWordAndModel(t *testing.T) {
	n := testNode()
	n.Annotations = map[string]string{"hami.io/node-register-Ascend910B3": `[{"id":"npu-die-0","type":"Ascend910B3","count":4,"devmem":65536,"devcore":20,"health":true}]`, "hami-vnpu-core": "false"}
	n.Status.Capacity = corev1.ResourceList{"huawei.com/Ascend910B": resource.MustParse("4")}
	c := testCatalog()
	c.Ascend = map[string]devicecatalog.AscendModel{"Ascend910B3": {CommonWord: "Ascend910B3", AICore: 20, ResourceName: "huawei.com/Ascend910B"}}
	p := allocationInventory(t, n)
	p.Entries[0].Facts.Containers[0].Allocations[0].Source.Source = "hami_annotation:Ascend910B3"
	p.Entries[0].Facts.Containers[0].Allocations[0].Records[0].DeviceID = "npu-die-0"
	p.Entries[0].Facts.Containers[0].Allocations[0].Records[0].RawDeviceID = "npu-die-0"
	p.Entries[0].Occupancies = podfacts.EffectiveOccupancies(p.Entries[0].Facts)

	s := vendorBuild(n, c, p, "Ascend")
	if len(s.VerifiedBindings) != 1 || s.VerifiedBindings[0].Device.Provider != "Ascend" || s.VerifiedBindings[0].Device.DeviceID != "npu-die-0" {
		t.Fatalf("expected Ascend VerifiedDeviceBinding, got %+v", s.VerifiedBindings)
	}
}

func TestUnlimitedAllocationRemainsUnknownAtDeviceBoundary(t *testing.T) {
	n := testNode()
	p := allocationInventory(t, n)
	r := &p.Entries[0].Facts.Containers[0].Allocations[0].Records[0]
	unlimited := true
	r.ComputeUnlimited = &unlimited
	r.States = map[string]*viewv1.FieldState{"computeUnlimited": known("hami_annotation_and_node_registration", "container")}
	r.Quantities["compute"] = absent("percent", "hami_annotation_and_node_registration", "container", "unverified")
	p.Entries[0].Occupancies = podfacts.EffectiveOccupancies(p.Entries[0].Facts)
	d := build(n, testCatalog(), p).Devices[0]
	if len(d.EffectiveOccupancies) != 1 || d.EffectiveOccupancies[0].ComputeAllocatedShare.Value != nil {
		t.Fatal("unlimited became zero device occupancy")
	}
}

// MThreads' scheduler resource proves a node has accelerator units. Neither
// advertised slots nor a configured resource name identify physical cards.
func TestMThreadsResourceOnlyNode(t *testing.T) {
	const customName = "custom.example/gpu"
	catalog := testCatalog()
	catalog.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"Mthreads": {Count: customName}}
	for _, name := range []string{"mthreads.com/vgpu", "mthreads.com/gpu", "mthreads.com/sgpu-core", "mthreads.com/sgpu-memory", customName} {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "a"}}
		node.Status.Capacity = corev1.ResourceList{corev1.ResourceName(name): resource.MustParse("16")}
		f := Build([]*corev1.Node{node}, catalog, podfacts.Inventory{}, nil, nil, time.Now(), nil, registry.DefaultInventoryDialects())
		if len(f.Devices) != 0 || len(f.Nodes) != 1 || len(f.Nodes[0].Resources) != 1 || f.Nodes[0].Resources[0].Resource != name {
			t.Fatalf("advertised %s: got %d physical records, pools=%v", name, len(f.Devices), f.Nodes[0].Resources)
		}
	}
}

func TestHygonHCUAndDCUOverlapGroupsViaPhysicalAssociation(t *testing.T) {
	node := testNode()
	node.Annotations = map[string]string{
		hygon.HCURegisterAnnos: "HCU-board-A,4,65536,100,HCU-K100,0,true:",
		hygon.RegisterAnnos:    "DCU-board-A,4,32768,100,DCU-Z100,0,true:DCU-0,4,32768,100,DCU-Z100,0,true:",
	}
	selectors := map[string]labels.Selector{"HCU": labels.Everything(), "DCU": labels.Everything()}
	snap := Build([]*corev1.Node{node}, testCatalog(), podfacts.Inventory{}, selectors, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), nil, registry.DefaultInventoryDialects())
	inv := resourceview.GroupPhysicalDevices(snap)
	if !inv.UnresolvedOnNode[node.UID] {
		t.Fatal("expected slot-only DCU-0 to be flagged unresolved while HCU is active")
	}
	if len(inv.Groups) != 1 || len(inv.Groups[0].Members) != 2 || inv.Groups[0].Members[0].Identity.Provider != "HCU" {
		t.Fatalf("expected 1 grouped HCU+DCU board preferring HCU first, got %+v", inv.Groups)
	}
	if model, _ := inv.Groups[0].Model(); model != "HCU-K100" {
		t.Fatalf("model = %q, want HCU-K100", model)
	}
}
