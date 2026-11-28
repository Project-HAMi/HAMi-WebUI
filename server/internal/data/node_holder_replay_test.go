package data

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/ascend"
)

// Update the shared index directly so the request can run between the informer
// store update and its asynchronous Node handler, without relying on timing.
func TestNodeHoldersDuringNodeReplay(t *testing.T) {
	for _, vendor := range []string{"NVIDIA", "Ascend"} {
		for _, stage := range []string{"missing node", "old node cached", "Filter reservation", "registration added"} {
			t.Run(vendor+"/"+stage, func(t *testing.T) {
				node, pod := coldAllocationObjects(vendor)
				node.CreationTimestamp = metav1.NewTime(time.Unix(2000, 0))
				pod.CreationTimestamp = metav1.NewTime(time.Unix(1100, 0))
				pod.Annotations["hami.io/vgpu-time"] = "2100"
				delete(pod.Annotations, ascend.VNPUModeAnnotation)
				pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(time.Unix(2101, 0))}}
				for k, v := range pod.Annotations {
					if strings.HasSuffix(k, "-devices-allocated") {
						pod.Annotations[k] = strings.Replace(v, "4096", "2048", 1)
					}
				}
				catalog := &mutableCatalog{current: loadedTestCatalog(t)}
				repo, _ := startResourceTestSource(t, newResourceTestClient(), catalog, map[string]string{vendor: ""})
				repo.ascend.Policy = ascend.PolicyNode
				nodes := repo.data.informers.Core().V1().Nodes().Informer().GetIndexer()
				switch stage {
				case "old node cached", "registration added":
					first := node.DeepCopy()
					first.ResourceVersion = "11"
					if stage == "old node cached" {
						first.UID = "old-uid"
						first.CreationTimestamp = metav1.NewTime(time.Unix(1000, 0))
					} else {
						first.Annotations = nil
					}
					if err := nodes.Add(first); err != nil {
						t.Fatal(err)
					}
				case "Filter reservation":
					pod.Spec.NodeName = ""
					pod.Status.Conditions = nil
					if err := nodes.Add(node); err != nil {
						t.Fatal(err)
					}
				}
				if err := repo.podIndexer.Add(pod); err != nil {
					t.Fatal(err)
				}
				repo.onAddPod(pod)
				repo.onFactsPod(pod)
				if err := nodes.Update(node); err != nil {
					t.Fatal(err)
				}
				if stage == "old node cached" || stage == "registration added" {
					repo.onFactsNode(node)
				}
				if stage == "Filter reservation" {
					preHoldersListed, listErr := repo.ListAll(context.Background())
					if listErr != nil || len(preHoldersListed) != 1 || preHoldersListed[0].NodeName != "" || preHoldersListed[0].NodeUID != string(node.UID) {
						t.Fatalf("pre-holders ListAll() = (%+v, %v), want NodeName=%q NodeUID=%q", preHoldersListed, listErr, "", node.UID)
					}
				}
				got, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Containers) != 1 {
					t.Fatalf("late-bound reservation: got %d holders, reservations=%v, want one holder and 2048 MiB", len(got.Containers), got.Reservations)
				}
				card := got.Containers[0].ContainerDevices[0]
				if got.Reservations[card.UUID].MemoryMiB != 2048 || card.Shape != "soft" || card.CoreAllocationUnknown {
					t.Fatalf("got %+v reservation=%+v, want 2048 MiB with soft allocation and known cores", card, got.Reservations[card.UUID])
				}
				listed, err := repo.ListAll(context.Background())
				if err != nil || len(listed) != 1 || listed[0].NodeName != pod.Spec.NodeName || listed[0].NodeUID != string(node.UID) {
					t.Fatalf("ListAll() node attribution = (%+v, %v), want node=%q uid=%q", listed, err, pod.Spec.NodeName, node.UID)
				}
			})
		}
	}
}

func TestPreBindReplacementNodeNotAttributedInListAll(t *testing.T) {
	node, pod := coldAllocationObjects("NVIDIA")
	node.UID = "new-uid"
	node.CreationTimestamp = metav1.NewTime(time.Unix(2000, 0))
	pod.Spec.NodeName = ""
	pod.Status.Conditions = nil
	pod.CreationTimestamp = metav1.NewTime(time.Unix(1100, 0))
	pod.Annotations["hami.io/vgpu-node"] = node.Name
	pod.Annotations["hami.io/vgpu-time"] = "1200"

	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo, _ := startResourceTestSource(t, newResourceTestClient(), catalog, map[string]string{"NVIDIA": ""})
	nodes := repo.data.informers.Core().V1().Nodes().Informer().GetIndexer()
	if err := nodes.Add(node); err != nil {
		t.Fatal(err)
	}
	if err := repo.podIndexer.Add(pod); err != nil {
		t.Fatal(err)
	}
	repo.onAddPod(pod)
	repo.onFactsPod(pod)

	listed, err := repo.ListAll(context.Background())
	if err != nil || len(listed) != 1 || listed[0].NodeName != "" || listed[0].NodeUID != "" {
		t.Fatalf("pre-holders ListAll() on replacement node = (%+v, %v), want empty NodeName and NodeUID", listed, err)
	}
	got, err := repo.NodeHolders(context.Background(), node.Name, "new-uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Containers) != 0 || !got.UncertainDeviceIDs["GPU-a"] {
		t.Fatalf("NodeHolders() on replacement node = %+v, want 0 containers and UncertainDeviceIDs[GPU-a]=true", got)
	}
	listedAfter, err := repo.ListAll(context.Background())
	if err != nil || len(listedAfter) != 1 || listedAfter[0].NodeName != "" || listedAfter[0].NodeUID != "" {
		t.Fatalf("post-holders ListAll() on replacement node = (%+v, %v), want empty NodeName and NodeUID", listedAfter, err)
	}
}

// The legacy NVIDIA encoder omitted CPU positions. Compare both encodings
// through the holder API so a correct card total cannot hide the wrong owner.
func TestMixedPodDeviceHolder(t *testing.T) {
	for _, raw := range []string{"GPU-a,NVIDIA,2048,20:;", ";GPU-a,NVIDIA,2048,20:;"} {
		t.Run(raw, func(t *testing.T) {
			node, pod := coldAllocationObjects("NVIDIA")
			pod.Spec.Containers = append([]corev1.Container{{Name: "cpu"}}, pod.Spec.Containers...)
			pod.Annotations["hami.io/vgpu-devices-allocated"] = raw
			catalog := &mutableCatalog{current: loadedTestCatalog(t)}
			catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
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
			if len(h.Containers) != 2 || len(h.Containers[0].ContainerDevices) != 0 || len(h.Containers[1].ContainerDevices) != 1 || h.Reservations["GPU-a"].MemoryMiB != 2048 || len(h.UnreadableAllocations) != 0 {
				t.Fatalf("GPU allocation must belong only to app: %+v", h)
			}
		})
	}
}
