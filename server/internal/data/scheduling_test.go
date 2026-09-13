package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/provider/util"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func schedulingTestPod(name, uid string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "dev", Name: name, UID: types.UID(uid), CreationTimestamp: metav1.NewTime(time.Unix(100, 0))},
		Spec: corev1.PodSpec{SchedulerName: "hami-scheduler", Containers: []corev1.Container{{Name: "main", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{
			"nvidia.com/gpu": resource.MustParse("1"), "nvidia.com/gpucores": resource.MustParse("5"), "nvidia.com/gpumem": resource.MustParse("256"),
		}}}}},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
}

func schedulingTestRepo(t *testing.T, client *fake.Clientset, pods ...*corev1.Pod) *podRepo {
	t.Helper()
	resources, err := schedulingResources(nil)
	if err != nil {
		t.Fatal(err)
	}
	r := &podRepo{data: &Data{k8sCl: client}, pods: map[types.UID]*biz.PodInfo{}, schedulingResourceNames: resources, schedulingEvents: newSchedulingEventReader(client)}
	r.schedulingEvents.limiter = rate.NewLimiter(rate.Inf, 1)
	r.podIndexer = cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc, schedulingGPUIndex: r.schedulingIndex})
	r.podLister = listers.NewPodLister(r.podIndexer)
	for _, pod := range pods {
		if err := r.podIndexer.Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestSchedulingStageUsesAssignmentAndGatesNotPhaseAlone(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*corev1.Pod)
	}{
		{"pending", "waiting", func(*corev1.Pod) {}},
		{"no status yet", "waiting", func(p *corev1.Pod) { p.Status.Phase = "" }},
		{"gate without condition", "gated", func(p *corev1.Pod) { p.Spec.SchedulingGates = []corev1.PodSchedulingGate{{Name: "test.io/hold"}} }},
		{"bound image pull failure", "bound", func(p *corev1.Pod) {
			p.Spec.NodeName = "node-a"
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse}}
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}
		}},
		{"preallocation is not binding", "waiting", func(p *corev1.Pod) { p.Annotations = map[string]string{util.AssignedNodeAnnotations: "node-a"} }},
		{"condition alone incomplete", "unknown", func(p *corev1.Pod) {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}}
		}},
		{"deleting", "terminating", func(p *corev1.Pod) { v := metav1.Now(); p.DeletionTimestamp = &v }},
		{"finished", "finished", func(p *corev1.Pod) { p.Status.Phase = corev1.PodFailed }},
		{"unbound unknown phase", "unknown", func(p *corev1.Pod) { p.Status.Phase = corev1.PodUnknown }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pod := schedulingTestPod("p", "u")
			tc.change(pod)
			if got := schedulingStage(pod); got != tc.want {
				t.Fatalf("stage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSchedulingIndexAndStablePendingListMakeNoAPIRequests(t *testing.T) {
	client := fake.NewSimpleClientset()
	one, two := schedulingTestPod("b", "2"), schedulingTestPod("a", "1")
	r := schedulingTestRepo(t, client, one, two)
	for i := 0; i < 5000; i++ {
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cpu-%d", i), Namespace: "dev"}}
		if err := r.podIndexer.Add(pod); err != nil {
			t.Fatal(err)
		}
	}
	candidates, _ := r.podIndexer.ByIndex(schedulingGPUIndex, "gpu")
	if len(candidates) != 2 {
		t.Fatalf("candidate index contains %d, want 2", len(candidates))
	}
	items, err := r.ListSchedulingPods(context.Background())
	if err != nil || len(items) != 2 || items[0].UID != "1" || items[1].UID != "2" {
		t.Fatalf("list = %#v, %v", items, err)
	}
	bound := two.DeepCopy()
	bound.Spec.NodeName = "node-a"
	if err := r.podIndexer.Update(bound); err != nil {
		t.Fatal(err)
	}
	items, _ = r.ListSchedulingPods(context.Background())
	if len(items) != 2 || items[0].UID != "1" || items[0].Stage != "bound" {
		t.Fatalf("binding removed GPU request from discovery: %#v", items)
	}
	removed := one.DeepCopy()
	removed.Spec.Containers[0].Resources = corev1.ResourceRequirements{}
	if err := r.podIndexer.Update(removed); err != nil {
		t.Fatal(err)
	}
	candidates, _ = r.podIndexer.ByIndex(schedulingGPUIndex, "gpu")
	if len(candidates) != 1 {
		t.Fatalf("index did not update resource removal: %d", len(candidates))
	}
	if len(client.Actions()) != 0 {
		t.Fatalf("list queried Kubernetes: %#v", client.Actions())
	}
}

func TestSchedulingResourcesRetainPerContainerSemanticsAndAliases(t *testing.T) {
	r := schedulingTestRepo(t, fake.NewSimpleClientset())
	pod := schedulingTestPod("p", "u")
	always := corev1.ContainerRestartPolicyAlways
	pod.Spec.InitContainers = []corev1.Container{{Name: "setup", Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{"nvidia.com/gpumem": resource.MustParse("512")}}},
		{Name: "side", RestartPolicy: &always, Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}}}}
	got := r.schedulingPod(pod)
	if len(got.Requests) != 3 || got.Requests[1].ContainerKind != "init" || got.Requests[2].ContainerKind != "sidecar" {
		t.Fatalf("requests lost container identity: %#v", got.Requests)
	}
	if got.Requests[1].Resources[0].Unit != "MiB" || got.Requests[1].Resources[0].Value != "512" {
		t.Fatalf("wrong memory units: %#v", got.Requests[1])
	}
	resources, err := schedulingResources(&conf.Scheduling{ResourceNames: []string{"custom.io/accelerator"}})
	if err != nil {
		t.Fatal(err)
	}
	r.schedulingResourceNames = resources
	pod.Spec.Containers[0].Resources.Limits["custom.io/accelerator"] = resource.MustParse("2")
	got = r.schedulingPod(pod)
	if len(got.Requests) != 1 || len(got.Requests[0].Resources) != 1 || got.Requests[0].Resources[0].Kind != "raw" || got.Requests[0].Resources[0].Unit != "" {
		t.Fatalf("alias should stay raw: %#v", got.Requests)
	}
	for _, bad := range []string{"cpu", "memory", "not valid/key", "example.io/"} {
		if _, err := schedulingResources(&conf.Scheduling{ResourceNames: []string{bad}}); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestSchedulingReasonsAreKnownTokensAndKeepConditionTime(t *testing.T) {
	resources, _ := schedulingResources(nil)
	for _, tc := range []struct{ message, codes, source string }{
		{"0/2 nodes: 1 1/1 CardInsufficientMemory, 1 NodeInsufficientDevice", "CardInsufficientMemory,NodeInsufficientDevice", "hami"},
		{"1 CardInsufficientCore, 1 Insufficient cpu", "CardInsufficientCore,InsufficientCPU", "mixed"},
		{"node had untolerated taint; Insufficient memory", "InsufficientHostMemory,UntoleratedTaint", "kubernetes"},
		{"prefixCardInsufficientMemorySuffix", "UnknownSchedulingReason", "unknown"},
		{"", "", "unknown"},
	} {
		codes, source := schedulingReasons(tc.message, resources)
		if strings.Join(codes, ",") != tc.codes || source != tc.source {
			t.Errorf("%q: %v/%s", tc.message, codes, source)
		}
	}
	r := schedulingTestRepo(t, fake.NewSimpleClientset())
	p := schedulingTestPod("p", "u")
	transition := metav1.NewTime(time.Unix(10, 0))
	p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Message: "CardInsufficientMemory", LastTransitionTime: transition}}
	one := r.schedulingPod(p)
	p.Status.Conditions[0].Message = "CardTypeMismatch"
	two := r.schedulingPod(p)
	if !two.Condition.TransitionAt.Equal(one.Condition.TransitionAt) || two.ReasonCodes[0] != "CardTypeMismatch" {
		t.Fatalf("message/time semantics changed: %#v", two)
	}
}

