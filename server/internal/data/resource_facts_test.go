package data

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

func TestResourceFactsProductionSourceUsesLocalSharedCaches(t *testing.T) {
	p := factTestPod()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1", Annotations: map[string]string{"hami.io/node-nvidia-register": "GPU-a,10,16384,100,T4,0,true:"}}}
	client := fake.NewSimpleClientset(p, node)
	stop := make(chan struct{})
	defer close(stop)
	data := &Data{k8sCl: client, informers: informers.NewSharedInformerFactory(client, 0), stopCh: stop}
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
	logger := log.NewStdLogger(io.Discard)
	repo, err := NewPodRepo(data, logger, &conf.Bootstrap{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	// Older configs may omit NVIDIA and carry selectors for other providers.
	providers := NewNodeProviders(data, map[string]string{"FutureVendor": "broken ("}, logger, catalog)
	source := NewResourceSnapshotSource(data, providers, logger, catalog, NewPodFactsLedger(repo))
	if err := wait.PollUntilContextTimeout(context.Background(), time.Millisecond*10, time.Second*3, true, func(context.Context) (bool, error) {
		snapshot, err := source.Snapshot(context.Background())
		return len(snapshot.Devices) == 1 && len(snapshot.Devices[0].EffectiveOccupancies) == 1, err
	}); err != nil {
		t.Fatal(err)
	}
	before := len(client.Actions())
	first, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Devices[0].EffectiveOccupancies[0].ComputeAllocatedShare.Unit != "share" || first.Devices[0].ComputeCapacityShare.Unit != "share" {
		t.Fatal("share unit changed at provider boundary")
	}
	first.Devices[0].Identity.Node = "mutated"
	first.Nodes[0].Name = "mutated"
	second, err := source.Snapshot(context.Background())
	if err != nil || second.Devices[0].Identity.Node != "node-a" || second.Nodes[0].Name != "node-a" {
		t.Fatal("read mutated shared facts")
	}
	if len(client.Actions()) != before {
		t.Fatal("resource snapshot performed network reads")
	}
	card := second.Devices[0]
	if v, ok := card.MemoryCapacityBytes.Value.AsInt64(); !ok || v != 16384*1024*1024 || card.MemoryUsedBytes.Value != nil || second.DeviceCoverage.Complete {
		t.Fatal("snapshot changed capacity units or fabricated telemetry/completeness")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Snapshot(canceled); err != context.Canceled {
		t.Fatal("canceled snapshot ignored")
	}
}

func TestResourceSnapshotSourceCaptureContinuityLagAndDeleteGuard(t *testing.T) {
	p := factTestPod()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1", Annotations: map[string]string{"hami.io/node-nvidia-register": "GPU-a,10,16384,100,T4,0,true:"}}}
	client := fake.NewSimpleClientset(p, node)
	stop := make(chan struct{})
	defer close(stop)
	data := &Data{k8sCl: client, informers: informers.NewSharedInformerFactory(client, 0), stopCh: stop}
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
	logger := log.NewStdLogger(io.Discard)
	repo, err := NewPodRepo(data, logger, &conf.Bootstrap{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	providers := NewNodeProviders(data, nil, logger, catalog)
	source := NewResourceSnapshotSource(data, providers, logger, catalog, NewPodFactsLedger(repo)).(*resourceSnapshotSource)

	if err := wait.PollUntilContextTimeout(context.Background(), time.Millisecond*10, time.Second*3, true, func(context.Context) (bool, error) {
		cap, err := source.Capture(context.Background())
		return err == nil && len(cap.Resources.Devices) == 1 && len(cap.Resources.VerifiedBindings) == 3, err
	}); err != nil {
		t.Fatal(err)
	}

	cap1, err := source.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	opaqueID, err := resourceview.DeviceID("NVIDIA", "node-a", "GPU-a")
	if err != nil {
		t.Fatal(err)
	}
	if dev, ok := cap1.LookupDevice(opaqueID); !ok || dev.Identity.DeviceID != "GPU-a" {
		t.Fatalf("Capture LookupDevice failed: ok=%v dev=%+v", ok, dev)
	}
	if n, ok := cap1.LookupNode("node-uid"); !ok || n.Name != "node-a" {
		t.Fatalf("Capture LookupNode failed: ok=%v node=%+v", ok, n)
	}
	if ctr, ok := cap1.LookupContainer(podfacts.ContainerKey{PodUID: p.UID, Name: "app"}); !ok || ctr.Key.Name != "app" {
		t.Fatalf("Capture LookupContainer failed: ok=%v ctr=%+v", ok, ctr)
	}

	// Stale DeleteFunc callback must not remove a live Pod still present in podLister.
	repo.onFactsDelete(p)
	if _, ok := repo.facts.Get(p.UID); !ok {
		t.Fatal("stale delete removed live Pod from ledger")
	}

	// Lagging InputRevision preserves the live Pod/Device while degrading AllocationCoverage.
	entry, ok := repo.facts.Get(p.UID)
	if !ok {
		t.Fatal("missing pod entry")
	}
	if !repo.facts.PutWithRevision(entry.Facts, func() bool { return true }, "stale-input-rev", entry.Assignment) {
		t.Fatal("expected stale PutWithRevision to succeed")
	}
	cap2, err := source.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cap2.Generation <= cap1.Generation {
		t.Fatalf("expected strictly increasing generation: cap2=%d cap1=%d", cap2.Generation, cap1.Generation)
	}
	if len(cap2.Resources.Devices) != 1 || len(cap2.Pods.Entries) != 1 {
		t.Fatalf("lagging pod or device dropped: devices=%d pods=%d", len(cap2.Resources.Devices), len(cap2.Pods.Entries))
	}
	if len(cap2.Resources.VerifiedBindings) != 0 {
		t.Fatalf("expected lagging Pod bindings excluded from VerifiedBindings, got %+v", cap2.Resources.VerifiedBindings)
	}
	snap := source.Snapshot
	snapCopy, err := snap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapCopy.Devices[0].Model = "MutatedByCaller"
	if stored, ok := source.captures.Load().LookupDevice(opaqueID); !ok || stored.Model == "MutatedByCaller" {
		t.Fatalf("Snapshot() mutated published ClusterCapture device: ok=%v dev=%+v", ok, stored)
	}
	allocCov := cap2.Resources.Devices[0].AllocationCoverage
	if allocCov.Complete {
		t.Fatal("expected incomplete AllocationCoverage under InputRevision lag")
	}
	foundPending := false
	for _, s := range allocCov.Sources {
		if s.Params["continuity"] == "dependent_fact_replay_pending" {
			foundPending = true
			break
		}
	}
	if !foundPending {
		t.Fatalf("expected dependent_fact_replay_pending source, got %+v", allocCov.Sources)
	}
}
