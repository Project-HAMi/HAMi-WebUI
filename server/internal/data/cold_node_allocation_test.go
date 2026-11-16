package data

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/biz"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/resourceview"
)

func TestColdStartAllocationAfterNodeReplacement(t *testing.T) {
	nodeCreated := time.Unix(2000, 0)
	before := metav1.NewTime(nodeCreated.Add(-time.Minute))
	after := metav1.NewTime(nodeCreated.Add(time.Minute))
	for _, vendor := range []string{"NVIDIA", "Ascend"} {
		for _, tc := range []struct {
			name      string
			scheduled *metav1.Time
			started   *metav1.Time
			filter    string
			unbound   bool
			wantKnown bool
		}{
			{name: "scheduled on old node", scheduled: &before, started: &after},
			{name: "started on old node without schedule condition", started: &before},
			{name: "created early but scheduled on new node", scheduled: &after, started: &after, wantKnown: true},
			{name: "scheduled on new node with an earlier kubelet clock", scheduled: &after, started: &before, wantKnown: true},
			{name: "unbound with old Filter", filter: "1940", unbound: true},
			{name: "unbound with new Filter", filter: "2060", unbound: true, wantKnown: true},
			{name: "unbound without dated Filter", unbound: true},
			{name: "unbound with same second Filter", filter: "2000", unbound: true},
			{name: "late Bind retains old Filter", scheduled: &after, filter: "1940"},
			{name: "late Bind retains ambiguous same-second Filter", scheduled: &after, filter: "2000"},
		} {
			t.Run(vendor+"/"+tc.name, func(t *testing.T) {
				node, pod := coldAllocationObjects(vendor)
				node.CreationTimestamp = metav1.NewTime(nodeCreated)
				pod.CreationTimestamp = metav1.NewTime(nodeCreated.Add(-time.Hour))
				pod.Status = corev1.PodStatus{Phase: corev1.PodRunning, StartTime: tc.started}
				if tc.unbound {
					pod.Spec.NodeName = ""
					pod.Status.Phase = corev1.PodPending
				}
				if tc.filter != "" {
					pod.Annotations["hami.io/vgpu-time"] = tc.filter
				}
				if tc.scheduled != nil {
					pod.Status.Conditions = []corev1.PodCondition{{
						Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: *tc.scheduled,
					}}
				}
				catalog := &mutableCatalog{current: loadedTestCatalog(t)}
				catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
				repo, source := startResourceTestSource(t, newResourceTestClient(node, pod), catalog, map[string]string{vendor: ""})
				// Wait for the initial LIST and fact replay before checking a negative.
				awaitContinuity(t, func() bool {
					entry, ok := repo.facts.Get(pod.UID)
					return ok && entry.InputRevision == factInputRevision(catalog.current, node) &&
						repo.data.continuity.nodes.snapshot().ready && repo.data.continuity.pods.snapshot().ready
				})
				holders, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
				if err != nil {
					t.Fatal(err)
				}
				if tc.wantKnown {
					if len(holders.Containers) != 1 || holders.Containers[0].NodeUID != string(node.UID) || len(holders.UncertainDeviceIDs) != 0 {
						t.Fatalf("expected verified holder, got %+v", holders)
					}
				} else if len(holders.Containers) != 0 || len(holders.UncertainDeviceIDs) != 1 {
					t.Fatalf("expected unresolved reservation, got %+v", holders)
				}
				card := deviceOnTestNode(t, source, node)
				occupancies := card.EffectiveOccupancies
				if !tc.wantKnown {
					if len(occupancies) != 0 || card.AllocationCoverage.Complete || card.AllocationCoverage.Total != nil {
						t.Fatalf("old Pod assigned to replacement node: occupancies=%+v coverage=%+v", occupancies, card.AllocationCoverage)
					}
					return
				}
				requireFourGiBAllocation(t, card, pod)
			})
		}
	}
}

// An unresolved reservation on a reused device alias must not hide a proven
// reservation on the same card, even if its facts handler is one revision behind.
func TestMixedNodeHoldersAfterReplacement(t *testing.T) {
	for _, vendor := range []string{"NVIDIA", "Ascend"} {
		t.Run(vendor, func(t *testing.T) {
			node, old := coldAllocationObjects(vendor)
			node.CreationTimestamp = metav1.NewTime(time.Unix(2000, 0))
			old.CreationTimestamp = metav1.NewTime(time.Unix(1000, 0))
			old.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(1500, 0))}}
			late := old.DeepCopy()
			late.Name, late.UID, late.ResourceVersion = "late", "late-uid", "14"
			late.Status.Conditions[0].LastTransitionTime = metav1.NewTime(time.Unix(2100, 0))
			for k, v := range late.Annotations {
				if strings.HasSuffix(k, "-devices-allocated") {
					late.Annotations[k] = strings.Replace(v, "4096", "2048", 1)
				}
			}
			catalog := &mutableCatalog{current: loadedTestCatalog(t)}
			repo, _ := startResourceTestSource(t, newResourceTestClient(node, old, late), catalog, map[string]string{vendor: ""})
			awaitContinuity(t, func() bool {
				_, a := repo.facts.Get(old.UID)
				_, b := repo.facts.Get(late.UID)
				repo.mutex.RLock()
				defer repo.mutex.RUnlock()
				return a && b && repo.pods[old.UID] != nil && repo.pods[late.UID] != nil
			})
			e, _ := repo.facts.Get(late.UID)
			e.Facts.Metadata.ResourceVersion = "13"
			repo.facts.PutWithRevision(e.Facts, func() bool { return true }, e.InputRevision, e.Assignment)
			h, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
			if err != nil || len(h.Containers) != 1 || h.Containers[0].PodUID != string(late.UID) || len(h.UncertainDeviceIDs) != 1 {
				t.Fatalf("expected only the late-bound container: %+v, %v", h, err)
			}
			for alias := range h.UncertainDeviceIDs {
				n, _, mem, _ := biz.ExactDeviceStatistics(h.Containers, alias)
				if n != 1 || mem != 2048 {
					t.Fatalf("verified reservations = %d, %d MiB", n, mem)
				}
			}
		})
	}
}