func TestSchedulingDetailValidatesIdentityBeforeEventsAndRefreshesAfter(t *testing.T) {
	client := fake.NewSimpleClientset()
	p := schedulingTestPod("p", "u")
	p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Message: "CardInsufficientMemory"}}
	r := schedulingTestRepo(t, client, p)
	for _, tc := range []struct {
		name, uid string
		code      int32
	}{{"p", "old", 409}, {"missing", "u", 404}} {
		_, err := r.GetSchedulingPod(context.Background(), "dev", tc.name, tc.uid)
		if kratoserrors.FromError(err).Code != tc.code {
			t.Fatalf("identity error = %v", err)
		}
	}
	if len(client.Actions()) != 0 {
		t.Fatal("invalid identity queried events")
	}
	client.PrependReactor("list", "events", func(action ktesting.Action) (bool, runtime.Object, error) {
		selector := action.(ktesting.ListAction).GetListRestrictions().Fields.String()
		if action.GetNamespace() != "dev" || selector != "involvedObject.uid=u" {
			t.Errorf("unscoped events query: %s %s", action.GetNamespace(), selector)
		}
		bound := p.DeepCopy()
		bound.Spec.NodeName = "node-a"
		if err := r.podIndexer.Update(bound); err != nil {
			t.Error(err)
		}
		return true, &corev1.EventList{}, nil
	})
	got, err := r.GetSchedulingPod(context.Background(), "dev", "p", "u")
	if err != nil || got.Pod.Stage != "bound" || len(got.Pod.ReasonCodes) > 0 || got.Events.Status != "empty" {
		t.Fatalf("stale detail: %#v %v", got, err)
	}
}

