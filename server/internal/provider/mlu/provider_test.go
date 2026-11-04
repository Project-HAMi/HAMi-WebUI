package mlu

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

// Fixtures follow HAMi v2.10.0 pkg/device/cambricon with the plugin's min-dsmlu-unit=256:
// a card publishes 100 vcore and cardMiB/256 vmemory, and PatchAnnotations writes Usedmem/256.
func hamiNode(name string, cards, cardMiB int64) *corev1.Node {
	return smluNode(name, "cambricon.com/mlu", cards, cardMiB)
}

func smluNode(name, resourceName string, cards, cardMiB int64) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Capacity: corev1.ResourceList{
			corev1.ResourceName(resourceName + ".smlu.vcore"):   *resource.NewQuantity(cards*100, resource.DecimalSI),
			corev1.ResourceName(resourceName + ".smlu.vmemory"): *resource.NewQuantity(cards*cardMiB/256, resource.DecimalSI),
		}},
	}
}

func TestCambriconCapacityFollowsPluginResourceNames(t *testing.T) {
	const nodeName = "mlu-node"
	withCapacity := func(node *corev1.Node, extra corev1.ResourceList) *corev1.Node {
		for name, quantity := range extra {
			node.Status.Capacity[name] = quantity
		}
		return node
	}
	for _, tc := range []struct {
		name string
		node *corev1.Node
		want int
	}{
		{name: "plugin default", node: smluNode(nodeName, "cambricon.com/mlu", 2, 24576), want: 2},
		{name: "device type on MLU370", node: smluNode(nodeName, "cambricon.com/mlu370", 2, 24576), want: 2},
		{name: "device type on MLU590", node: smluNode(nodeName, "cambricon.com/mlu590", 2, 24576), want: 2},
		{name: "device type on MLU270-X5K", node: smluNode(nodeName, "cambricon.com/mlu270-x5k", 2, 24576), want: 2},
		{
			name: "other resources are not capacity",
			node: withCapacity(smluNode(nodeName, "cambricon.com/mlu", 2, 24576), corev1.ResourceList{
				"cambricon.com/mlu":             *resource.NewQuantity(8, resource.DecimalSI),
				"cambricon.com/real-mlu-counts": *resource.NewQuantity(8, resource.DecimalSI),
				"example.com/mlu.smlu.vcore":    *resource.NewQuantity(800, resource.DecimalSI),
				"example.com/mlu.smlu.vmemory":  *resource.NewQuantity(768, resource.DecimalSI),
			}),
			want: 2,
		},
		{
			name: "stale name left at zero",
			node: withCapacity(smluNode(nodeName, "cambricon.com/mlu", 2, 24576), corev1.ResourceList{
				"cambricon.com/mlu370.smlu.vcore":   *resource.NewQuantity(0, resource.DecimalSI),
				"cambricon.com/mlu370.smlu.vmemory": *resource.NewQuantity(0, resource.DecimalSI),
			}),
			want: 2,
		},
		{
			name: "rename with only the new vcore registered",
			node: withCapacity(smluNode(nodeName, "cambricon.com/mlu370", 2, 24576), corev1.ResourceList{
				"cambricon.com/mlu.smlu.vcore": *resource.NewQuantity(200, resource.DecimalSI),
			}),
			want: 2,
		},
		{
			name: "rename with the old vcore already cleared",
			node: withCapacity(smluNode(nodeName, "cambricon.com/mlu", 2, 24576), corev1.ResourceList{
				"cambricon.com/mlu370.smlu.vcore":   *resource.NewQuantity(0, resource.DecimalSI),
				"cambricon.com/mlu370.smlu.vmemory": *resource.NewQuantity(192, resource.DecimalSI),
			}),
			want: 2,
		},
		{
			name: "rename with both names complete",
			node: withCapacity(smluNode(nodeName, "cambricon.com/mlu", 2, 24576), corev1.ResourceList{
				"cambricon.com/mlu370.smlu.vcore":   *resource.NewQuantity(200, resource.DecimalSI),
				"cambricon.com/mlu370.smlu.vmemory": *resource.NewQuantity(192, resource.DecimalSI),
			}),
			want: 2,
		},
		{name: "no sMLU resources", node: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := NewCambricon(healthyCards(t, nodeName, 2), log.NewHelper(log.DefaultLogger), "")
			devices, err := provider.FetchDevices(tc.node)
			if err != nil || len(devices) != tc.want {
				t.Fatalf("FetchDevices() = (%d devices, %v), want %d", len(devices), err, tc.want)
			}
			for _, device := range devices {
				if device.Devmem != 24576 {
					t.Fatalf("card %d memory = %d MiB, want 24576", device.Index, device.Devmem)
				}
			}
		})
	}
}

