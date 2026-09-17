package data

import (
	"io"
	"math"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/provider"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func TestNodeInventoryReadsANodeWithoutStatus(t *testing.T) {
	repo := &nodeRepo{}

	node := repo.fetchNodeInfo(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-node", UID: "node-1"},
	})
	if node.Name != "pending-node" || node.OperatingSystem != "" || node.Architecture != "" {
		t.Fatalf("a node without status: %+v", node)
	}

	ready := repo.fetchNodeInfo(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{
			OperatingSystem: "linux",
			Architecture:    "amd64",
		}},
	})
	if ready.OperatingSystem != "Linux" || ready.Architecture != "AMD64" {
		t.Fatalf("a registered node: %+v", ready)
	}
}

type staticProvider struct{ devices []*util.DeviceInfo }

func (p staticProvider) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Everything(), nil
}
func (p staticProvider) GetProvider() string { return biz.NvidiaGPUDevice }
func (p staticProvider) FetchDevices(*corev1.Node) ([]*util.DeviceInfo, error) {
	return p.devices, nil
}

func TestNodeInventoryCarriesMIGProfiles(t *testing.T) {
	registered := []*util.DeviceInfo{{ID: "GPU-M", AliasId: "GPU-M", Mode: "mig", MigProfiles: []util.MigProfile{
		{Name: "1g.5gb", MemoryMB: 4864, Core: 14, SliceCount: 1, Placements: []util.MigPlacement{{Start: 6, Size: 1}}},
		{Name: "broken", SliceCount: math.MaxUint32},
		{Name: "far", SliceCount: 1, Placements: []util.MigPlacement{{Start: math.MaxUint32, Size: 1}}},
	}}}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-node", UID: "node-1"}}); err != nil {
		t.Fatal(err)
	}
	repo := &nodeRepo{
		nodeNotify: make(chan struct{}, 1),
		nodes:      map[k8stypes.UID]*biz.Node{},
		nodeLister: listerscorev1.NewNodeLister(indexer),
		log:        log.NewHelper(log.NewStdLogger(io.Discard)),
		providers:  []provider.Provider{staticProvider{devices: registered}},
	}
	go repo.updateLocalNodes()
	repo.nodeNotify <- struct{}{}

	deadline := time.Now().Add(5 * time.Second)
	for {
		repo.mutex.RLock()
		node := repo.nodes["node-1"]
		repo.mutex.RUnlock()
		if node != nil && len(node.Devices) == 1 {
			device := node.Devices[0]
			// Profiles outside HAMi's int32 range are dropped, not truncated.
			if device.Mode != "mig" || len(device.MigProfiles) != 1 {
				t.Fatalf("device = %+v", device)
			}
			if got := device.MigProfiles[0]; got.Name != "1g.5gb" || got.MemoryMB != 4864 || got.Core != 14 || got.SliceCount != 1 ||
				len(got.Placements) != 1 || got.Placements[0] != (biz.MigPlacement{Start: 6, Size: 1}) {
				t.Fatalf("profile = %+v", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the node inventory was not rebuilt")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
