package data

import (
	"io"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func TestNodeProvidersUseVendorSelectorsAndIsolateInvalidConfiguration(t *testing.T) {
	for _, explicitHCU := range []bool{false, true} {
		selectors := map[string]string{
			"Ascend":       "accelerator in (npu, mixed)",
			"MLU":          "broken (",
			"FutureVendor": "broken (",
		}
		if explicitHCU {
			selectors["HCU"] = ""
		}
		providers := NewNodeProviders(&Data{}, selectors, log.NewStdLogger(io.Discard), nil)
		got := map[string]labels.Selector{}
		for _, p := range providers {
			got[p.GetProvider()] = p.selector
		}
		if len(got) != 5 || got["MLU"] != nil || got["FutureVendor"] != nil {
			t.Fatalf("selected providers: %v", got)
		}
		for _, name := range []string{"NVIDIA", "DCU", "Metax"} {
			if got[name] == nil || !got[name].Matches(labels.Set{}) {
				t.Fatalf("omitted %s selector should accept unlabeled nodes", name)
			}
		}
		if got["Ascend"] == nil || got["Ascend"].Matches(labels.Set{}) || !got["Ascend"].Matches(labels.Set{"accelerator": "mixed"}) {
			t.Fatalf("Ascend selector lost its configured expression: %v", got["Ascend"])
		}
		if got["HCU"] == nil || got["HCU"].Matches(labels.Set{}) || !got["HCU"].Matches(labels.Set{"hcu": "on"}) {
			t.Fatalf("empty or omitted HCU selector should match hcu=on, got %v", got["HCU"])
		}
	}
}

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
	stopCh := make(chan struct{})
	defer close(stopCh)
	repo := &nodeRepo{
		data:       &Data{stopCh: stopCh},
		nodeNotify: make(chan struct{}, 1),
		nodes:      map[k8stypes.UID]*biz.Node{},
		nodeLister: listerscorev1.NewNodeLister(indexer),
		log:        log.NewHelper(log.NewStdLogger(io.Discard)),
		providers:  []nodeProvider{{Provider: staticProvider{devices: registered}, selector: labels.Everything()}},
	}
	go repo.updateLocalNodes()
	repo.notifyNodeChange()

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

type blockingNodeProvider struct {
	mu      sync.Mutex
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingNodeProvider) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Everything(), nil
}
func (p *blockingNodeProvider) GetProvider() string { return biz.NvidiaGPUDevice }
func (p *blockingNodeProvider) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	call := p.calls.Add(1)
	if call == 1 && p.started != nil {
		p.once.Do(func() { close(p.started) })
		<-p.release
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return []*util.DeviceInfo{{ID: "GPU-" + node.Annotations["rev"], AliasId: "GPU-" + node.Annotations["rev"]}}, nil
}

func TestNodeInformerCallbacksNeverBlockAndCoalesceBurst(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	baseNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "gpu-node",
			UID:         "node-1",
			Annotations: map[string]string{"rev": "0"},
		},
	}
	if err := indexer.Add(baseNode); err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	defer close(stopCh)
	provider := &blockingNodeProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	repo := &nodeRepo{
		data:       &Data{stopCh: stopCh},
		nodeNotify: make(chan struct{}, 1),
		nodes:      map[k8stypes.UID]*biz.Node{},
		nodeLister: listerscorev1.NewNodeLister(indexer),
		log:        log.NewHelper(log.NewStdLogger(io.Discard)),
		providers:  []nodeProvider{{Provider: provider, selector: labels.Everything()}},
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		repo.updateLocalNodes()
	}()

	// Trigger the first refresh and wait until FetchDevices is blocked inside the worker.
	repo.notifyNodeChange()
	<-provider.started

	// Update lister to revision 100 and flood informer callbacks while worker is blocked.
	latest := baseNode.DeepCopy()
	latest.Annotations["rev"] = "100"
	if err := indexer.Update(latest); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		prev := baseNode
		for i := 1; i <= 100; i++ {
			next := baseNode.DeepCopy()
			next.Annotations["rev"] = string(rune('a' + (i % 26)))
			repo.onAddNode(next)
			repo.onUpdateNode(prev, next)
			repo.onDeletedNode(prev)
			prev = next
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(provider.release)
		t.Fatal("informer callbacks blocked on nodeNotify while refresh was in flight")
	}

	close(provider.release)

	deadline := time.Now().Add(3 * time.Second)
	for {
		repo.mutex.RLock()
		node := repo.nodes["node-1"]
		repo.mutex.RUnlock()
		if node != nil && len(node.Devices) == 1 && node.Devices[0].Id == "GPU-100" && provider.calls.Load() == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected coalesced refresh to observe GPU-100 in 2 calls, calls=%d node=%+v", provider.calls.Load(), node)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Close stopCh via defer so worker exits cleanly.
}

