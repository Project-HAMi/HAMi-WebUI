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
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/resourceview"
)

func testNode() *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1", Labels: map[string]string{"gpu": "on"}, Annotations: map[string]string{nvidia.RegisterAnnos: "GPU-a,10,16384,150,T4,0,true:"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("10")}, Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("0")}}}
}
func testCatalog() *devicecatalog.Snapshot {
	return &devicecatalog.Snapshot{State: devicecatalog.StateLoaded, ResourceNames: map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}}
}
func build(node *corev1.Node, catalog *devicecatalog.Snapshot, pods podfacts.Inventory) resourceview.Snapshot {
	return Build([]*corev1.Node{node}, catalog, pods, map[string]labels.Selector{"NVIDIA": labels.SelectorFromSet(labels.Set{"gpu": "on"})}, log.NewHelper(log.NewStdLogger(io.Discard)), time.Unix(100, 0), nil)
}
func TestNodeUnitsDoNotCreatePhysicalDevicesOrProviders(t *testing.T) {
	node := testNode()
	node.Annotations = nil
	node.Status.Capacity["amd.com/gpu"] = resource.MustParse("4")
	snapshot := build(node, testCatalog(), podfacts.Inventory{})
	if len(snapshot.Devices) != 0 || len(snapshot.Nodes) != 1 || len(snapshot.Nodes[0].Resources) != 2 {
		t.Fatal("invented devices or lost native resource units")
	}
	for _, pool := range snapshot.Nodes[0].Resources {
		if pool.Capacity.Unit != "resource_units" {
			t.Fatal("resource name proved provider or physical cards")
		}
	}
	if snapshot.Nodes[0].Ready != nil || snapshot.Nodes[0].CreatedAt != nil || snapshot.NodeCoverage.Complete || snapshot.NodeCoverage.Total != nil {
		t.Fatal("unknown metadata/coverage became a false value")
	}
	if snapshot.Nodes[0].Resources[1].Allocatable.Value == nil || snapshot.Nodes[0].Resources[1].Allocatable.Value.Sign() != 0 {
		t.Fatal("observed zero lost")
	}
	node.Labels["gpu"] = "off"
	node.Annotations = map[string]string{nvidia.RegisterAnnos: "GPU-a,10,16384,100,T4,0,true:"}
	if len(build(node, testCatalog(), podfacts.Inventory{}).Devices) != 0 {
		t.Fatal("unselected stale registration created inventory")
	}
}
func TestNVIDIARegistrationCapacityIsSeparateFromPhysicalAndTelemetry(t *testing.T) {
	node := testNode()
	snapshot := build(node, testCatalog(), podfacts.Inventory{})
	if len(snapshot.Devices) != 1 {
		t.Fatal("registration lost")
	}
	d := snapshot.Devices[0]
	if d.SlotsCapacity.Value.Value() != 10 || d.ComputeCapacityShare.Value.Cmp(resource.MustParse("1.5")) != 0 || d.MemoryCapacityBytes.Value.Value() != 16384*1024*1024 {
		t.Fatalf("wrong units or denominator: %+v", d)
	}
	if d.MemoryPhysicalBytes.Value != nil || d.MemoryUsedBytes.Value != nil || d.UtilizationRatio.Value != nil {
		t.Fatal("registration used as telemetry")
	}
	if d.Resource != "nvidia.com/gpu" || d.Configured == nil || !*d.Configured {
		t.Fatal("explicit configuration missing")
	}
	custom := testCatalog()
	custom.ResourceNames["NVIDIA"] = devicecatalog.DeviceResourceNames{Count: "private.example/gpu"}
	if row := build(node, custom, podfacts.Inventory{}).Devices[0]; row.Resource != "" || row.States["resource"].State != resourceview.Unknown {
		t.Fatal("config without corresponding Node resource proved actual mapping")
	}
	unknown := build(node, nil, podfacts.Inventory{}).Devices[0]
	if unknown.Configured != nil || unknown.Resource != "" || unknown.States["resource"].State != resourceview.Unknown {
		t.Fatal("missing config defaulted")
	}
	snapshot.Nodes[0].Resources[0].Capacity.Value.Add(resource.MustParse("1"))
	snapshot.Devices[0].SlotsCapacity.Value.Add(resource.MustParse("1"))
	if again := build(node, testCatalog(), podfacts.Inventory{}); again.Nodes[0].Resources[0].Capacity.Value.Value() != 10 || again.Devices[0].SlotsCapacity.Value.Value() != 10 {
		t.Fatal("snapshot returned aliases")
	}

	for _, raw := range []string{"GPU-a,bad,1,100,T4,0,true:", `[{"id":"GPU-a","type":"T4","devmem":10,"devcore":100,"health":true}]`} {
		node.Annotations[nvidia.RegisterAnnos] = raw
		got := build(node, testCatalog(), podfacts.Inventory{})
		if len(got.Devices) != 1 || got.Devices[0].SlotsCapacity.Value != nil || got.Devices[0].SlotsCapacity.State.State != resourceview.Unknown || got.Devices[0].MemoryCapacityBytes.Value == nil {
			t.Fatal("missing/invalid scalar lost identity or became zero")
		}
	}
	node.Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-a","type":"T4"}]`
	row := build(node, testCatalog(), podfacts.Inventory{}).Devices[0]
	if row.Healthy != nil || row.MemoryCapacityBytes.Value != nil || row.ComputeCapacityShare.Value != nil || row.States["health"].State != resourceview.Unknown {
		t.Fatal("omitted false/zero became known")
	}
	node.Annotations[nvidia.RegisterAnnos] = `[{"id":"GPU-a","type":"T4","count":0,"devmem":0,"devcore":0,"health":false}]`
	row = build(node, testCatalog(), podfacts.Inventory{}).Devices[0]
	if row.Healthy == nil || *row.Healthy || row.SlotsCapacity.Value == nil || row.SlotsCapacity.Value.Sign() != 0 {
		t.Fatal("explicit false/zero lost")
	}
	node.Annotations[nvidia.RegisterAnnos] = "GPU-a,10,1,100,T4,0,true:GPU-a,10,1,100,T4,0,true:"
	if got := build(node, testCatalog(), podfacts.Inventory{}); len(got.Devices) != 0 || got.Nodes[0].Sources[1].State != resourceview.Unknown {
		t.Fatal("duplicate identities resolved arbitrarily")
	}

}
func allocationInventory(t *testing.T, node *corev1.Node) podfacts.Inventory {
	t.Helper()
	always := corev1.ContainerRestartPolicyAlways
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "work", UID: "pod-uid", ResourceVersion: "1"}, Spec: corev1.PodSpec{NodeName: node.Name, InitContainers: []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "init"}}, Containers: []corev1.Container{{Name: "app"}}}}
	s := podfacts.AllocationSnapshot{Accounting: podfacts.AccountingPodPeak, PodUID: p.UID, ResourceVersion: p.ResourceVersion, Complete: true, Source: &viewv1.SourceState{Source: "hami_annotation:NVIDIA", State: resourceview.Known}}
	for name, value := range map[string]int64{"sidecar": 2, "init": 3, "app": 1} {
		s.Records = append(s.Records, podfacts.Allocation{Container: name, RecordID: name, Node: node.Name, NodeUID: node.UID, Resource: "NVIDIA", DeviceID: "GPU-a", Quantities: map[string]podfacts.Quantity{"memory": number(value*1024*1024, "bytes", "hami_annotation", "container"), "compute": number(value*10, "percent", "hami_annotation", "container"), "slots": number(1, "slots", "hami_annotation", "container")}})
	}
	facts, err := podfacts.Extract(p, podfacts.Evidence{Allocations: []podfacts.AllocationSnapshot{s}}, podfacts.Options{ResourceNames: []corev1.ResourceName{"NVIDIA"}})
	if err != nil {
		t.Fatal(err)
	}
	ledger := podfacts.NewLedger()
	ledger.Put(facts, func() bool { return true })
	return ledger.Snapshot()
}
func TestAllocationsRequireNodeUIDAndKeepContainerRecordsApartFromPeak(t *testing.T) {
	node := testNode()
	pods := allocationInventory(t, node)
	snapshot := build(node, testCatalog(), pods)
	d := snapshot.Devices[0]
	if len(d.EffectiveOccupancies) != 1 {
		t.Fatal("records/consumers/physical occupancy conflated")
	}
	if d.AllocationCoverage == nil || d.AllocationCoverage.Entity != "occupancy" || d.AllocationCoverage.Observed != 1 || d.AllocationCoverage.Complete || d.AllocationCoverage.Total != nil {
		t.Fatal("allocation coverage claimed global total or counted container members")
	}
	peak := d.EffectiveOccupancies[0]
	if peak.MemoryAllocatedBytes.Value.Value() != 5*1024*1024 || peak.ComputeAllocatedShare.Value.Cmp(resource.MustParse("0.5")) != 0 {
		t.Fatal("physical occupancy sums mutually exclusive init records")
	}
	empty := build(node, testCatalog(), podfacts.Inventory{}).Devices[0].AllocationCoverage
	if empty.Observed != 0 || empty.Complete || empty.Total != nil {
		t.Fatal("observed empty allocation became known whole-inventory zero")
	}
	node.UID = "replacement-node"
	if d := build(node, testCatalog(), pods).Devices[0]; len(d.EffectiveOccupancies) != 0 {
		t.Fatal("old Pod allocation inherited replacement Node identity")
	}
	node.UID = "node-uid"
	pods.Entries[0].Facts.Terminal = true
	if len(build(node, testCatalog(), pods).Devices[0].EffectiveOccupancies) != 0 {
		t.Fatal("terminal Pod counted as live allocation")
	}
}
