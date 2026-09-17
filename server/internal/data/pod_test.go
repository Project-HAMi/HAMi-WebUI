package data

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"

	"vgpu/internal/biz"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

func TestMergeContainerDevicesBySlotKeepsInitAlignmentAndDeviceTypes(t *testing.T) {
	podDevices := biz.PodDevices{
		"Ascend910B3": {{}, {{UUID: "B3-0", Type: "Ascend910B3", Usedmem: 28672, Usedcores: 40}}, {}},
		"NVIDIA":      {{}, {}, {{UUID: "GPU-0", Type: "NVIDIA", Usedmem: 4096, Usedcores: 20}}},
	}
	got := mergeContainerDevicesBySlot(3, podDevices)
	if len(got) != 3 || len(got[0]) != 0 || len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("merged slots = %#v", got)
	}
	if got[1][0].Type != "Ascend910B3" || got[2][0].Type != "NVIDIA" {
		t.Fatalf("device types moved or overwrote one another: %#v", got)
	}
}

type mutableCatalog struct {
	mu          sync.Mutex
	current     *devicecatalog.Snapshot
	subscribers []func(previous, current *devicecatalog.Snapshot)
}

func (m *mutableCatalog) Snapshot() *devicecatalog.Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *mutableCatalog) Subscribe(fn func(previous, current *devicecatalog.Snapshot)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers = append(m.subscribers, fn)
}

func (m *mutableCatalog) set(next *devicecatalog.Snapshot) {
	m.mu.Lock()
	previous := m.current
	m.current = next
	subscribers := m.subscribers
	m.mu.Unlock()
	for _, fn := range subscribers {
		fn(previous, next)
	}
}

func loadedTestCatalog(t *testing.T, extra ...devicecatalog.AscendModel) *devicecatalog.Snapshot {
	t.Helper()
	content, err := os.ReadFile("../devicecatalog/testdata/hami-v2.10.0.yaml")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := devicecatalog.Parse(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range extra {
		parsed.Ascend[model.CommonWord] = model
	}
	return &devicecatalog.Snapshot{State: devicecatalog.StateLoaded, Ascend: parsed.Ascend}
}

// The repo reads its nodes from the cache; node-1 carries no hami-vnpu-core annotation.
func ascendTestRepo(t *testing.T, catalog *mutableCatalog, nodes ...*corev1.Node) *podRepo {
	t.Helper()
	if len(nodes) == 0 {
		nodes = []*corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}}
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, node := range nodes {
		if err := indexer.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	repo := &podRepo{
		data:       &Data{k8sCl: fake.NewSimpleClientset()},
		nodeLister: listerscorev1.NewNodeLister(indexer),
		pods:       map[k8stypes.UID]*biz.PodInfo{},
		log:        log.NewHelper(log.NewStdLogger(io.Discard)),
		ascend:     ascend.Decoder{Catalog: catalog},
	}
	catalog.Subscribe(repo.onCatalogChange)
	return repo
}

func TestPodEventsReadTheirNodeFromTheCache(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo := ascendTestRepo(t, catalog, &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:        "node-1",
		Annotations: map[string]string{ascend.NodeHamiCoreAnnotation: "true"},
	}})
	repo.onAddPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "train", UID: "pod-1", Annotations: map[string]string{
			util.AssignedNodeAnnotations:            "node-1",
			"hami.io/Ascend910B3-devices-allocated": "B3-0,Ascend910B3,16384,0:",
		}},
		Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "worker"}}},
	})

	containers, _ := repo.ListAll(context.Background())
	device := containers[0].ContainerDevices[0]
	// The node's annotation was read: without the node the reason is node_mode_unknown.
	if device.CoreReason != ascend.ReasonModeAmbiguous {
		t.Fatalf("the node was not read from the cache: %+v", device)
	}
	if actions := repo.data.k8sCl.(*fake.Clientset).Actions(); len(actions) != 0 {
		t.Fatalf("a Pod event reached the API server: %v", actions)
	}
}

