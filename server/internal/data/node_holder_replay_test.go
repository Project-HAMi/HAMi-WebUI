package data

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
				got, err := repo.NodeHolders(context.Background(), node.Name, string(node.UID))
				if err != nil {
					t.Fatal(err)
				}
				if len(got.Containers) != 1 {
					t.Fatalf("late-bound reservation: got %d holders, want one holder and 2048 MiB", len(got.Containers))
				}
				card := got.Containers[0].ContainerDevices[0]
				if card.Usedmem != 2048 || card.Shape != "soft" || card.CoreAllocationUnknown {
					t.Fatalf("got %+v, want 2048 MiB with soft allocation and known cores", card)
				}
			})
		}
	}
}
