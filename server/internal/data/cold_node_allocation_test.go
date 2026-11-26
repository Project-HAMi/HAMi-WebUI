package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8stypes "k8s.io/apimachinery/pkg/types"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/resourceview"
	"vgpu/internal/service"
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
					repo.mutex.RLock()
					defer repo.mutex.RUnlock()
					return ok && repo.pods[pod.UID] != nil && entry.InputRevision == factInputRevision(catalog.current, node) &&
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
				total := h.Reservations[alias]
				if total.Slots != 1 || total.MemoryMiB != 2048 {
					t.Fatalf("verified reservation = %+v", total)
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

func TestInitDeviceReservationThroughRelease(t *testing.T) {
	for _, vendor := range []string{"NVIDIA", "Ascend"} {
		for _, tc := range []struct {
			name    string
			appMiB  int32
			sidecar bool
		}{
			{name: "init only"}, {name: "shared with app", appMiB: 2048}, {name: "restartable sidecar", appMiB: 2048, sidecar: true},
		} {
			t.Run(vendor+"/"+tc.name, func(t *testing.T) {
				node, pod := coldAllocationObjects(vendor)
				pod.Status.Phase = corev1.PodPending
				setup := pod.Spec.Containers[0].DeepCopy()
				setup.Name = "setup"
				if tc.sidecar {
					restart := corev1.ContainerRestartPolicyAlways
					setup.RestartPolicy = &restart
				}
				pod.Spec.InitContainers = []corev1.Container{*setup}
				alias := "GPU-a"
				if vendor == "Ascend" {
					alias = "NPU-0"
				}
				for k, v := range pod.Annotations {
					if !strings.HasSuffix(k, "-devices-allocated") {
						continue
					}
					if tc.appMiB > 0 {
						pod.Annotations[k] = v + strings.Replace(v, "4096", "2048", 1)
					} else {
						pod.Annotations[k] = v + ";"
						pod.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
					}
				}
				pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "setup", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
				catalog := &mutableCatalog{current: loadedTestCatalog(t)}
				catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
				client := newResourceTestClient(node, pod)
				repo, source := startResourceTestSource(t, client, catalog, map[string]string{vendor: ""})
				check := func(wantMem, wantSlots int32, wantInit bool) {
					t.Helper()
					awaitContinuity(t, func() bool {
						e, ok := repo.facts.Get(pod.UID)
						if !ok || e.Facts.Metadata.ResourceVersion != pod.ResourceVersion || e.InputRevision != factInputRevision(catalog.current, node) {
							return false
						}
						h, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
						if err != nil {
							return false
						}
						total := h.Reservations[alias]
						if h.UncertainDeviceIDs[alias] || total.MemoryMiB != wantMem || total.HasInit != wantInit || total.Slots != wantSlots {
							return false
						}
						for _, c := range h.Containers {
							if c.Name == "setup" {
								t.Fatal("init should not appear as a linked app workload")
							}
						}
						return repo.data.continuity.nodes.snapshot().ready && repo.data.continuity.pods.snapshot().ready
					})
					card := deviceOnTestNode(t, source, node)
					var mib int64
					for _, o := range card.EffectiveOccupancies {
						if q := o.MemoryAllocatedBytes.Value; q != nil {
							mib += q.Value() >> 20
						}
					}
					if !card.AllocationCoverage.Complete || mib != int64(wantMem) {
						t.Fatalf("inventory %d MiB, detail %d MiB", mib, wantMem)
					}
				}
				activeMiB, slots := int32(4096), int32(1)
				if tc.sidecar {
					activeMiB += tc.appMiB
					slots++
				}
				check(activeMiB, slots, true)
				pod = pod.DeepCopy()
				pod.ResourceVersion = "14"
				pod.Status.Phase = corev1.PodRunning
				if !tc.sidecar {
					pod.Status.InitContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}
				}
				if _, err := client.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
					t.Fatal(err)
				}
				switch {
				case tc.sidecar:
					check(activeMiB, slots, true)
				case tc.appMiB > 0:
					check(tc.appMiB, 1, false)
				default:
					check(0, 0, false)
				}
			})
		}
	}
}

