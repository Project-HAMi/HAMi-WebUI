package data

import (
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"vgpu/internal/biz"
	"vgpu/internal/provider/mthreads"
)

// failingNodeLister reports every lookup as a cache miss, forcing the live
// API fallback path.
type failingNodeLister struct{}

func (failingNodeLister) List(_ labels.Selector) ([]*corev1.Node, error) { return nil, nil }
func (failingNodeLister) Get(_ string) (*corev1.Node, error) {
	return nil, apierrors.NewNotFound(corev1.Resource("nodes"), "")
}

type staticNodeLister struct{ nodes map[string]*corev1.Node }

func (s staticNodeLister) List(_ labels.Selector) ([]*corev1.Node, error) {
	out := make([]*corev1.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (s staticNodeLister) Get(name string) (*corev1.Node, error) {
	if n, ok := s.nodes[name]; ok {
		return n, nil
	}
	return nil, apierrors.NewNotFound(corev1.Resource("nodes"), name)
}

func wholeGPUEntry(uid k8stypes.UID, node, ns, name string, cards map[string][]int64) *wholeGPUPod {
	ctrs := make([]wholeGPUContainer, 0, len(cards))
	statuses := make([]corev1.ContainerStatus, 0, len(cards))
	for cname, ids := range cards {
		ctrs = append(ctrs, wholeGPUContainer{name: cname, count: int64(len(ids)), cards: ids})
		statuses = append(statuses, corev1.ContainerStatus{
			Name:  cname,
			Ready: true,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		})
	}
	return &wholeGPUPod{
		pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid},
			Spec:       corev1.PodSpec{NodeName: node},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: statuses,
			},
		},
		nodeID:   node,
		nodeUID:  "nuid-" + node,
		nodeName: node,
		memMiB:   81920,
		ctrs:     ctrs,
	}
}