func TestAscendAllocationsFollowTheCurrentConfigurationWithoutRedecoding(t *testing.T) {
	catalog := &mutableCatalog{current: &devicecatalog.Snapshot{State: devicecatalog.StateForbidden}}
	repo := ascendTestRepo(t, catalog)
	repo.onAddPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "train", UID: "pod-1", Annotations: map[string]string{
			util.AssignedNodeAnnotations:            "node-1",
			ascend.VNPUModeAnnotation:               "template",
			"hami.io/Ascend910B3-devices-allocated": "B3-0,Ascend910B3,16384,0:",
		}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
	})
	containers, _ := repo.ListAll(context.Background())
	device := containers[0].ContainerDevices[0]
	if !device.CoreAllocationUnknown || device.CoreReason != ascend.ReasonCatalogUnavailable || device.Vendor != biz.AscendGPUDevice || device.Usedmem != 16384 {
		t.Fatalf("before the configuration is readable: %+v", device)
	}
	if _, _, _, known := biz.ContainersStatisticsInfo(containers, ""); known {
		t.Fatal("device totals claimed a known compute share")
	}

	catalog.set(loadedTestCatalog(t))
	containers, _ = repo.ListAll(context.Background())
	device = containers[0].ContainerDevices[0]
	if device.CoreAllocationUnknown || device.Usedcores != 25 || device.Shape != ascend.ShapeTemplate || device.Template != "vir05_1c_16g" {
		t.Fatalf("after the configuration is read: %+v", device)
	}
	stored, _ := repo.FindOne(context.Background(), "pod-1", "worker")
	if stored.ContainerDevices[0].Usedcores != 25 || repo.pods["pod-1"].Ctrs[0].ContainerDevices[0].Shape != "" {
		t.Fatal("interpretation was written back into the shared inventory")
	}
}

func TestPodEventsReadANodeTheCacheHasNotSeenYet(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo := ascendTestRepo(t, catalog, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "other-node"}})
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:        "node-1",
		UID:         "node-uid-1",
		Annotations: map[string]string{ascend.NodeHamiCoreAnnotation: "true"},
	}})
	repo.data = &Data{k8sCl: client}
	repo.onAddPod(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "train", UID: "pod-1", Annotations: map[string]string{
			util.AssignedNodeAnnotations:            "node-1",
			"hami.io/Ascend910B3-devices-allocated": "B3-0,Ascend910B3,16384,0:",
		}},
		Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "worker"}}},
	})

	containers, _ := repo.ListAll(context.Background())
	if containers[0].NodeUID != "node-uid-1" || containers[0].ContainerDevices[0].CoreReason != ascend.ReasonModeAmbiguous {
		t.Fatalf("the node was not read from the API: %+v", containers[0])
	}
	if actions := client.Actions(); len(actions) != 1 || actions[0].GetVerb() != "get" {
		t.Fatalf("expected one node read, got %v", actions)
	}
}

func TestNewlyConfiguredModelsAreDecodedAgain(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo := ascendTestRepo(t, catalog)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "research", Name: "custom", UID: "pod-2", Annotations: map[string]string{
			util.AssignedNodeAnnotations:          "node-1",
			"hami.io/Custom910-devices-allocated": "X-0,Custom910,1000,0:",
		}},
		Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "worker"}}},
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	if err := indexer.Add(pod); err != nil {
		t.Fatal(err)
	}
	repo.podLister = listerscorev1.NewPodLister(indexer)
	repo.onAddPod(pod)
	if containers, _ := repo.ListAll(context.Background()); len(containers) != 0 {
		t.Fatalf("an unconfigured, unprefixed model was decoded: %+v", containers)
	}
	catalog.set(loadedTestCatalog(t, devicecatalog.AscendModel{CommonWord: "Custom910", MemoryAllocatable: 1000}))
	containers, _ := repo.ListAll(context.Background())
	if len(containers) != 1 || len(containers[0].ContainerDevices) != 1 || containers[0].ContainerDevices[0].Shape != ascend.ShapeWhole {
		t.Fatalf("after configuring the model: %+v", containers)
	}
}

