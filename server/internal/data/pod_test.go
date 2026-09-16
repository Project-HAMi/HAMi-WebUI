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

// The repo reads node-1, which carries no hami-vnpu-core annotation.
func ascendTestRepo(catalog *mutableCatalog) *podRepo {
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})
	repo := &podRepo{data: &Data{k8sCl: client}, pods: map[k8stypes.UID]*biz.PodInfo{}, log: log.NewHelper(log.NewStdLogger(io.Discard)), ascend: ascend.Decoder{Catalog: catalog}}
	catalog.Subscribe(repo.onCatalogChange)
	return repo
}

func TestAscendAllocationsFollowTheCurrentConfigurationWithoutRedecoding(t *testing.T) {
	catalog := &mutableCatalog{current: &devicecatalog.Snapshot{State: devicecatalog.StateForbidden}}
	repo := ascendTestRepo(catalog)
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

func TestNewlyConfiguredModelsAreDecodedAgain(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	repo := ascendTestRepo(catalog)
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
	repo := ascendTestRepo(catalog)
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