func healthyCards(t *testing.T, node string, cards int) *prom.Client {
	t.Helper()
	results := make([]string, 0, cards)
	for i := 0; i < cards; i++ {
		results = append(results, fmt.Sprintf(`{"metric":{"mlu":"%d","uuid":"MLU-%d","model":"MLU370-X8","driver":"5.10"},"value":[1,"1"]}`, i, i))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if got, want := r.Form.Get("query"), fmt.Sprintf(`mlu_health{node="%s"}`, node); got != want {
			t.Errorf("query = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[%s]}}`, strings.Join(results, ",")); err != nil {
			t.Errorf("write Prometheus response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := prom.NewClient(server.URL, time.Second, prom.HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestCambriconConflictingCapacityIsNotGuessed(t *testing.T) {
	const nodeName = "mlu-node"
	node := smluNode(nodeName, "cambricon.com/mlu", 2, 24576)
	node.Status.Capacity["cambricon.com/mlu370.smlu.vcore"] = *resource.NewQuantity(100, resource.DecimalSI)
	node.Status.Capacity["cambricon.com/mlu370.smlu.vmemory"] = *resource.NewQuantity(96, resource.DecimalSI)

	provider := NewCambricon(healthyCards(t, nodeName, 2), log.NewHelper(log.DefaultLogger), "")
	devices, err := provider.FetchDevices(node)
	if err == nil || devices != nil {
		t.Fatalf("FetchDevices() = (%d devices, %v), want a conflict error", len(devices), err)
	}
}

func TestCambriconAllocationAndCardMemoryShareHAMiUnit(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cards, cardMiB int
	}{
		{name: "one 24 GiB card", cards: 1, cardMiB: 24576},
		{name: "eight 48 GiB cards", cards: 8, cardMiB: 49152},
		{name: "four 80 GiB cards", cards: 4, cardMiB: 81920},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const nodeName = "mlu-node"
			provider := NewCambricon(healthyCards(t, nodeName, tc.cards), log.NewHelper(log.DefaultLogger), "")
			devices, err := provider.FetchDevices(hamiNode(nodeName, int64(tc.cards), int64(tc.cardMiB)))
			if err != nil || len(devices) != tc.cards {
				t.Fatalf("FetchDevices() = (%d devices, %v), want %d", len(devices), err, tc.cards)
			}
			for _, d := range devices {
				if d.Devmem != int32(tc.cardMiB) {
					t.Fatalf("card %d memory = %d MiB, want %d", d.Index, d.Devmem, tc.cardMiB)
				}
			}

			slot := tc.cards - 1
			card := devices[slot]
			for _, usedMiB := range []int{256, 4096, 8192, tc.cardMiB} {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
						util.AssignedNodeAnnotations:            nodeName,
						util.SupportDevices[CambriconMLUDevice]: fmt.Sprintf("%s,MLU,%d,20:;", card.AliasId, usedMiB),
					}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
				}
				allocations, failures := util.DecodePodDevices(pod, log.NewHelper(log.DefaultLogger))
				if len(failures) != 0 {
					t.Fatal(failures)
				}
				ctrs := allocations[CambriconMLUDevice]
				if len(ctrs) != 1 || len(ctrs[0]) != 1 {
					t.Fatalf("allocation = %+v, want one device", ctrs)
				}
				got := ctrs[0][0]
				if got.UUID != card.AliasId || got.Usedmem != int32(usedMiB) {
					t.Fatalf("allocation = %+v, want %d MiB on %s", got, usedMiB, card.AliasId)
				}
				// A whole-card allocation must book exactly the card's memory.
				if usedMiB == tc.cardMiB && got.Usedmem != card.Devmem {
					t.Fatalf("whole-card allocation = %d MiB, card memory = %d MiB", got.Usedmem, card.Devmem)
				}
			}
		})
	}
}
