package mthreads

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func nodeWithCapacity(name string, labels map[string]string, capacity corev1.ResourceList) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Status:     corev1.NodeStatus{Capacity: capacity},
	}
}

func TestParseCardIDList(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []int64
	}{
		{name: "dash separated", raw: "0-2-3", want: []int64{0, 2, 3}},
		{name: "comma separated", raw: "1,2", want: []int64{1, 2}},
		{name: "deduplicates keeping first seen order", raw: "3-1-3-2", want: []int64{3, 1, 2}},
		{name: "drops unparsable and negative", raw: "0-x--2", want: []int64{0, 2}},
		{name: "empty", raw: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCardIDList(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseCardIDList(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ParseCardIDList(%q) = %v, want %v", tt.raw, got, tt.want)
				}
			}
		})
	}
}

// A whole-card-only node publishes no sGPU pool. The inventory must still
// enumerate its cards, otherwise FetchDevices reports nothing at all.
func TestNodeCardInventoryWholeCardOnlyNode(t *testing.T) {
	node := nodeWithCapacity("n", nil, corev1.ResourceList{
		corev1.ResourceName(NodeWholeGPUResource): resource.MustParse("4"),
	})

	sliced, whole, perCard := NodeCardInventory(node)
	if len(sliced) != 0 {
		t.Fatalf("sliced ids = %v, want none", sliced)
	}
	if len(whole) != 4 {
		t.Fatalf("whole ids = %v, want 4 cards", whole)
	}
	for i, id := range whole {
		if id != int64(i) {
			t.Fatalf("whole ids = %v, want [0 1 2 3]", whole)
		}
	}
	if perCard != 0 {
		t.Fatalf("per card memory = %d, want 0 without telemetry", perCard)
	}
}

// Whole-card ids must not collide with sliced-card ids: the telemetry layer
// uses DeviceInfo.Index as the DCGM gpu selector, so each physical ordinal
// has to appear exactly once.
func TestNodeCardInventoryKeepsWholeIdsDisjointFromSliced(t *testing.T) {
	node := nodeWithCapacity("n", map[string]string{
		SGPUCoresLabel: "0-2-3",
		GPUCountLabel:  "8",
	}, corev1.ResourceList{
		corev1.ResourceName(NodeSGPUCoresResource):  resource.MustParse("48"),
		corev1.ResourceName(NodeSGPUMemoryResource): resource.MustParse("1024"),
	})

	sliced, whole, perCard := NodeCardInventory(node)
	if len(sliced) != 3 || len(whole) != 5 {
		t.Fatalf("sliced=%v whole=%v, want 3 sliced + 5 whole", sliced, whole)
	}
	seen := map[int64]string{}
	for _, id := range sliced {
		seen[id] = "sliced"
	}
	for _, id := range whole {
		if prev, dup := seen[id]; dup {
			t.Fatalf("card %d reported as both %s and whole", id, prev)
		}
		seen[id] = "whole"
	}
	if len(seen) != 8 {
		t.Fatalf("card ordinals = %v, want 8 distinct", seen)
	}
	// 1024 units x 512MiB spread over 3 sliced cards.
	if want := int64(1024 * 512 / 3); perCard != want {
		t.Fatalf("per card memory = %d, want %d", perCard, want)
	}
}

// Without the sgpu.cores label the sliced pool is derived from the core
// capacity (16 units per card).
func TestNodeCardInventoryDerivesSlicedIdsFromCapacity(t *testing.T) {
	node := nodeWithCapacity("n", nil, corev1.ResourceList{
		corev1.ResourceName(NodeSGPUCoresResource):  resource.MustParse("32"),
		corev1.ResourceName(NodeSGPUMemoryResource): resource.MustParse("256"),
	})

	sliced, _, perCard := NodeCardInventory(node)
	if len(sliced) != 2 || sliced[0] != 0 || sliced[1] != 1 {
		t.Fatalf("sliced ids = %v, want [0 1]", sliced)
	}
	if want := int64(256 * 512 / 2); perCard != want {
		t.Fatalf("per card memory = %d, want %d", perCard, want)
	}
}