// wholeGPUPodSpec builds a pod that actually requests vendor whole cards.
func wholeGPUPodSpec(uid k8stypes.UID, node, ns, name, ctrName string, count int64) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid},
		Spec: corev1.PodSpec{
			NodeName: node,
			Containers: []corev1.Container{{
				Name: ctrName,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceName(mthreads.NodeWholeGPUResource): *resource.NewQuantity(count, resource.DecimalSI),
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  ctrName,
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}

func devicesByContainer(containers []*biz.Container) map[string][]string {
	out := map[string][]string{}
	for _, c := range containers {
		out[string(c.PodUID)+"/"+c.Name] = nil
		for _, d := range c.ContainerDevices {
			out[string(c.PodUID)+"/"+c.Name] = append(out[string(c.PodUID)+"/"+c.Name], d.UUID)
		}
	}
	return out
}

// Consumers resolve a synthesized device UUID back to a physical card, so a
// pod's cards must not change when an unrelated pod appears or when pods that
// sort earlier are added.
func TestListWholeGPUContainersKeepsCardIDsStableAcrossPodChanges(t *testing.T) {
	first := wholeGPUEntry("uid-a", "node-a", "ns", "b-pod", map[string][]int64{"main": {1}})
	second := wholeGPUEntry("uid-b", "node-a", "ns", "c-pod", map[string][]int64{"main": {0}})

	repo := &podRepo{wholeGPUPods: map[k8stypes.UID]*wholeGPUPod{
		"uid-a": first, "uid-b": second,
	}}
	before := devicesByContainer(repo.listWholeGPUContainers())

	// A pod that sorts before both existing pods is added; the retained ids
	// must keep every pre-existing container on the same card.
	repo.wholeGPUPods["uid-0"] = wholeGPUEntry("uid-0", "node-a", "aaa", "a-pod", map[string][]int64{"main": {2}})
	after := devicesByContainer(repo.listWholeGPUContainers())

	if len(before) != 2 || len(after) != 3 {
		t.Fatalf("containers before=%d after=%d, want 2 then 3", len(before), len(after))
	}
	for name, ids := range before {
		got := after[name]
		if len(got) != len(ids) {
			t.Fatalf("%s devices = %v, want %v", name, got, ids)
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Fatalf("%s devices changed from %v to %v", name, ids, got)
			}
		}
	}
}

// Same-name pods in different namespaces must receive distinct, stable slots.
func TestListWholeGPUContainersOrdersSameNamePodsDeterministically(t *testing.T) {
	repo := &podRepo{wholeGPUPods: map[k8stypes.UID]*wholeGPUPod{
		"uid-1": wholeGPUEntry("uid-1", "node-a", "ns-b", "same", map[string][]int64{"main": {0}}),
		"uid-2": wholeGPUEntry("uid-2", "node-a", "ns-a", "same", map[string][]int64{"main": {1}}),
	}}

	var first []string
	for range 20 {
		got := repo.listWholeGPUContainers()
		if len(got) != 2 {
			t.Fatalf("containers = %d, want 2", len(got))
		}
		order := []string{got[0].Namespace, got[1].Namespace}
		if first == nil {
			first = order
			continue
		}
		if order[0] != first[0] || order[1] != first[1] {
			t.Fatalf("order changed between calls: %v then %v", first, order)
		}
	}
	if first[0] != "ns-a" {
		t.Fatalf("order = %v, want ns-a first", first)
	}
}

// The node informer can miss while the API read also fails; the ledger must
// not gain an entry with a nil node, which would panic in ListAll.
func TestRefreshWholeGPUPodSkipsLedgerWhenNodeUnresolved(t *testing.T) {
	repo := &podRepo{
		data:         &Data{k8sCl: fake.NewSimpleClientset()},
		log:          log.NewHelper(log.NewStdLogger(io.Discard)),
		wholeGPUPods: make(map[k8stypes.UID]*wholeGPUPod),
		nodeLister:   failingNodeLister{},
	}
	pod := wholeGPUPodSpec("uid-x", "missing-node", "ns", "p", "main", 1)

	if !repo.refreshWholeGPUPod(pod) {
		t.Fatal("refreshWholeGPUPod = false, want true (whole-GPU pod)")
	}
	repo.mutex.RLock()
	defer repo.mutex.RUnlock()
	if _, ok := repo.wholeGPUPods[pod.UID]; ok {
		t.Fatal("ledger stored an entry for an unresolved node")
	}
	// ListAll must stay safe even with an empty ledger.
	if _, err := repo.ListAll(t.Context()); err != nil {
		t.Fatalf("ListAll: %v", err)
	}
}

// Once the node resolves, the pod is tracked and its cards are retained.
func TestRefreshWholeGPUPodStoresResolvedNodeContext(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-a", UID: "nuid-a",
		Labels: map[string]string{mthreads.GPUCountLabel: "2"},
	}}
	repo := &podRepo{
		data:         &Data{k8sCl: fake.NewSimpleClientset()},
		log:          log.NewHelper(log.NewStdLogger(io.Discard)),
		wholeGPUPods: make(map[k8stypes.UID]*wholeGPUPod),
		nodeLister:   staticNodeLister{nodes: map[string]*corev1.Node{"node-a": node}},
	}
	pod := wholeGPUPodSpec("uid-y", "node-a", "ns", "p", "main", 1)

	if !repo.refreshWholeGPUPod(pod) {
		t.Fatal("refreshWholeGPUPod = false, want true")
	}
	repo.mutex.RLock()
	entry := repo.wholeGPUPods[pod.UID]
	repo.mutex.RUnlock()
	if entry == nil {
		t.Fatal("ledger entry missing for a resolved node")
	}
	if entry.nodeUID != "nuid-a" {
		t.Fatalf("nodeUID = %q, want nuid-a", entry.nodeUID)
	}
	if len(entry.ctrs) != 1 || len(entry.ctrs[0].cards) != 1 || entry.ctrs[0].cards[0] != 0 {
		t.Fatalf("cards = %+v, want container main holding card 0", entry.ctrs)
	}
}

// Removing a pod releases its card ids, so the next pod reclaims the lowest
// free ordinal instead of being pushed onto a synthetic overcommit id.
func TestRemoveWholeGPUPodReleasesCardIDs(t *testing.T) {
	repo := &podRepo{wholeGPUPods: map[k8stypes.UID]*wholeGPUPod{
		"uid-1": wholeGPUEntry("uid-1", "node-a", "ns", "p1", map[string][]int64{"main": {0}}),
	}}
	repo.removeWholeGPUPod("uid-1")
	if len(repo.wholeGPUPods) != 0 {
		t.Fatalf("ledger = %d entries, want 0 after removal", len(repo.wholeGPUPods))
	}

	counts := []wholeGPUContainer{{name: "main", count: 1}}
	repo.assignWholeCardIDs("uid-2", "node-a", []int64{0, 1}, counts)
	if got := counts[0].cards; len(got) != 1 || got[0] != 0 {
		t.Fatalf("cards = %v, want [0] reclaimed from the removed pod", got)
	}
}