func TestNodeUpdateFiltersHeartbeatOnlyChurn(t *testing.T) {
	now := metav1.Now()
	later := metav1.NewTime(now.Add(10 * time.Second))
	base := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "worker-1",
			UID:               "uid-1",
			ResourceVersion:   "10",
			CreationTimestamp: now,
			Labels:            map[string]string{"gpu": "on"},
			Annotations:       map[string]string{"hami.io/node-nvidia-register": "[]"},
		},
		Spec: corev1.NodeSpec{Unschedulable: false},
		Status: corev1.NodeStatus{
			NodeInfo:    corev1.NodeSystemInfo{OSImage: "Ubuntu"},
			Addresses:   []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: now}},
			Capacity:    corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")},
			Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")},
		},
	}

	heartbeat := base.DeepCopy()
	heartbeat.ResourceVersion = "11"
	heartbeat.Status.Conditions[0].LastHeartbeatTime = later
	if nodeUpdateAffectsInventory(base, heartbeat) {
		t.Fatal("pure kubelet heartbeat update should be filtered")
	}

	mutateCases := []struct {
		name   string
		mutate func(*corev1.Node)
	}{
		{"label", func(n *corev1.Node) { n.Labels["zone"] = "az1" }},
		{"annotation", func(n *corev1.Node) { n.Annotations["hami.io/node-nvidia-register"] = "[1]" }},
		{"capacity", func(n *corev1.Node) { n.Status.Capacity[corev1.ResourceMemory] = resource.MustParse("16Gi") }},
		{"allocatable", func(n *corev1.Node) { n.Status.Allocatable[corev1.ResourceCPU] = resource.MustParse("7") }},
		{"unschedulable", func(n *corev1.Node) { n.Spec.Unschedulable = true }},
		{"ready-flip", func(n *corev1.Node) { n.Status.Conditions[0].Status = corev1.ConditionFalse }},
		{"internal-ip", func(n *corev1.Node) { n.Status.Addresses[0].Address = "10.0.0.2" }},
		{"node-info", func(n *corev1.Node) { n.Status.NodeInfo.KubeletVersion = "v1.30.1" }},
		{"uid", func(n *corev1.Node) { n.UID = "uid-2" }},
	}
	for _, tc := range mutateCases {
		next := base.DeepCopy()
		tc.mutate(next)
		if !nodeUpdateAffectsInventory(base, next) {
			t.Fatalf("%s change should trigger node inventory refresh", tc.name)
		}
	}
	if !nodeUpdateAffectsInventory(base, nil) {
		t.Fatal("nil node update should fail open")
	}

	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	_ = indexer.Add(base)
	stopCh := make(chan struct{})
	defer close(stopCh)
	provider := &blockingNodeProvider{}
	repo := &nodeRepo{
		data:       &Data{stopCh: stopCh},
		nodeNotify: make(chan struct{}, 1),
		nodes:      map[k8stypes.UID]*biz.Node{},
		nodeLister: listerscorev1.NewNodeLister(indexer),
		log:        log.NewHelper(log.NewStdLogger(io.Discard)),
		providers:  []nodeProvider{{Provider: provider, selector: labels.Everything()}},
	}
	go repo.updateLocalNodes()

	repo.onUpdateNode(base, heartbeat)
	time.Sleep(40 * time.Millisecond)
	if provider.calls.Load() != 0 {
		t.Fatalf("filtered heartbeat unexpectedly invoked FetchDevices %d times", provider.calls.Load())
	}
}

func TestNodeRepoStopsWorkerOnShutdownAndRefreshesOnTicker(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(&corev1.Node{
		ObjectMeta:  metav1.ObjectMeta{Name: "gpu-node", UID: "node-1", Annotations: map[string]string{"rev": "1"}},
	}); err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	provider := &blockingNodeProvider{}
	repo := &nodeRepo{
		data:            &Data{stopCh: stopCh},
		nodeNotify:      make(chan struct{}, 1),
		refreshInterval: 15 * time.Millisecond,
		nodes:           map[k8stypes.UID]*biz.Node{},
		nodeLister:      listerscorev1.NewNodeLister(indexer),
		log:             log.NewHelper(log.NewStdLogger(io.Discard)),
		providers:       []nodeProvider{{Provider: provider, selector: labels.Everything()}},
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		repo.updateLocalNodes()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for provider.calls.Load() < 2 {
		if time.Now().After(deadline) {
			close(stopCh)
			t.Fatalf("expected ticker-driven periodic refreshes, got calls=%d", provider.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(stopCh)
	select {
	case <-workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("updateLocalNodes worker did not exit after stopCh closed")
	}
}