func TestSchedulingEventSingleflightAndCanceledCallerIsolation(t *testing.T) {
	client := fake.NewSimpleClientset()
	reader := newSchedulingEventReader(client)
	reader.limiter = rate.NewLimiter(rate.Inf, 1)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return true, &corev1.EventList{}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := reader.get(ctx, "dev", "u"); first <- err }()
	<-started
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := reader.get(context.Background(), "dev", "u")
			if err != nil || result.Status != "empty" {
				t.Errorf("shared read: %#v %v", result, err)
			}
		}()
	}
	cancel()
	if err := <-first; err != context.Canceled {
		t.Fatalf("canceled caller: %v", err)
	}
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("100 readers caused %d queries", calls.Load())
	}
	if _, err := reader.get(context.Background(), "dev", "u"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("empty result was not cached")
	}
}

func TestSchedulingEventPagingNormalizationAndBoundedRetention(t *testing.T) {
	var pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		pages++
		query := request.URL.Query()
		if query.Get("limit") != "200" || (pages > 1 && query.Get("continue") == "") || query.Get("fieldSelector") != "involvedObject.uid=u" {
			t.Errorf("bad pagination: %#v", query)
		}
		list := &corev1.EventList{TypeMeta: metav1.TypeMeta{Kind: "EventList", APIVersion: "v1"}, ListMeta: metav1.ListMeta{Continue: "next"}}
		for i := 0; i < 200; i++ {
			list.Items = append(list.Items, corev1.Event{ObjectMeta: metav1.ObjectMeta{UID: types.UID(fmt.Sprintf("%d-%d", pages, i)), Namespace: "dev"},
				InvolvedObject: corev1.ObjectReference{UID: "u"}, Message: strings.Repeat("显", 1500), ReportingController: "hami-scheduler",
				Series: &corev1.EventSeries{Count: 5, LastObservedTime: metav1.NewMicroTime(time.Unix(int64(pages*200+i), 0))}})
		}
		list.Items = append(list.Items, corev1.Event{ObjectMeta: metav1.ObjectMeta{Namespace: "dev"}, InvolvedObject: corev1.ObjectReference{UID: "wrong"}})
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(list); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	client, err := newBoundedSchedulingEventClient(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	reader := newSchedulingEventReader(client)
	reader.limiter = rate.NewLimiter(rate.Inf, 1)
	got, err := reader.get(context.Background(), "dev", "u")
	if err != nil || pages != 3 || !got.Incomplete || len(got.Items) != 50 {
		t.Fatalf("paging: %d %#v %v", pages, got, err)
	}
	first := got.Items[0]
	if first.UID != "3-199" || first.Source != "hami-scheduler" || first.Count != 5 || !first.MessageTruncated || len(first.Message) > 4096 || !utf8.ValidString(first.Message) {
		t.Fatalf("normalization: %#v", first)
	}
	if cap(got.Items) > 64 {
		t.Fatalf("retained the full backing array: cap=%d", cap(got.Items))
	}
}

func TestSchedulingEventResponseStopsBeforeUnboundedDecode(t *testing.T) {
	body := &schedulingEventBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat("x", schedulingEventPageBytes+100))), remaining: schedulingEventPageBytes}
	data, err := io.ReadAll(body)
	if len(data) != schedulingEventPageBytes || !errors.Is(err, errSchedulingEventPageTooLarge) {
		t.Fatalf("body bound: %d %v", len(data), err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"kind":"EventList","apiVersion":"v1","items":[{"message":"`+strings.Repeat("x", schedulingEventPageBytes)+`"}]}`)
	}))
	defer server.Close()
	client, err := newBoundedSchedulingEventClient(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	reader := newSchedulingEventReader(client)
	result, err := reader.get(context.Background(), "dev", "u")
	if err != nil || result.Status != "error" || !result.Incomplete || len(result.Items) > 0 {
		t.Fatalf("oversized page: %#v %v", result, err)
	}
}

func TestSchedulingEventFailuresExpireAndCachePayloadIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		err        error
	}{
		{"forbidden", "forbidden", apierrors.NewForbidden(schema.GroupResource{Resource: "events"}, "", fmt.Errorf("denied"))},
		{"timeout", "timeout", context.DeadlineExceeded},
		{"limited", "limited", apierrors.NewTooManyRequests("busy", 1)},
		{"error", "error", fmt.Errorf("connection failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			reader := newSchedulingEventReader(client)
			reader.limiter = rate.NewLimiter(rate.Inf, 1)
			now := time.Unix(10, 0)
			reader.now = func() time.Time { return now }
			client.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) { return true, nil, tc.err })
			got, err := reader.get(context.Background(), "dev", "u")
			if err != nil || got.Status != tc.want || !got.Incomplete {
				t.Fatalf("error status: %#v %v", got, err)
			}
			if _, ok := reader.cached("dev/u"); !ok {
				t.Fatal("negative cache missing")
			}
			now = now.Add(3 * time.Second)
			if _, ok := reader.cached("dev/u"); ok {
				t.Fatal("error cache did not expire promptly")
			}
		})
	}
	reader := newSchedulingEventReader(fake.NewSimpleClientset())
	reader.maxBytes = 2048
	for i := 0; i < 1000; i++ {
		reader.remember(fmt.Sprint(i), biz.SchedulingEvents{Status: "available", Items: []biz.SchedulingEvent{{Message: strings.Repeat("x", 100)}}})
	}
	if reader.bytes > reader.maxBytes || len(reader.entries) > schedulingEventCacheItems || len(reader.order) != len(reader.entries) {
		t.Fatalf("unbounded cache: %d %d %d", reader.bytes, len(reader.entries), len(reader.order))
	}
}

func TestSchedulingEventConcurrentDifferentPodsHaveNoUnboundedQueue(t *testing.T) {
	reader := newSchedulingEventReader(fake.NewSimpleClientset())
	reader.slots <- struct{}{}
	reader.slots <- struct{}{}
	got, err := reader.get(context.Background(), "dev", "u")
	if err != nil || got.Status != "limited" || !got.Incomplete {
		t.Fatalf("overload: %#v %v", got, err)
	}
	<-reader.slots
	<-reader.slots
}

func TestSchedulingEventPageRateBudgetRespectsDeadline(t *testing.T) {
	reader := newSchedulingEventReader(fake.NewSimpleClientset())
	reader.limiter = rate.NewLimiter(1, 1)
	if !reader.limiter.Allow() {
		t.Fatal("expected initial token")
	}
	reader.timeout = 10 * time.Millisecond
	started := time.Now()
	result, err := reader.get(context.Background(), "dev", "u")
	if err != nil || result.Status != "timeout" || !result.Incomplete {
		t.Fatalf("rate budget: %#v %v", result, err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("rate limit created an unbounded wait")
	}
	if len(reader.client.(*fake.Clientset).Actions()) != 0 {
		t.Fatal("exhausted budget still queried API")
	}
}

func TestSchedulingReasonsMatchExactExtendedResourcesAndMissingPVC(t *testing.T) {
	defaults, _ := schedulingResources(nil)
	aliases, _ := schedulingResources(&conf.Scheduling{ResourceNames: []string{"custom.io/gpu", "memory.example/gpu", "custom.io/CardInsufficientCore"}})
	for _, tc := range []struct {
		message       string
		resources     map[corev1.ResourceName]struct{}
		codes, source string
	}{
		{"0/1 nodes: Insufficient nvidia.com/gpu, Insufficient nvidia.com/gpucores", defaults, "ExtendedResourceUnavailable", "kubernetes"},
		{"Insufficient nvidia.com/gpumem.", defaults, "ExtendedResourceUnavailable", "kubernetes"},
		{"Insufficient nvidia.com/gpu-extra", defaults, "UnknownSchedulingReason", "unknown"},
		{"Insufficient custom.io/gpu", aliases, "ExtendedResourceUnavailable", "kubernetes"},
		{"Insufficient nvidia.com/gpu", aliases, "UnknownSchedulingReason", "unknown"},
		{"Insufficient memory.example/gpu", defaults, "UnknownSchedulingReason", "unknown"},
		{"Insufficient memory.example/gpu", aliases, "ExtendedResourceUnavailable", "kubernetes"},
		{"Insufficient custom.io/CardInsufficientCore", aliases, "ExtendedResourceUnavailable", "kubernetes"},
		{"Insufficient cpu.example/gpu; Insufficient memory-other", defaults, "UnknownSchedulingReason", "unknown"},
		{"Insufficient cpu, Insufficient memory. preemption: no victims", defaults, "InsufficientCPU,InsufficientHostMemory", "kubernetes"},
		{"CardInsufficientMemory; Insufficient nvidia.com/gpu", defaults, "CardInsufficientMemory,ExtendedResourceUnavailable", "mixed"},
		{`0/1 nodes are available: persistentvolumeclaim "webui-demo-intentionally-missing-pvc" not found. not found`, defaults, "PVCNotFound", "kubernetes"},
		{`persistentvolumeclaims "cache" not found`, defaults, "PVCNotFound", "kubernetes"},
		{`persistentvolumeclaim "cache" is not bound; node "worker" not found`, defaults, "UnknownSchedulingReason", "unknown"},
		{`configmap "cache" not found`, defaults, "UnknownSchedulingReason", "unknown"},
	} {
		got, source := schedulingReasons(tc.message, tc.resources)
		if strings.Join(got, ",") != tc.codes || source != tc.source {
			t.Errorf("%q = %v/%s, want %s/%s", tc.message, got, source, tc.codes, tc.source)
		}
	}
}

func TestSchedulingDiscoveryKeepsBoundGPUWithoutHAMiAnnotationsAndObservedStatuses(t *testing.T) {
	pod := schedulingTestPod("default-scheduler-work", "bound-without-hami")
	pod.Spec.NodeName = "node-a"
	pod.Spec.SchedulerName = "default-scheduler"
	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	always := corev1.ContainerRestartPolicyAlways
	pod.Spec.InitContainers = []corev1.Container{
		{Name: "prepare", Resources: pod.Spec.Containers[0].Resources},
		{Name: "side", RestartPolicy: &always, Resources: pod.Spec.Containers[0].Resources},
	}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "main", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
		Reason: "CreateContainerError", Message: "no binding pod found on node node-a",
	}}}}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{
		{Name: "prepare", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
		{Name: "side", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	}
	unobserved := schedulingTestPod("unobserved", "unobserved")
	unobserved.Spec.NodeName = "node-a"
	unobserved.Status.Phase = corev1.PodRunning
	client := fake.NewSimpleClientset()
	r := schedulingTestRepo(t, client, pod, unobserved)
	items, err := r.ListSchedulingPods(context.Background())
	if err != nil || len(items) != 2 || items[0].UID != "bound-without-hami" || items[0].Stage != "bound" || items[0].Preallocated {
		t.Fatalf("bound requests disappeared or fabricated allocation: %#v, %v", items, err)
	}
	requests := items[0].Requests
	if len(requests) != 3 || requests[0].Status != biz.ContainerStatusError || requests[0].StatusDetail.Reason != "CreateContainerError" || requests[1].Status != biz.ContainerStatusClosed || requests[2].Status != biz.ContainerStatusSuccess {
		t.Fatalf("regular/init/sidecar observations incorrect: %#v", requests)
	}
	if items[1].Requests[0].Status != biz.ContainerStatusUnknown {
		t.Fatal("Pod Running phase alone incorrectly became container Running")
	}
	if len(r.pods) != 0 || len(client.Actions()) != 0 || pod.Spec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Fatal("read-only discovery changed inventory, Pod or queried Kubernetes")
	}
}

func TestSchedulingOrdinaryInitRunningDoesNotRequireReadiness(t *testing.T) {
	pod := schedulingTestPod("init", "init")
	pod.Spec.RestartPolicy = corev1.RestartPolicyAlways
	ctr := corev1.Container{Name: "prepare"}
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "prepare", Ready: false, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}}
	status, detail := schedulingContainerStatus(pod, ctr, "init")
	if status != biz.ContainerStatusSuccess || detail.ContainerState != "Running" || detail.Ready != nil {
		t.Fatalf("ordinary init readiness incorrectly treated as failure: %s/%#v", status, detail)
	}
	now := metav1.Now()
	pod.DeletionTimestamp = &now
	status, _ = schedulingContainerStatus(pod, ctr, "init")
	if status != biz.ContainerStatusTerminating {
		t.Fatalf("init running overrode deletion: %s", status)
	}
}

func TestSchedulingListOmitsFinishedPodsAndKeepsTerminatingOnes(t *testing.T) {
	now := metav1.Now()
	pod := func(name string, phase corev1.PodPhase, deleting bool) *corev1.Pod {
		p := schedulingTestPod(name, name+"-uid")
		p.Spec.NodeName = "node-a"
		p.Status.Phase = phase
		if deleting {
			p.DeletionTimestamp = &now
		}
		return p
	}
	repo := schedulingTestRepo(t, fake.NewSimpleClientset(),
		pod("succeeded", corev1.PodSucceeded, false),
		pod("failed", corev1.PodFailed, false),
		pod("succeeded-deleting", corev1.PodSucceeded, true),
		pod("stopping", corev1.PodRunning, true),
		pod("running", corev1.PodRunning, false),
	)
	listed, err := repo.ListSchedulingPods(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stages := map[string]string{}
	for _, p := range listed {
		stages[p.Name] = p.Stage
	}
	if want := map[string]string{"stopping": "terminating", "running": "bound"}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("listed stages = %v, want %v", stages, want)
	}
	if _, err := repo.GetSchedulingPod(context.Background(), "dev", "failed", "failed-uid"); err != nil {
		t.Fatalf("a finished Pod remains inspectable: %v", err)
	}
}

func TestSchedulingReasonsRecognizeCommonNodeAndHAMiFeedback(t *testing.T) {
	resources, err := schedulingResources(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		message, source string
		want            []string
	}{
		{"0/3 nodes are available: 3 node(s) were unschedulable. preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.", "kubernetes", []string{"NodeUnschedulable"}},
		{"0/4 nodes are available: 2 node(s) didn't match pod anti-affinity rules, 2 node(s) didn't satisfy existing pods anti-affinity rules.", "kubernetes", []string{"PodAffinity"}},
		{"0/2 nodes are available: 1 Too many pods, 1 node(s) didn't have free ports for the requested pod ports.", "kubernetes", []string{"TooManyPods", "HostPortConflict"}},
		{"0/1 nodes are available: 1 1/8 CardNotFoundCustomFilterRule.", "hami", []string{"CardNotFoundCustomFilterRule"}},
		{"0/2 nodes are available: 1 1/4 ModeNotFit, 1 node(s) were unschedulable.", "mixed", []string{"ModeNotFit", "NodeUnschedulable"}},
	} {
		codes, source := schedulingReasons(tc.message, resources)
		if source != tc.source || !reflect.DeepEqual(codes, tc.want) {
			t.Errorf("schedulingReasons(%q) = %v, %q; want %v, %q", tc.message, codes, source, tc.want, tc.source)
		}
	}
}
