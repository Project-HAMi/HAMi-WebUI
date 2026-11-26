package data

import (
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

func TestClusterCaptureIndexesAndConcurrentPublish(t *testing.T) {
	store := NewCaptureStore()
	if store.Load() != nil {
		t.Fatal("initial capture must be nil")
	}

	dev := resourceview.Device{
		Identity: resourceview.DeviceIdentity{
			Provider: "NVIDIA",
			Node:     "node-1",
			NodeUID:  types.UID("node-uid-1"),
			DeviceID: "GPU-1",
		},
		Vendor: "NVIDIA",
		Model:  "A100",
	}
	opaqueID, err := resourceview.DeviceID("NVIDIA", "node-1", "GPU-1")
	if err != nil {
		t.Fatal(err)
	}

	ctrKey := podfacts.ContainerKey{PodUID: types.UID("pod-uid-1"), Name: "trainer"}
	snap := resourceview.Snapshot{
		At:      time.Unix(1700000000, 0).UTC(),
		Nodes:   []resourceview.Node{{UID: types.UID("node-uid-1"), Name: "node-1"}},
		Devices: []resourceview.Device{dev},
	}
	inv := podfacts.Inventory{
		Entries: []podfacts.Entry{{
			Facts: &podfacts.Facts{
				Metadata: metav1.ObjectMeta{
					Namespace:       "default",
					Name:            "train-0",
					UID:             types.UID("pod-uid-1"),
					ResourceVersion: "101",
				},
				Containers: []podfacts.Container{{Key: ctrKey, Kind: podfacts.Regular}},
			},
		}},
	}

	c1 := store.Publish("cat-1", snap, inv, time.Unix(1700000001, 0).UTC())
	if c1.Generation != 1 || c1.CatalogRV != "cat-1" {
		t.Fatalf("unexpected generation/catalog: %+v", c1)
	}
	if got, ok := c1.LookupDevice(opaqueID); !ok || got.Model != "A100" {
		t.Fatalf("LookupDevice failed: ok=%v got=%+v", ok, got)
	}
	if got, ok := c1.LookupNode(types.UID("node-uid-1")); !ok || got.Name != "node-1" {
		t.Fatalf("LookupNode failed: ok=%v got=%+v", ok, got)
	}
	if got, ok := c1.LookupNodeByName("node-1"); !ok || got.UID != types.UID("node-uid-1") {
		t.Fatalf("LookupNodeByName failed: ok=%v got=%+v", ok, got)
	}
	if got, ok := c1.LookupPod(types.UID("pod-uid-1")); !ok || got.Facts.Metadata.Name != "train-0" {
		t.Fatalf("LookupPod failed: ok=%v got=%+v", ok, got)
	}
	if got, ok := c1.LookupContainer(ctrKey); !ok || got.Kind != podfacts.Regular {
		t.Fatalf("LookupContainer failed: ok=%v got=%+v", ok, got)
	}
	if _, ok := c1.LookupDevice(""); ok {
		t.Fatal("empty device lookup must return false")
	}

	// Concurrent readers + publishers remain race-free and strictly monotonic.
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				store.Publish("cat-next", snap, inv, time.Now().UTC())
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			var lastGen uint64
			for i := 0; i < 100; i++ {
				cur := store.Load()
				if cur == nil {
					continue
				}
				if cur.Generation < lastGen {
					t.Errorf("generation went backward: %d < %d", cur.Generation, lastGen)
				}
				lastGen = cur.Generation
				_, _ = cur.LookupDevice(opaqueID)
				_, _ = cur.LookupContainer(ctrKey)
			}
		}()
	}
	wg.Wait()
	if got := store.Load(); got.Generation != 201 {
		t.Fatalf("expected final generation 201, got %d", got.Generation)
	}
}

func TestClusterCaptureDetachesCallerSlices(t *testing.T) {
	store := NewCaptureStore()
	ctrKey := podfacts.ContainerKey{PodUID: types.UID("pod-a"), Name: "ctr-a"}
	dev := resourceview.Device{
		Identity: resourceview.DeviceIdentity{
			Provider: "NVIDIA",
			Node:     "node-a",
			NodeUID:  types.UID("uid-a"),
			DeviceID: "GPU-A",
		},
		Model: "A100",
	}
	opaqueID, err := resourceview.DeviceID("NVIDIA", "node-a", "GPU-A")
	if err != nil {
		t.Fatal(err)
	}
	snap := resourceview.Snapshot{
		Nodes:   []resourceview.Node{{UID: types.UID("uid-a"), Name: "node-a"}},
		Devices: []resourceview.Device{dev},
	}
	inv := podfacts.Inventory{
		Entries: []podfacts.Entry{{
			Facts: &podfacts.Facts{
				Metadata: metav1.ObjectMeta{UID: types.UID("pod-a"), Name: "pod-a"},
				Containers: []podfacts.Container{{
					Key:  ctrKey,
					Kind: podfacts.Regular,
				}},
			},
		}},
	}

	c := store.Publish("cat-1", snap, inv, time.Unix(1700000002, 0).UTC())
	snap.Nodes[0].Name = "node-b"
	snap.Devices[0].Model = "H100"
	inv.Entries[0].Facts.Containers[0].Kind = podfacts.Init

	if got, ok := c.LookupNodeByName("node-a"); !ok || got.Name != "node-a" {
		t.Fatalf("expected detached node-a, got ok=%v node=%+v", ok, got)
	}
	if got, ok := c.LookupDevice(opaqueID); !ok || got.Model != "A100" {
		t.Fatalf("expected detached device model A100, got ok=%v dev=%+v", ok, got)
	}
	if got, ok := c.LookupContainer(ctrKey); !ok || got.Kind != podfacts.Regular {
		t.Fatalf("expected detached container kind regular, got ok=%v ctr=%+v", ok, got)
	}
}
