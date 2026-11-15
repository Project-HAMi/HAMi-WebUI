package data

import (
	"context"
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"

	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/resourceview"
)

// A real API server supplies a resource version on every LIST. The client-go
// tracker leaves it empty by default, so its WATCH cannot prove continuity.
func newResourceTestClient(objects ...runtime.Object) *fake.Clientset {
	client := fake.NewSimpleClientset(objects...)
	client.PrependReactor("list", "*", func(a kt.Action) (bool, runtime.Object, error) {
		action := a.(kt.ListActionImpl)
		list, err := client.Tracker().List(action.GetResource(), action.GetKind(), action.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		metadata, err := meta.ListAccessor(list)
		if err != nil {
			return true, nil, err
		}
		metadata.SetResourceVersion("100")
		return true, list, nil
	})
	return client
}

func startResourceTestSource(t *testing.T, client *fake.Clientset, catalog *mutableCatalog, selectors map[string]string) (*podRepo, resourceview.SnapshotSource) {
	t.Helper()
	factory, continuity := newTrackedInformerFactory(client, 0)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	data := &Data{k8sCl: client, informers: factory, continuity: continuity, stopCh: stop}
	logger := log.NewStdLogger(io.Discard)
	repo, err := NewPodRepo(data, logger, &conf.Bootstrap{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewResourceSnapshotSource(data, selectors, logger, catalog, repo.facts)
	if err != nil {
		t.Fatal(err)
	}
	return repo, source
}

func TestProductionContinuityZeroRecoveryAndNodeUIDReplacement(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-old", ResourceVersion: "1", Annotations: map[string]string{"hami.io/node-nvidia-register": "GPU-a,10,16384,100,T4,0,true:"}}, Status: corev1.NodeStatus{Capacity: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("10")}}}
	other := node.DeepCopy()
	other.Name, other.UID = "node-b", "other-uid"
	other.Annotations["hami.io/node-nvidia-register"] = "GPU-b,10,16384,100,T4,0,true:"
	client := newResourceTestClient(node, other)
	streams := make(chan watch.Interface, 5)
	client.PrependWatchReactor("pods", func(a kt.Action) (bool, watch.Interface, error) {
		w, err := client.Tracker().Watch(a.GetResource(), a.GetNamespace())
		streams <- w
		return true, w, err
	})
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
	repo, source := startResourceTestSource(t, client, catalog, map[string]string{"NVIDIA": ""})
	snapshot := func() resourceview.Snapshot {
		v, e := source.Snapshot(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	awaitContinuity(t, func() bool { s := snapshot(); return len(s.Devices) == 2 && s.Devices[0].AllocationCoverage.Complete })
	if got := snapshot().Devices[0].AllocationCoverage; got.Total == nil || *got.Total != 0 {
		t.Fatal("empty live source did not establish zero")
	}
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "worker", Namespace: "work", UID: "pod-old", ResourceVersion: "2",
			Annotations: map[string]string{"hami.io/vgpu-node": "node-a", "hami.io/vgpu-devices-allocated": ";"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-a",
			Containers: []corev1.Container{{
				Name: "app", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if _, err := client.CoreV1().Pods(p.Namespace).Create(context.Background(), p, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		s := snapshot()
		c := s.Devices[0].AllocationCoverage
		if c.Complete || c.Total != nil || !s.Devices[1].AllocationCoverage.Complete {
			return false
		}
		for _, source := range c.Sources {
			if source.Source == "hami_annotation:NVIDIA" && source.Params["completeness"] == "candidate_allocation_source_missing" {
				return true
			}
		}
		return false
	})
	p.ResourceVersion = "3"
	p.Annotations["hami.io/vgpu-devices-allocated"] = "GPU-a,NVIDIA,4096,20:;"
	if _, err := client.CoreV1().Pods(p.Namespace).Update(context.Background(), p, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		d := snapshot().Devices[0]
		return d.AllocationCoverage.Complete && len(d.EffectiveOccupancies) == 1
	})
	// A dependent Node/catalog replay is not proved by matching Pod RV alone.
	entry, _ := repo.facts.Get(p.UID)
	repo.facts.PutWithRevision(entry.Facts, func() bool { return true }, "pending-dependent-replay", entry.Assignment)
	pending := snapshot()
	if pending.Devices[0].AllocationCoverage.Complete {
		t.Fatal("old dependency revision accepted as complete")
	}
	if other := pending.Devices[1]; other.Identity.Node != "node-b" || !other.AllocationCoverage.Complete || other.AllocationCoverage.Total == nil || *other.AllocationCoverage.Total != 0 {
		t.Fatalf("node-a replay invalidated unaffected node-b: %+v", other.AllocationCoverage)
	}
	// Conflicting bindings put both nodes in scope. Without either binding,
	// the graph cannot safely limit the replay to any particular node.
	for _, binding := range []struct{ bound, assigned string }{{"node-a", "node-b"}, {"", ""}} {
		entry.Facts.NodeName = binding.bound
		entry.Facts.Metadata.Annotations["hami.io/vgpu-node"] = binding.assigned
		repo.facts.PutWithRevision(entry.Facts, func() bool { return true }, "pending-dependent-replay", entry.Assignment)
		s := snapshot()
		if s.Devices[0].AllocationCoverage.Complete || s.Devices[1].AllocationCoverage.Complete {
			t.Fatalf("replay with bindings %+v must leave both nodes incomplete", binding)
		}
	}
	repo.onFactsPod(p)
	awaitContinuity(t, func() bool { return snapshot().Devices[0].AllocationCoverage.Complete })
	(<-streams).Stop()
	awaitContinuity(t, func() bool { return !snapshot().Devices[0].AllocationCoverage.Complete })
	s := snapshot()
	if s.Devices[1].AllocationCoverage.Complete {
		t.Fatal("Pod watch interruption must invalidate allocations on both nodes")
	}
	if !s.NodeCoverage.Complete || !s.DeviceCoverage.Complete || !s.Devices[0].Coverage.Complete {
		t.Fatal("Pod watch interruption invalidated independent physical device inventory")
	}
	awaitContinuity(t, func() bool { return snapshot().Devices[0].AllocationCoverage.Complete })
	lists := map[string]int{}
	for _, action := range client.Actions() {
		if action.GetVerb() == "list" {
			lists[action.GetResource().Resource]++
		}
	}
	if lists["pods"] != 2 || lists["nodes"] != 1 {
		t.Fatalf("unexpected LIST counts: %v", lists)
	}
	node.UID = "node-new"
	node.ResourceVersion = "3"
	if _, err := client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		s := snapshot()
		return s.Devices[0].Identity.NodeUID == "node-new" && len(s.Devices[0].EffectiveOccupancies) == 0 && !s.Devices[0].AllocationCoverage.Complete
	})
	p.ResourceVersion = "4"
	if _, err := client.CoreV1().Pods(p.Namespace).Update(context.Background(), p, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		entry, ok := repo.facts.Get(p.UID)
		return ok && entry.Facts.Metadata.ResourceVersion == "4"
	})
	if len(snapshot().Devices[0].EffectiveOccupancies) != 0 || repo.facts.NodeAssignment(p.UID).UID != "node-old" {
		t.Fatal("later replay forgot the replaced assignment Node UID")
	}
	if err := client.CoreV1().Pods(p.Namespace).Delete(context.Background(), p.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	p.UID = "pod-new"
	p.ResourceVersion = "5"
	if _, err := client.CoreV1().Pods(p.Namespace).Create(context.Background(), p, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		d := snapshot().Devices[0]
		return d.AllocationCoverage.Complete && len(d.EffectiveOccupancies) == 1 && d.EffectiveOccupancies[0].PodUID == "pod-new"
	})
	if _, exists := repo.facts.Get("pod-old"); exists {
		t.Fatal("old Pod UID revived")
	}
	if err := client.CoreV1().Pods(p.Namespace).Delete(context.Background(), p.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		d := snapshot().Devices[0]
		return d.AllocationCoverage.Complete && d.AllocationCoverage.Total != nil && *d.AllocationCoverage.Total == 0
	})
}