func TestConflictingInitPeaksDoNotSumOnOneCard(t *testing.T) {
	slots, memory := resource.MustParse("1"), resource.MustParse("4Gi")
	row := podfacts.Occupancy{Node: "node", NodeUID: "uid", DeviceID: "card", Source: "hami_annotation", Resource: "gpu", Quantities: map[string]podfacts.Quantity{
		"slots": {Value: &slots, Unit: "slots"}, "memory": {Value: &memory, Unit: "bytes"},
	}}
	other := row
	other.Resource = "another-resource"
	h := biz.NodeHolders{Reservations: map[string]biz.DeviceReservation{}, UncertainDeviceIDs: map[string]bool{}}
	addInitReservations(&h, map[string]bool{"card": true}, podfacts.NodeAllocation{UID: "uid", Peaks: []podfacts.Occupancy{row, other}}, "node", "uid")
	if !h.UncertainDeviceIDs["card"] || len(h.Reservations) != 0 {
		t.Fatalf("ambiguous init reservation: %+v", h)
	}
}

// A rejected annotation must suppress the idle state without hiding a valid
// reservation on the same card or marking unrelated cards incomplete.
func TestNodeDetailWithUnreadableAllocation(t *testing.T) {
	for _, tc := range []struct {
		vendor, alias, damaged string
		vendorWide             bool
	}{
		{"NVIDIA", "GPU-a", "GPU-a,NVIDIA,4096,invalid:;", false},
		{"Ascend", "NPU-0", "NPU-0,Ascend910B3,4096,invalid:;", false},
		{"NVIDIA", "GPU-a", ",NVIDIA,4096,invalid:;", true},
		{"Ascend", "NPU-0", "damaged:;", true},
	} {
		t.Run(tc.damaged, func(t *testing.T) {
			node, bad := coldAllocationObjects(tc.vendor)
			good := bad.DeepCopy()
			good.Name, good.UID = "valid", "valid-uid"
			for key, v := range bad.Annotations {
				if strings.HasSuffix(key, "-devices-allocated") {
					good.Annotations[key] = strings.Replace(v, "4096", "2048", 1)
					bad.Annotations[key] = tc.damaged
				}
			}
			catalog := &mutableCatalog{current: loadedTestCatalog(t)}
			repo, _ := startResourceTestSource(t, newResourceTestClient(node, bad, good), catalog, map[string]string{tc.vendor: ""})
			awaitContinuity(t, func() bool {
				_, a := repo.facts.Get(bad.UID)
				_, b := repo.facts.Get(good.UID)
				repo.mutex.RLock()
				defer repo.mutex.RUnlock()
				return a && b && repo.pods[bad.UID] != nil && repo.pods[good.UID] != nil
			})
			uid := string(node.UID)
			devices := []*biz.DeviceInfo{
				{Id: "affected", AliasId: tc.alias, NodeName: node.Name, NodeUid: uid, Provider: tc.vendor},
				{Id: "sibling", AliasId: "another", NodeName: node.Name, NodeUid: uid, Provider: tc.vendor},
				{Id: "other", AliasId: "unrelated", NodeName: node.Name, NodeUid: uid, Provider: "Other"},
			}
			nodes := &nodeRepo{nodes: map[k8stypes.UID]*biz.Node{node.UID: {Devices: devices}}}
			cards := service.NewCardService(biz.NewNodeUsecase(nodes, log.DefaultLogger), biz.NewPodUseCase(repo, log.DefaultLogger))
			reply, err := cards.GetAllGPUs(context.Background(), &pb.GetAllGpusReq{Filters: &pb.GetAllGpusReq_Filters{NodeName: node.Name, NodeUid: uid}})
			if err != nil {
				t.Fatal(err)
			}
			if len(reply.List) != len(devices) {
				t.Fatalf("got %d cards, want %d", len(reply.List), len(devices))
			}
			for _, card := range reply.List {
				complete := card.Uuid == "other" || (card.Uuid == "sibling" && !tc.vendorWide)
				if card.NodeAllocationComplete == nil || card.GetNodeAllocationComplete() != complete {
					t.Errorf("%s: want allocation complete=%t, got %t (field present=%t)", card.Uuid, complete, card.GetNodeAllocationComplete(), card.NodeAllocationComplete != nil)
				}
				if card.Uuid == "affected" && card.MemoryUsed != 2048 {
					t.Errorf("known reservation = %d MiB, want 2048", card.MemoryUsed)
				}
			}
		})
	}
}

func TestMissingSecondRequestedDevice(t *testing.T) {
	for _, vendor := range []string{"NVIDIA", "Ascend"} {
		t.Run(vendor, func(t *testing.T) {
			node, pod := coldAllocationObjects(vendor)
			second := pod.Spec.Containers[0].DeepCopy()
			second.Name = "second"
			pod.Spec.Containers = append(pod.Spec.Containers, *second)
			catalog := loadedTestCatalog(t)
			catalog.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
			repo := factRepo(t, &mutableCatalog{current: catalog}, pod)
			evidence, _ := repo.hamiFactEvidence(pod, node, catalog)
			hints := evidence.UnreadableAllocations
			if len(hints) != 1 || hints[0].Provider != vendor || hints[0].DeviceID != "" {
				t.Fatalf("unidentified second card: got %+v", hints)
			}
		})
	}
}

