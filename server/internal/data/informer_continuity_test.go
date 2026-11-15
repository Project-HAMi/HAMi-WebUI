package data

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	kt "k8s.io/client-go/testing"
)

func continuityPod(uid, rv string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: types.UID(uid), ResourceVersion: rv}}
}
func listContinuity(s *informerContinuity, rv string, pods ...corev1.Pod) {
	o := metav1.ListOptions{}
	s.beginList(o)
	s.listResult(o, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: rv}, Items: pods}, nil, true)
}
func awaitContinuity(t *testing.T, predicate func() bool) {
	t.Helper()
	if err := wait.PollUntilContextTimeout(context.Background(), 5*time.Millisecond, 5*time.Second, true, func(context.Context) (bool, error) { return predicate(), nil }); err != nil {
		t.Fatal(err)
	}
}
func TestContinuityNeedsCompleteListWatchAndConsumedUIDRV(t *testing.T) {
	s := newInformerContinuity()
	p := continuityPod("old", "10")
	listContinuity(s, "10", p)
	if s.snapshot().ready {
		t.Fatal("LIST alone complete")
	}
	o := metav1.ListOptions{ResourceVersion: "10"}
	if err := s.allowWatch(o, true); err != nil {
		t.Fatal(err)
	}
	upstream := watch.NewRaceFreeFake()
	wrapped, err := s.wrapWatch(context.Background(), o, upstream, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer wrapped.Stop()
	before := s.snapshot()
	if !continuityCoverage("pod", "pods", before, []metav1.Object{&p}, s.unchanged(before)).Complete {
		t.Fatal("exact consumer snapshot not complete")
	}
	updated := p.DeepCopy()
	updated.ResourceVersion = "11"
	upstream.Modify(updated)
	awaitContinuity(t, func() bool { return s.snapshot().rv == "11" })
	pending := s.snapshot()
	if continuityCoverage("pod", "pods", pending, []metav1.Object{&p}, s.unchanged(pending)).Complete {
		t.Fatal("queued event declared consumed")
	}
	<-wrapped.ResultChan()
	if !continuityCoverage("pod", "pods", pending, []metav1.Object{updated}, s.unchanged(pending)).Complete {
		t.Fatal("consumed event not recognized")
	}
	upstream.Stop()
	awaitContinuity(t, func() bool { return !s.snapshot().ready })
	if err := s.allowWatch(metav1.ListOptions{ResourceVersion: "11"}, true); !apierrors.IsResourceExpired(err) {
		t.Fatal("closed watch did not demand reflector relist")
	}
	replacement := continuityPod("new", "20")
	listContinuity(s, "20", replacement)
	o.ResourceVersion = "20"
	if err := s.allowWatch(o, true); err != nil {
		t.Fatal(err)
	}
	next, _ := s.wrapWatch(context.Background(), o, watch.NewRaceFreeFake(), nil)
	defer next.Stop()
	recovered := s.snapshot()
	if continuityCoverage("pod", "pods", recovered, []metav1.Object{updated}, s.unchanged(recovered)).Complete {
		t.Fatal("old UID reused on recovery")
	}
	if !continuityCoverage("pod", "pods", recovered, []metav1.Object{&replacement}, s.unchanged(recovered)).Complete {
		t.Fatal("relist and replacement failed to recover")
	}
}
func TestContinuityPaginationFailureAndEmptySnapshot(t *testing.T) {
	s := newInformerContinuity()
	p := continuityPod("a", "3")
	o := metav1.ListOptions{}
	s.beginList(o)
	s.listResult(o, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "10", Continue: "next"}, Items: []corev1.Pod{p}}, nil, true)
	if err := s.allowWatch(metav1.ListOptions{ResourceVersion: "10"}, true); err == nil {
		t.Fatal("partial LIST allowed watch proof")
	}
	o.Continue = "next"
	s.beginList(o)
	s.listResult(o, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "10"}}, nil, true)
	if err := s.allowWatch(metav1.ListOptions{ResourceVersion: "10"}, true); err != nil {
		t.Fatal(err)
	}
	s.invalidate("test")
	listContinuity(s, "20")
	w, _ := s.wrapWatch(context.Background(), metav1.ListOptions{ResourceVersion: "20"}, watch.NewRaceFreeFake(), nil)
	defer w.Stop()
	v := s.snapshot()
	c := continuityCoverage("pod", "pods", v, nil, s.unchanged(v))
	if !c.Complete || c.Total == nil || *c.Total != 0 {
		t.Fatal("proven empty snapshot lost zero")
	}
	s.beginList(metav1.ListOptions{})
	s.listResult(metav1.ListOptions{}, nil, errors.New("forbidden"), true)
	if s.snapshot().ready || s.snapshot().reason != "list_failed" {
		t.Fatal("failed relist kept old complete")
	}
}
func TestContinuityWatchErrorAndDeadlineInvalidate(t *testing.T) {
	for _, failure := range []string{"error", "silent"} {
		t.Run(failure, func(t *testing.T) {
			s := newInformerContinuity()
			listContinuity(s, "1")
			upstream := watch.NewRaceFreeFake()
			s.mu.Lock()
			s.ready = true
			s.watchGeneration = 1
			s.mu.Unlock()
			w := &continuityWatch{upstream: upstream, results: make(chan watch.Event), done: make(chan struct{}), state: s, generation: 1}
			go w.run(context.Background(), 30*time.Millisecond)
			defer w.Stop()
			if failure == "error" {
				upstream.Error(&metav1.Status{Reason: metav1.StatusReasonExpired, Code: 410})
				<-w.ResultChan()
			}
			awaitContinuity(t, func() bool { return !s.snapshot().ready })
			if s.snapshot().reason == "" {
				t.Fatal("gap reason missing")
			}
		})
	}
}
func TestTrackedFactoryActuallyRelistsAfterClosedWatch(t *testing.T) {
	p := continuityPod("old", "10")
	client := fake.NewSimpleClientset(&p)
	var lists, watches atomic.Int32
	client.PrependReactor("list", "pods", func(a kt.Action) (bool, runtime.Object, error) {
		lists.Add(1)
		obj, err := client.Tracker().List(corev1.SchemeGroupVersion.WithResource("pods"), corev1.SchemeGroupVersion.WithKind("Pod"), "")
		if err == nil {
			obj.(*corev1.PodList).ResourceVersion = "10"
		}
		return true, obj, err
	})
	streams := make(chan watch.Interface, 5)
	client.PrependWatchReactor("pods", func(a kt.Action) (bool, watch.Interface, error) {
		w, err := client.Tracker().Watch(corev1.SchemeGroupVersion.WithResource("pods"), "")
		watches.Add(1)
		streams <- w
		return true, w, err
	})
	factory, state := newTrackedInformerFactory(client, 0)
	informer := factory.Core().V1().Pods().Informer()
	stop := make(chan struct{})
	defer close(stop)
	factory.Start(stop)
	factory.WaitForCacheSync(stop)
	awaitContinuity(t, func() bool { return state.pods.snapshot().ready })
	if lists.Load() != 1 || watches.Load() != 1 {
		t.Fatalf("startup requests list=%d watch=%d", lists.Load(), watches.Load())
	}
	first := <-streams
	// One delivered event makes EOF a normal (nil-error) watch completion.
	// Recovery must therefore exercise our explicit relist requirement.
	p.ResourceVersion = "11"
	if err := client.Tracker().Update(corev1.SchemeGroupVersion.WithResource("pods"), &p, p.Namespace); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		obj, exists, _ := informer.GetStore().GetByKey("work/pod")
		return exists && obj.(*corev1.Pod).ResourceVersion == "11"
	})
	first.Stop()
	awaitContinuity(t, func() bool { return !state.pods.snapshot().ready })
	if err := client.Tracker().Delete(corev1.SchemeGroupVersion.WithResource("pods"), p.Namespace, p.Name); err != nil {
		t.Fatal(err)
	}
	replacement := continuityPod("new", "20")
	if err := client.Tracker().Create(corev1.SchemeGroupVersion.WithResource("pods"), &replacement, p.Namespace); err != nil {
		t.Fatal(err)
	}
	awaitContinuity(t, func() bool {
		obj, exists, _ := informer.GetStore().GetByKey("work/pod")
		return state.pods.snapshot().ready && exists && obj.(*corev1.Pod).UID == "new"
	})
	if lists.Load() != 2 || watches.Load() != 2 {
		t.Fatalf("recovery request budget list=%d watch=%d", lists.Load(), watches.Load())
	}
}

func TestContinuityRejectsWrongObjectType(t *testing.T) {
	s := newInformerContinuity("pods")
	listContinuity(s, "10")
	s.mu.Lock()
	s.ready = true
	s.watchGeneration = 1
	s.mu.Unlock()
	if s.watchEvent(1, watch.Event{Type: watch.Added, Object: &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "uid", ResourceVersion: "11"}}}) || s.snapshot().ready {
		t.Fatal("wrong source type retained completeness")
	}
}

func TestContinuityAccessFailureKeepsExplicitReason(t *testing.T) {
	s := newInformerContinuity("pods")
	s.beginList(metav1.ListOptions{})
	s.listResult(metav1.ListOptions{}, nil, apierrors.NewForbidden(corev1.Resource("pods"), "", errors.New("denied")), true)
	if s.snapshot().ready || s.snapshot().reason != "list_forbidden" {
		t.Fatal("authorization failure lost its category")
	}
	if c := continuityCoverage("pod", "shared_pod_informer", s.snapshot(), nil, false); c.Sources[0].Reason != "no_permission" {
		t.Fatal("permission evidence hidden in internal parameters")
	}
}
