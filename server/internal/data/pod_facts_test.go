package data

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

func factTestPod() *corev1.Pod {
	always := corev1.ContainerRestartPolicyAlways
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "work", Name: "worker", UID: "uid-1", ResourceVersion: "1", CreationTimestamp: metav1.NewTime(time.Unix(100, 0)), Annotations: map[string]string{util.AssignedNodeAnnotations: "node-a", util.SupportDevices[util.NvidiaGPUDevice]: "GPU-a,NVIDIA,2,20:;GPU-a,NVIDIA,3,30:;GPU-a,NVIDIA,1,10:;"}},
		Spec:       corev1.PodSpec{NodeName: "node-a", InitContainers: []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "init"}}, Containers: []corev1.Container{{Name: "app"}}, EphemeralContainers: []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug"}, TargetContainerName: "app"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}
func factRepo(t *testing.T, catalog *mutableCatalog, pod *corev1.Pod) *podRepo {
	t.Helper()
	r := ascendTestRepo(t, catalog, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1"}})
	r.facts = podfacts.NewLedger()
	r.factsCatalog = catalog
	r.podIndexer = cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	r.podLister = listers.NewPodLister(r.podIndexer)
	if pod != nil {
		if err := r.podIndexer.Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	return r
}
func factEntry(t *testing.T, r *podRepo, pod *corev1.Pod) podfacts.Entry {
	t.Helper()
	r.onFactsPod(pod)
	entry, ok := r.facts.Get(pod.UID)
	if !ok {
		t.Fatal("Pod not recorded")
	}
	return entry
}
func TestFactAdapterOccupancyAndSourceBounds(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	p := factTestPod()
	r := factRepo(t, catalog, p)
	entry := factEntry(t, r, p)
	if len(entry.Facts.Containers[0].Allocations) != 1 {
		t.Fatal("known annotation duplicated as unsupported source")
	}
	if len(entry.Facts.Containers) != 4 || entry.Facts.Candidate != podfacts.CandidateAccelerator || len(entry.Occupancies) != 1 {
		t.Fatalf("lost member/assignment: %+v", entry)
	}
	q := entry.Occupancies[0].Quantities["memory"]
	if !entry.Occupancies[0].Complete || q.Unit != "bytes" || q.Value.Value() != 5*1024*1024 || entry.Occupancies[0].Node != "node-a" {
		t.Fatalf("wrong source/peak/unit: %+v", entry.Occupancies)
	}
	if len(entry.Facts.Containers[3].Allocations) != 0 {
		t.Fatal("ephemeral inherited allocation")
	}
	if entry.Facts.Containers[0].Allocations[0].Records[0].AllocationMode != nil {
		t.Fatal("inferred mode without node registration")
	}
	// A source-local complete stream does not make overall allocation/inventory complete.
	if entry.Facts.Containers[0].States["allocations"].State != resourceview.Unknown || r.facts.Snapshot().Coverage.Complete {
		t.Fatal("claimed global completeness")
	}
	if len(r.data.k8sCl.(*fake.Clientset).Actions()) != 0 {
		t.Fatal("facts made a direct API call")
	}
	for _, tc := range []struct {
		name, raw     string
		wantRecords   bool
		wantCandidate podfacts.Candidate
	}{
		{"truncated", "GPU-a,NVIDIA,2,20:", true, podfacts.CandidateAccelerator},
		{"malformed", "not-a-device", false, podfacts.CandidateUnresolved},
		{"extra slot", "GPU-a,NVIDIA,2,20:;;;;", false, podfacts.CandidateUnresolved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := factTestPod()
			p.Annotations[util.SupportDevices[util.NvidiaGPUDevice]] = tc.raw
			r := factRepo(t, catalog, p)
			entry := factEntry(t, r, p)
			if entry.Facts.Candidate != tc.wantCandidate {
				t.Fatalf("candidate = %s", entry.Facts.Candidate)
			}
			set := entry.Facts.Containers[0].Allocations[0]
			if set.Complete || (len(set.Records) > 0) != tc.wantRecords {
				t.Fatalf("bad source completeness: %+v", set)
			}
			for _, row := range entry.Occupancies {
				if row.Complete || row.Quantities["memory"].Value != nil {
					t.Fatal("untrusted slot stream counted as zero/known")
				}
			}
		})
	}
}
func TestFactAdapterDoesNotInferAllocationFromRequestOrBinding(t *testing.T) {
	p := factTestPod()
	p.Annotations = nil
	r := factRepo(t, &mutableCatalog{current: loadedTestCatalog(t)}, p)
	entry := factEntry(t, r, p)
	if len(entry.Occupancies) != 0 || len(entry.Facts.Containers[0].Allocations) != 0 {
		t.Fatal("binding became allocation")
	}
	p.Annotations = map[string]string{util.SupportDevices[util.NvidiaGPUDevice]: "GPU-a,NVIDIA,1,10:;;", "nvidia.com/vgpu-mode": "hami-core"}
	_ = r.podIndexer.Update(p)
	entry = factEntry(t, r, p)
	if len(entry.Occupancies) != 0 || entry.Facts.Candidate != podfacts.CandidateUnresolved {
		t.Fatal("missing assignment context accepted")
	}
	p.Annotations[util.AssignedNodeAnnotations] = "node-a"
	_ = r.podIndexer.Update(p)
	entry = factEntry(t, r, p)
	if record := entry.Facts.Containers[0].Allocations[0].Records[0]; record.RawMode != "hami-core" || record.AllocationMode != nil {
		t.Fatal("requested mode became actual mode")
	}
}
func TestFactAdapterStaleReplayReplacementAndTerminal(t *testing.T) {
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	p := factTestPod()
	r := factRepo(t, catalog, p)
	factEntry(t, r, p)
	newer := p.DeepCopy()
	newer.ResourceVersion = "2"
	newer.Status.Phase = corev1.PodSucceeded
	_ = r.podIndexer.Update(newer)
	r.onFactsPod(newer)
	r.onFactsPod(p)
	entry, _ := r.facts.Get(p.UID)
	if !entry.Facts.Terminal || len(entry.Facts.Containers) != 4 || len(entry.Occupancies) != 0 {
		t.Fatal("stale replay restored terminal occupancy")
	}
	replacement := p.DeepCopy()
	replacement.UID = "uid-2"
	replacement.ResourceVersion = "3"
	replacement.Annotations = nil
	_ = r.podIndexer.Update(replacement)
	r.onFactsPod(replacement)
	r.onFactsPod(p)
	r.onFactsDelete(cache.DeletedFinalStateUnknown{Obj: p})
	if _, exists := r.facts.Get(p.UID); exists {
		t.Fatal("recreated UID inherited old record")
	}
	if entry, exists := r.facts.Get(replacement.UID); !exists || len(entry.Occupancies) != 0 {
		t.Fatal("old delete removed replacement or inherited occupancy")
	}
	_ = r.podIndexer.Delete(replacement)
	r.onFactsDelete(replacement)
	r.onFactsPod(replacement)
	if len(r.facts.Snapshot().Entries) != 0 {
		t.Fatal("replay revived deleted Pod")
	}
}
func TestFactProductionUsesSharedInformerAndTracksCPU(t *testing.T) {
	p := factTestPod()
	cpu := p.DeepCopy()
	cpu.Name = "cpu"
	cpu.UID = "cpu-uid"
	cpu.Annotations = nil
	client := fake.NewSimpleClientset(p, cpu, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1"}})
	stop := make(chan struct{})
	defer close(stop)
	data := &Data{k8sCl: client, informers: informers.NewSharedInformerFactory(client, 0), stopCh: stop}
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	r, err := NewPodRepo(data, log.NewStdLogger(io.Discard), &conf.Bootstrap{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	eventually := func(check func() bool) {
		t.Helper()
		if err := wait.PollUntilContextTimeout(context.Background(), time.Millisecond*10, time.Second*3, true, func(context.Context) (bool, error) { return check(), nil }); err != nil {
			t.Fatal(err)
		}
	}
	eventually(func() bool { return len(NewPodFactsLedger(r).Snapshot().Entries) == 2 })
	eventually(func() bool { old, err := r.ListAll(context.Background()); return err == nil && len(old) == 1 })

	lists, watches := 0, 0
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "pods" {
			if action.GetVerb() == "list" {
				lists++
			}
			if action.GetVerb() == "watch" {
				watches++
			}
		}
	}
	if lists != 1 || watches != 1 {
		t.Fatalf("duplicate Pod source: lists=%d watches=%d", lists, watches)
	}
	cpu.ResourceVersion = "2"
	cpu.Status.Phase = corev1.PodSucceeded
	if _, err := client.CoreV1().Pods(cpu.Namespace).Update(context.Background(), cpu, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(func() bool { entry, ok := r.facts.Get(cpu.UID); return ok && entry.Facts.Terminal })
	if err := client.CoreV1().Pods(cpu.Namespace).Delete(context.Background(), cpu.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(func() bool { _, ok := r.facts.Get(cpu.UID); return !ok })
	// A newly configured annotation key is replayed through the same cached UID/RV.
	custom := p.DeepCopy()
	custom.ResourceVersion = "2"
	custom.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a", "hami.io/PrivateNPU-devices-allocated": "NPU-0,PrivateNPU,8,10:;;"}
	if _, err := client.CoreV1().Pods(custom.Namespace).Update(context.Background(), custom, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	eventually(func() bool {
		entry, ok := r.facts.Get(custom.UID)
		return ok && entry.Facts.Metadata.ResourceVersion == "2"
	})
	catalog.set(loadedTestCatalog(t, devicecatalog.AscendModel{CommonWord: "PrivateNPU"}))
	eventually(func() bool {
		entry, ok := r.facts.Get(custom.UID)
		return ok && entry.Facts.Candidate == podfacts.CandidateAccelerator
	})
}
func TestFactAdapterMLUMemoryUnitAndUnknownMetaXAssociation(t *testing.T) {
	for _, tc := range []struct {
		kind, key, device string
		complete          bool
	}{{"MLU", "hami.io/cambricon-mlu-devices-allocated", "node-a-cambricon-mlu-0", true}, {"Metax-SGPU", "hami.io/metax-sgpu-devices-allocated", "raw-device", false}} {
		t.Run(tc.kind, func(t *testing.T) {
			p := factTestPod()
			p.Spec.InitContainers = nil
			p.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a", tc.key: tc.device + "," + tc.kind + ",2048,50:;"}
			r := factRepo(t, &mutableCatalog{current: loadedTestCatalog(t)}, p)
			entry := factEntry(t, r, p)
			if len(entry.Occupancies) != 1 || entry.Occupancies[0].Complete != tc.complete {
				t.Fatalf("wrong association: %+v", entry)
			}
			record := entry.Facts.Containers[0].Allocations[0].Records[0]
			if record.Quantities["memory"].Value.Value() != 2048*1024*1024 || record.RawDeviceID != tc.device {
				t.Fatal("lost raw identity or converted node units twice")
			}
		})
	}
}
func TestFactAdapterAscendModesAndExactShares(t *testing.T) {
	for _, tc := range []struct {
		name, mode, template, memory string
		total, cores                 int32
		wantMode, wantCompute        string
	}{
		{"ENPU stays unknown", "ENPU", "", "8", 3, 1, "", ""},
		{"blank recorded is not whole", "template", "", "8", 3, 1, "", ""},
		{"named exact fraction", "template", "vir", "8", 32, 1, "template", "3.125"},
		{"named repeating fraction", "template", "vir", "8", 3, 1, "template", ""},
		{"whole capacity", "template", "", "64", 32, 1, "whole", "100"},
		{"soft percentage", "hami-core", "", "8", 32, 1, "soft", "10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := loadedTestCatalog(t, devicecatalog.AscendModel{CommonWord: "AscendTest", MemoryAllocatable: 64, AICore: tc.total, Templates: []devicecatalog.AscendTemplate{{Name: "vir", Memory: 8, AICore: tc.cores}}})
			p := factTestPod()
			p.Spec.InitContainers = nil
			p.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a", "hami.io/AscendTest-devices-allocated": "NPU-0,AscendTest," + tc.memory + ",10:;", ascend.VNPUModeAnnotation: tc.mode, "huawei.com/AscendTest": `[{"UUID":"NPU-0","temp":"` + tc.template + `","memory":` + tc.memory + `}]`}
			r := factRepo(t, &mutableCatalog{current: catalog}, p)
			entry := factEntry(t, r, p)
			record := entry.Facts.Containers[0].Allocations[0].Records[0]
			mode := ""
			if record.AllocationMode != nil {
				mode = *record.AllocationMode
			}
			if mode != tc.wantMode || record.RawMode != tc.mode {
				t.Fatalf("mode=%q raw=%q", mode, record.RawMode)
			}
			q := record.Quantities["compute"]
			if tc.wantCompute == "" {
				if q.Value != nil || q.State.State != resourceview.Unknown {
					t.Fatalf("unknown became a number: %+v", q)
				}
			} else if q.Value == nil || q.Value.Cmp(resource.MustParse(tc.wantCompute)) != 0 {
				t.Fatalf("compute=%+v want=%s", q, tc.wantCompute)
			}
			if tc.template != "" && record.Quantities["aiCores"].Value.Value() != int64(tc.cores) {
				t.Fatal("exact AI Cores lost")
			}
		})
	}
}

func TestFactNodeReplayUpdatesModeWithoutPodEvent(t *testing.T) {
	p := factTestPod()
	client := fake.NewSimpleClientset(p, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1"}})
	stop := make(chan struct{})
	defer close(stop)
	data := &Data{k8sCl: client, informers: informers.NewSharedInformerFactory(client, 0), stopCh: stop}
	r, err := NewPodRepo(data, log.NewStdLogger(io.Discard), &conf.Bootstrap{}, &mutableCatalog{current: loadedTestCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		if err := wait.PollUntilContextTimeout(context.Background(), time.Millisecond*10, time.Second*3, true, func(context.Context) (bool, error) {
			entry, ok := r.facts.Get(p.UID)
			if !ok {
				return false, nil
			}
			record := entry.Facts.Containers[0].Allocations[0].Records[0]
			mode := ""
			if record.AllocationMode != nil {
				mode = *record.AllocationMode
			}
			return mode == want, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	check("")
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "2", Annotations: map[string]string{"hami.io/node-nvidia-register": `[{"id":"GPU-a","count":10,"devmem":16384,"devcore":100,"type":"NVIDIA","mode":"hami-core","health":true}]`}}}
	if _, err := client.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	check("soft")
	if err := client.CoreV1().Nodes().Delete(context.Background(), node.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	check("")
}

func TestFactCandidatesUseLoadedCatalogResourceNames(t *testing.T) {
	catalog := loadedTestCatalog(t)
	catalog.Cambricon = &devicecatalog.CambriconConfig{ResourceCountName: "private.example/accelerator"}
	p := factTestPod()
	p.Annotations = nil
	p.Spec.Containers[0].Resources.Limits = corev1.ResourceList{"private.example/accelerator": resource.MustParse("1")}
	r := factRepo(t, &mutableCatalog{current: catalog}, p)
	entry := factEntry(t, r, p)
	if entry.Facts.Candidate != podfacts.CandidateAccelerator || len(entry.Occupancies) != 0 {
		t.Fatal("lost configured request or inferred allocation")
	}
}

func TestUnknownAllocatorAnnotationRetainsIncompleteRecognition(t *testing.T) {
	p := factTestPod()
	p.Annotations = map[string]string{"hami.io/mthreads-vgpu-devices-allocated": "card,MTHREADS,4096,0:;"}
	r := factRepo(t, &mutableCatalog{current: loadedTestCatalog(t)}, p)
	f := factEntry(t, r, p).Facts
	if f.Candidate != podfacts.CandidateUnresolved {
		t.Fatalf("unsupported allocation became no accelerator evidence: %s", f.Candidate)
	}
	set := f.Containers[0].Allocations[0]
	if set.Source.State != resourceview.Unknown || set.Source.Reason != "version_unsupported" || len(set.Records) != 0 || set.Complete {
		t.Fatal("unsupported source manufactured facts/completeness")
	}
}
func TestNVIDIAUnlimitedComputeRequiresActualHamiCoreEvidence(t *testing.T) {
	for _, tc := range []struct {
		kind, raw, registration string
		unlimited               bool
		zeroKnown               bool
	}{
		{util.NvidiaGPUDevice, "GPU-a,NVIDIA,4096,0:;", "GPU-a,10,16384,100,T4,0,true:", true, false},
		{util.NvidiaGPUDevice, "GPU-a,NVIDIA,4096,0:;", "", false, true},
		{util.HygonGPUDevice, "DCU-0,DCU,4096,0:;", "", false, true},
	} {
		t.Run(tc.kind+tc.registration, func(t *testing.T) {
			p := factTestPod()
			p.Spec.InitContainers = nil
			p.Spec.EphemeralContainers = nil
			p.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a", util.SupportDevices[tc.kind]: tc.raw}
			r := factRepo(t, &mutableCatalog{current: loadedTestCatalog(t)}, p)
			node, _ := r.nodeLister.Get("node-a")
			node.Annotations = map[string]string{"hami.io/node-nvidia-register": tc.registration}
			entry := factEntry(t, r, p)
			record := entry.Facts.Containers[0].Allocations[0].Records[0]
			if tc.unlimited {
				if record.ComputeUnlimited == nil || !*record.ComputeUnlimited || record.Quantities["compute"].Value != nil || record.States["computeUnlimited"].State != resourceview.Known {
					t.Fatal("unlimited became known zero/one")
				}
			} else if record.ComputeUnlimited != nil {
				t.Fatal("unlimited inferred without supported mode")
			}
			q := entry.Occupancies[0].Quantities["compute"]
			if tc.zeroKnown && (q.Value == nil || q.Value.Sign() != 0) {
				t.Fatal("unrelated zero altered")
			}
			if tc.unlimited && q.Value != nil {
				t.Fatal("unlimited effective occupancy became zero")
			}
			if record.Quantities["memory"].Value.Value() != 4096*1024*1024 || record.Quantities["slots"].Value.Value() != 1 {
				t.Fatal("unlimited destroyed known allocation dimensions")
			}
		})
	}
}

func TestNVIDIAAllocationShapeUsesRecordsAndSourceEvidence(t *testing.T) {
	for _, tc := range []struct{ name, mode, reservation, request, want string }{
		{"registered software allocation", "hami-core", "", "", "soft"},
		{"request alone", "", "", "hami-core", ""},
		{"MIG registration alone", "mig", "", "", ""},
		{"matched MIG reservation", "mig", `[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-a","profile":"1g.5gb","placement":{"start":0,"size":1}}]`, "", "mig"},
		{"conflicting MIG reservation", "hami-core", `[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-a","profile":"1g.5gb","placement":{"start":0,"size":1}}]`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := factTestPod()
			p.Spec.InitContainers = nil
			p.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a", util.SupportDevices[util.NvidiaGPUDevice]: "GPU-a,NVIDIA,16384,100:;", "hami.io/vgpu-mig-allocations": tc.reservation, "nvidia.com/vgpu-mode": tc.request}
			r := factRepo(t, &mutableCatalog{current: loadedTestCatalog(t)}, p)
			node, _ := r.nodeLister.Get("node-a")
			if tc.mode != "" {
				node.Annotations = map[string]string{"hami.io/node-nvidia-register": `[{"id":"GPU-a","mode":"` + tc.mode + `"}]`}
			}
			record := factEntry(t, r, p).Facts.Containers[0].Allocations[0].Records[0]
			got := ""
			if record.AllocationMode != nil {
				got = *record.AllocationMode
			}
			if got != tc.want {
				t.Fatalf("shape=%q want=%q", got, tc.want)
			}
			if got == "soft" && record.States["allocationMode"].Source != "hami_annotation_and_node_registration" {
				t.Fatal("software shape hid registration evidence")
			}
		})
	}
}