func TestFractionalInitComputeShare(t *testing.T) {
	catalog := loadedTestCatalog(t, devicecatalog.AscendModel{CommonWord: "TestNPU", AICore: 32,
		Templates: []devicecatalog.AscendTemplate{{Name: "one-core", AICore: 1, Memory: 8}}})
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "work", Name: "p", UID: "pod-uid", ResourceVersion: "1",
			Annotations: map[string]string{
				provider.AssignedNodeAnnotation:     "node",
				"hami.io/TestNPU-devices-allocated": "card,TestNPU,8,10:;",
				ascend.VNPUModeAnnotation:           ascend.ModeTemplate,
				"huawei.com/TestNPU":                `[{"UUID":"card","temp":"one-core","memory":8}]`,
			},
		},
		Spec: corev1.PodSpec{NodeName: "node", Containers: []corev1.Container{{Name: "app"}}},
	}
	out := ascend.NewAllocationDialect("").ExtractPodAllocations(provider.PodAllocationContext{
		Pod:            pod,
		Catalog:        catalog,
		ContainerNames: []string{"app"},
	})
	record := out.Allocations[0].Records[0]
	peak := podfacts.Occupancy{DeviceID: "card", Node: "node", NodeUID: "uid", Quantities: record.Quantities}
	h := biz.NodeHolders{Reservations: map[string]biz.DeviceReservation{}, UncertainDeviceIDs: map[string]bool{}}
	addInitReservations(&h, map[string]bool{"card": true}, podfacts.NodeAllocation{UID: "uid", Peaks: []podfacts.Occupancy{peak}}, "node", "uid")
	if got := h.Reservations["card"]; got.CoreUnknown || got.Cores != 3 {
		t.Fatalf("expected the app container's rounded share of 3%%, got %+v", got)
	}
}

func TestMissingCambriconAllocationRow(t *testing.T) {
	node, pod := coldAllocationObjects("NVIDIA")
	const resourceName = "custom.example/mlu"
	pod.Annotations = map[string]string{
		"hami.io/vgpu-node": node.Name, "hami.io/cambricon-mlu-devices-allocated": "node-a-cambricon-mlu-0,MLU,2048,20:;",
	}
	pod.Spec.Containers = []corev1.Container{{Name: "one"}, {Name: "two"}}
	for i := range pod.Spec.Containers {
		pod.Spec.Containers[i].Resources.Limits = corev1.ResourceList{resourceName: resource.MustParse("1")}
	}
	catalog := loadedTestCatalog(t)
	catalog.Cambricon = &devicecatalog.CambriconConfig{ResourceCountName: resourceName}
	repo := factRepo(t, &mutableCatalog{current: catalog}, pod)
	evidence, _ := repo.hamiFactEvidence(pod, node, catalog)
	if hints := evidence.UnreadableAllocations; len(hints) != 1 || hints[0].Provider != "MLU" || hints[0].DeviceID != "" {
		t.Fatalf("second MLU has no recorded ID: got hints %+v", hints)
	}
}

func TestPartialInitPeakPreservesKnownAppAllocation(t *testing.T) {
	node, pod := coldAllocationObjects("NVIDIA")
	original := pod.Spec.Containers[0]
	pod.Spec.InitContainers = []corev1.Container{original}
	pod.Spec.InitContainers[0].Name = "setup"
	pod.Spec.Containers = []corev1.Container{original, original, original}
	pod.Spec.Containers[1].Name, pod.Spec.Containers[2].Name = "two", "three"
	pod.Annotations["hami.io/vgpu-devices-allocated"] = "GPU-a,NVIDIA,4096,20:;GPU-a,NVIDIA,2048,10:;"
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo, _ := startResourceTestSource(t, newResourceTestClient(node, pod), catalog, map[string]string{"NVIDIA": ""})
	awaitContinuity(t, func() bool {
		_, ok := repo.facts.Get(pod.UID)
		repo.mutex.RLock()
		defer repo.mutex.RUnlock()
		return ok && repo.pods[pod.UID] != nil
	})
	h, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Reservations["GPU-a"]; !h.UncertainDeviceIDs["GPU-a"] || got.MemoryMiB != 2048 || got.Cores != 10 || got.Slots != 1 {
		t.Fatalf("incomplete peak should retain the confirmed app row: reservations=%+v uncertain=%v", got, h.UncertainDeviceIDs)
	}
}