func TestReadsCopyOnlyContainersWithAscendDevices(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo := ascendTestRepo(t, catalog)
	nvidiaOnly := &biz.Container{Name: "worker", ContainerDevices: biz.ContainerDevices{{UUID: "GPU-0", Type: "NVIDIA"}}}
	if got := repo.interpretContainer(catalog.Snapshot(), nvidiaOnly); got != nvidiaOnly {
		t.Fatal("a container without Ascend devices was copied on read")
	}
	ascendOne := &biz.Container{Name: "worker", ContainerDevices: biz.ContainerDevices{
		{UUID: "B3-0", Type: "Ascend910B3", Usedmem: 16384, Ascend: &biz.AscendFacts{Mode: ascend.ModeTemplate, Recorded: true, Template: "vir05_1c_16g"}},
	}}
	got := repo.interpretContainer(catalog.Snapshot(), ascendOne)
	if got == ascendOne || got.ContainerDevices[0].Usedcores != 25 || ascendOne.ContainerDevices[0].Usedcores != 0 {
		t.Fatalf("interpreted = %+v, stored = %+v", got.ContainerDevices[0], ascendOne.ContainerDevices[0])
	}
}
func TestSplitModeIsReportedForBothVendors(t *testing.T) {
	// The allocation vocabulary is shared, so the API never mixes two spellings.
	if ascend.ShapeSoft != biz.SplitShapeSoft || ascend.ShapeTemplate != biz.SplitShapeTemplate ||
		ascend.ShapeWhole != biz.SplitShapeWhole || ascend.ShapeUnknown != biz.SplitShapeUnknown {
		t.Fatal("the Ascend provider and the API disagree on shape names")
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "train", UID: "pod-3", Annotations: map[string]string{
			util.AssignedNodeAnnotations:     "node-1",
			"hami.io/vgpu-devices-allocated": "GPU-0,NVIDIA,4096,30:;GPU-1,NVIDIA,40960,100:",
			nvidia.MigAllocationsAnnotation:  `[{"containerIndex":1,"deviceIndex":0,"gpuUUID":"GPU-1","profile":"3g.40gb","placement":{"start":0,"size":4}}]`,
		}},
		Spec: corev1.PodSpec{NodeName: "node-1", InitContainers: []corev1.Container{{Name: "prepare"}}, Containers: []corev1.Container{{Name: "worker"}}},
	}
	modes := map[string]string{"GPU-0": nvidia.ModeHamiCore, "GPU-1": nvidia.ModeMig}
	devices := bizPodDevices(nvidia.ReadAllocations(pod, modes), mustDecodePodDevices(t, pod))
	slots := devices[biz.NvidiaGPUDevice]
	if len(slots) != 2 {
		t.Fatalf("slots = %v", slots)
	}
	if got := slots[0][0]; got.Vendor != biz.NvidiaGPUDevice || got.Shape != biz.SplitShapeSoft || got.Template != "" {
		t.Fatalf("hami-core device = %+v", got)
	}
	if got := slots[1][0]; got.Shape != biz.SplitShapeMig || got.Template != "3g.40gb" || got.MigPlacement != (biz.MigPlacement{Start: 0, Size: 4}) {
		t.Fatalf("MIG device = %+v", got)
	}
}

func TestMigReservationsStayOnNVIDIAGPUs(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed", UID: "pod-4", Annotations: map[string]string{
			util.AssignedNodeAnnotations:     "node-1",
			"hami.io/vgpu-devices-allocated": "GPU-1[0-3],NVIDIA,10240,14:",
			"hami.io/dcu-devices-allocated":  "DCU-1,DCU,8192,50:",
			nvidia.MigAllocationsAnnotation:  `[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-1","profile":"1g.10gb","placement":{"start":3,"size":1}}]`,
		}},
		Spec: corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "worker"}}},
	}
	devices := bizPodDevices(nvidia.ReadAllocations(pod, map[string]string{"GPU-1": nvidia.ModeMig}), mustDecodePodDevices(t, pod))
	// A UUID recorded before HAMi v2.10 names its GPU once the suffix is removed.
	if got := devices[biz.NvidiaGPUDevice][0][0]; got.UUID != "GPU-1" || got.Shape != biz.SplitShapeMig || got.Template != "" {
		t.Fatalf("NVIDIA device = %+v", got)
	}
	if got := devices["DCU"][0][0]; got.Shape != "" || got.Template != "" || got.MigPlacement != (biz.MigPlacement{}) {
		t.Fatalf("DCU device took the NVIDIA reservation: %+v", got)
	}
}

func mustDecodePodDevices(t *testing.T, pod *corev1.Pod) util.PodDevices {
	t.Helper()
	decoded, err := util.DecodePodDevices(pod, log.NewHelper(log.NewStdLogger(io.Discard)))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}