// Whole-card memory must not report 0 on a node without an sGPU pool: the
// provider and the pod ledger both derive per-card memory from here.
func TestPerCardMemoryMiBFallsBackToVendorLabels(t *testing.T) {
	node := nodeWithCapacity("n", map[string]string{
		GPUCountLabel:         "2",
		GPUMemoryLabelName(0): "85899345920", // 80GiB
		GPUMemoryLabelName(1): "85899345920",
	}, corev1.ResourceList{
		corev1.ResourceName(NodeWholeGPUResource): resource.MustParse("2"),
	})

	if got, want := PerCardMemoryMiB(node), int32(81920); got != want {
		t.Fatalf("PerCardMemoryMiB = %d MiB, want %d MiB", got, want)
	}

	_, _, perCard := NodeCardInventory(node)
	if perCard != 81920 {
		t.Fatalf("NodeCardInventory per card memory = %d, want 81920", perCard)
	}
}

func TestPerCardMemoryMiBPrefersSgpuPool(t *testing.T) {
	node := nodeWithCapacity("n", map[string]string{
		GPUCountLabel:         "1",
		GPUMemoryLabelName(0): "1073741824", // 1GiB, must lose to the sGPU pool
	}, corev1.ResourceList{
		corev1.ResourceName(NodeSGPUCoresResource):  resource.MustParse("16"),
		corev1.ResourceName(NodeSGPUMemoryResource): resource.MustParse("160"),
	})

	if got, want := PerCardMemoryMiB(node), int32(160*512); got != want {
		t.Fatalf("PerCardMemoryMiB = %d MiB, want %d MiB from the sGPU pool", got, want)
	}
}

func TestPerCardMemoryMiBWithoutAnySource(t *testing.T) {
	node := nodeWithCapacity("n", nil, corev1.ResourceList{})
	if got := PerCardMemoryMiB(node); got != 0 {
		t.Fatalf("PerCardMemoryMiB = %d, want 0 with no telemetry", got)
	}
}

// FetchDevices must report one device per physical card, carrying the real
// physical ordinal so DCGM series map 1:1.
func TestFetchDevicesCoversSlicedAndWholeCards(t *testing.T) {
	node := nodeWithCapacity("node-a", map[string]string{
		SGPUCoresLabel:        "0",
		GPUCountLabel:         "3",
		GPUMemoryLabelName(0): "85899345920",
		GPUMemoryLabelName(1): "85899345920",
		GPUMemoryLabelName(2): "85899345920",
	}, corev1.ResourceList{
		corev1.ResourceName(NodeSGPUCoresResource):  resource.MustParse("16"),
		corev1.ResourceName(NodeSGPUMemoryResource): resource.MustParse("160"),
	})

	devices, err := NewMthreads(nil, nil, "").FetchDevices(node)
	if err != nil {
		t.Fatalf("FetchDevices: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("devices = %d, want 3", len(devices))
	}

	indexes := map[uint]string{}
	for _, d := range devices {
		if prev, dup := indexes[d.Index]; dup {
			t.Fatalf("index %d used by both %s and %s", d.Index, prev, d.ID)
		}
		indexes[d.Index] = d.ID
	}
	if _, ok := indexes[0]; !ok {
		t.Fatalf("sliced card index 0 missing from %v", indexes)
	}
	for _, want := range []uint{1, 2} {
		if _, ok := indexes[want]; !ok {
			t.Fatalf("whole-card index %d missing from %v", want, indexes)
		}
	}
	if devices[0].Mode != "sgpu" || devices[0].Type != MthreadsDevice {
		t.Fatalf("sliced device = %+v, want sgpu mode", *devices[0])
	}
	if devices[1].Mode != "full" || devices[1].Type != MthreadsWholeGPUType || devices[1].Count != 1 {
		t.Fatalf("whole device = %+v, want full mode with count 1", *devices[1])
	}
}

func TestFetchDevicesWholeCardOnlyNode(t *testing.T) {
	node := nodeWithCapacity("node-b", map[string]string{
		GPUMemoryLabelName(0): "85899345920",
	}, corev1.ResourceList{
		corev1.ResourceName(NodeWholeGPUResource): resource.MustParse("1"),
	})

	devices, err := NewMthreads(nil, nil, "").FetchDevices(node)
	if err != nil {
		t.Fatalf("FetchDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %d, want 1 whole card", len(devices))
	}
	if devices[0].Mode != "full" || devices[0].Devmem != 81920 {
		t.Fatalf("whole device = %+v, want full mode with 81920 MiB", *devices[0])
	}
}

func TestFetchDevicesWithoutAnyCard(t *testing.T) {
	devices, err := NewMthreads(nil, nil, "").FetchDevices(nodeWithCapacity("node-c", nil, corev1.ResourceList{}))
	if err != nil {
		t.Fatalf("FetchDevices: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("devices = %d, want none", len(devices))
	}
}