// The replacement advertises the exact device identifier in the Pod annotation.
func coldAllocationObjects(vendor string) (*corev1.Node, *corev1.Pod) {
	registration := map[string]string{"hami.io/node-nvidia-register": "GPU-a,8,16384,100,T4,0,true:"}
	assignment := map[string]string{"hami.io/vgpu-node": "node-a", "hami.io/vgpu-devices-allocated": "GPU-a,NVIDIA,4096,20:;"}
	countResource := corev1.ResourceName("nvidia.com/gpu")
	if vendor == "Ascend" {
		registration = map[string]string{
			"hami.io/node-register-Ascend910B3": `[{"id":"NPU-0","type":"Ascend910B3","count":8,"devmem":65536,"devcore":20,"health":true}]`,
			"hami-vnpu-core":                    "true",
		}
		assignment = map[string]string{
			"hami.io/vgpu-node": "node-a", "hami.io/Ascend910B3-devices-allocated": "NPU-0,Ascend910B3,4096,20:;",
			ascend.VNPUModeAnnotation: "hami-core",
		}
		countResource = "huawei.com/Ascend910B3"
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "new-node-uid", ResourceVersion: "12", Annotations: registration},
		Status:     corev1.NodeStatus{Capacity: corev1.ResourceList{countResource: resource.MustParse("8")}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "work", UID: "worker-uid", ResourceVersion: "13", Annotations: assignment},
		Spec: corev1.PodSpec{NodeName: node.Name, Containers: []corev1.Container{{
			Name: "app", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{countResource: resource.MustParse("1")}},
		}}},
	}
	return node, pod
}

func TestNominatedPodBindsAfterNodeReplacement(t *testing.T) {
	node, bound := coldAllocationObjects("NVIDIA")
	node.UID = "old-node-uid"
	node.CreationTimestamp = metav1.NewTime(time.Unix(100, 0))
	bound.CreationTimestamp = metav1.NewTime(time.Unix(200, 0))
	pending := bound.DeepCopy()
	pending.Spec.NodeName = ""
	pending.Status.Phase = corev1.PodPending
	delete(pending.Annotations, "hami.io/vgpu-devices-allocated")

	ctx := context.Background()
	client := newResourceTestClient(node, pending)
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
	repo, source := startResourceTestSource(t, client, catalog, map[string]string{"NVIDIA": ""})
	waitForReplay := func(node *corev1.Node, pod *corev1.Pod) {
		t.Helper()
		awaitContinuity(t, func() bool {
			entry, ok := repo.facts.Get(pod.UID)
			return ok && entry.InputRevision == factInputRevision(catalog.current, node) && entry.Facts.Metadata.ResourceVersion == pod.ResourceVersion
		})
	}
	waitForReplay(node, pending)
	replacement := node.DeepCopy()
	replacement.ResourceVersion, replacement.UID = "14", "new-node-uid"
	replacement.CreationTimestamp = metav1.NewTime(time.Unix(300, 0))
	if err := client.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Nodes().Create(ctx, replacement, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForReplay(replacement, pending)

	scheduled := metav1.NewTime(time.Unix(400, 0))
	bound.ResourceVersion = "15"
	bound.Status = corev1.PodStatus{Phase: corev1.PodRunning, StartTime: &scheduled, Conditions: []corev1.PodCondition{{
		Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: scheduled,
	}}}
	if _, err := client.CoreV1().Pods(bound.Namespace).Update(ctx, bound, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForReplay(replacement, bound)
	requireFourGiBAllocation(t, deviceOnTestNode(t, source, replacement), bound)
}

func deviceOnTestNode(t *testing.T, source resourceview.SnapshotSource, node *corev1.Node) resourceview.Device {
	t.Helper()
	snapshot, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Devices) != 1 || snapshot.Devices[0].Identity.NodeUID != node.UID {
		t.Fatalf("expected one device on node %s, got %+v", node.UID, snapshot.Devices)
	}
	return snapshot.Devices[0]
}

func requireFourGiBAllocation(t *testing.T, card resourceview.Device, pod *corev1.Pod) {
	t.Helper()
	rows := card.EffectiveOccupancies
	if len(rows) != 1 || rows[0].PodUID != pod.UID || !card.AllocationCoverage.Complete {
		t.Fatalf("expected a complete allocation for Pod %s: occupancies=%+v coverage=%+v", pod.UID, rows, card.AllocationCoverage)
	}
	if mem := rows[0].MemoryAllocatedBytes.Value; mem == nil || mem.Value() != 4<<30 {
		t.Fatalf("expected exactly 4 GiB from Pod %s, got %v", pod.UID, mem)
	}
}
