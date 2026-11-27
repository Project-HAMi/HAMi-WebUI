package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	obsv1 "vgpu/api/observability/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/resourceview"
)

type fakeRangeQuerier struct {
	fn func(context.Context, string, promv1.Range) (model.Value, error)
}

func (f fakeRangeQuerier) QueryRange(ctx context.Context, query string, r promv1.Range) (model.Value, error) {
	return f.fn(ctx, query, r)
}

func TestHistoricalReaderValidationAndCanonicalParams(t *testing.T) {
	now := time.Unix(1760000400, 0).UTC()
	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(context.Context, string, promv1.Range) (model.Value, error) {
			return model.Matrix{}, nil
		},
	})
	reader.now = func() time.Time { return now }

	invalid := []*obsv1.TrendsRequest{
		nil,
		{Scope: ""},
		{Scope: "unknown"},
		{Scope: "cluster", Node: "n1"},
		{Scope: "cluster", NodeUid: "u1"},
		{Scope: "node"},
		{Scope: "node", Node: "n1"},
		{Scope: "node", Node: "n1", NodeUid: "u1", DeviceId: "d1"},
		{Scope: "device"},
		{Scope: "device", DeviceId: "d1", Node: "n1"},
		{Scope: "container", Namespace: "ns", Pod: "p", Container: "c"},
		{Scope: "container", Namespace: "ns", Pod: "p", Container: "c", PodUid: "u1", Node: "n1"},
		{Scope: "container", Namespace: "ns", Pod: "p", Container: "c", PodUid: "u1", NodeUid: "n1"},
		{Scope: "cluster", Start: "not-a-time"},
		{Scope: "cluster", End: "not-a-time"},
		{Scope: "cluster", Start: "2026-01-02T00:00:00Z", End: "2026-01-01T00:00:00Z"},
		{Scope: "cluster", Start: "2026-01-01T00:00:00Z", End: "2026-02-02T00:00:00Z"},
		{Scope: "cluster", Start: "2026-01-01T12:05:00Z", End: "2026-01-01T12:10:00Z", Step: "1h"},
		{Scope: "cluster", Step: "15s"},
		{Scope: "cluster", Step: "7h"},
		{Scope: "cluster", Step: "4500ms"},
		{Scope: "cluster", Start: "2026-01-01T00:00:00Z", End: "2026-01-01T06:00:00Z", Step: "30s"},
	}
	for i, req := range invalid {
		if _, err := reader.Trends(context.Background(), req); !errors.Is(err, ErrInvalidTrendsQuery) {
			t.Fatalf("case %d (%+v): err = %v, want ErrInvalidTrendsQuery", i, req, err)
		}
	}

	reply, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{Scope: "cluster"})
	if err != nil {
		t.Fatalf("default cluster trends failed: %v", err)
	}
	if reply.StepSeconds != 60 {
		t.Fatalf("StepSeconds = %d, want 60", reply.StepSeconds)
	}
	if len(reply.Series) != 4 {
		t.Fatalf("cluster series count = %d, want 4", len(reply.Series))
	}

	singleAligned, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope: "cluster",
		Start: "2026-01-01T12:05:00Z",
		End:   "2026-01-01T13:05:00Z",
		Step:  "1h",
	})
	if err != nil {
		t.Fatalf("single aligned boundary query failed: %v", err)
	}
	wantPoint := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	if !singleAligned.Start.AsTime().Equal(wantPoint) || !singleAligned.End.AsTime().Equal(wantPoint) {
		t.Fatalf("aligned bounds = [%v, %v], want [%v, %v]", singleAligned.Start.AsTime(), singleAligned.End.AsTime(), wantPoint, wantPoint)
	}
	if len(singleAligned.Series[0].Points) != 1 {
		t.Fatalf("single aligned points = %d, want 1", len(singleAligned.Series[0].Points))
	}

	reader.now = func() time.Time { return time.Date(2026, 1, 1, 12, 10, 17, 0, time.UTC) }
	unalignedDefault, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{Scope: "cluster"})
	if err != nil {
		t.Fatalf("unaligned default window failed: %v", err)
	}
	wantStart := time.Date(2026, 1, 1, 11, 11, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 1, 1, 12, 10, 0, 0, time.UTC)
	if !unalignedDefault.Start.AsTime().Equal(wantStart) || !unalignedDefault.End.AsTime().Equal(wantEnd) {
		t.Fatalf("unaligned default bounds = [%v, %v], want [%v, %v]", unalignedDefault.Start.AsTime(), unalignedDefault.End.AsTime(), wantStart, wantEnd)
	}

	thirtyDayReply, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope: "cluster",
		Start: "2026-01-01T00:00:00Z",
		End:   "2026-01-31T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("30d cluster trends failed: %v", err)
	}
	if thirtyDayReply.StepSeconds != int64((6 * time.Hour) / time.Second) {
		t.Fatalf("30d StepSeconds = %d, want %d", thirtyDayReply.StepSeconds, int64((6 * time.Hour) / time.Second))
	}
}

func TestHistoricalReaderBuildsSafeNamedPromQLQueries(t *testing.T) {
	now := time.Unix(1760000000, 0).UTC()
	var seenQueries []string
	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(_ context.Context, query string, _ promv1.Range) (model.Value, error) {
			seenQueries = append(seenQueries, query)
			return model.Matrix{}, nil
		},
	})
	reader.now = func() time.Time { return now }

	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope:   "node",
		Node:    `worker-1",or="inject`,
		NodeUid: "node-uid-1",
	}); err != nil {
		t.Fatalf("node trends failed: %v", err)
	}
	for _, q := range seenQueries {
		if !strings.Contains(q, "hami_webui_device_") || !strings.Contains(q, `node="worker-1\",or=\"inject",node_uid="node-uid-1"`) {
			t.Fatalf("unsafe or unexpected physical node query: %s", q)
		}
	}

	seenQueries = nil
	opaqueID, err := resourceview.DeviceID("hami-nvidia", "worker-1", "dev-1")
	if err != nil {
		t.Fatalf("DeviceID() error = %v", err)
	}
	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope:    "device",
		DeviceId: opaqueID,
		NodeUid:  "node-uid-2",
	}); err != nil {
		t.Fatalf("device trends with node_uid failed: %v", err)
	}
	if len(seenQueries) != 6 {
		t.Fatalf("device queries = %d, want 6: %v", len(seenQueries), seenQueries)
	}
	for _, q := range seenQueries[:4] {
		if !strings.Contains(q, fmt.Sprintf(`device_id=%q,node_uid="node-uid-2"`, opaqueID)) {
			t.Fatalf("device query missing optional node_uid filter: %s", q)
		}
	}
	if seenQueries[4] != `avg(hami_device_temperature{node="worker-1",device_uuid="dev-1"})` ||
		seenQueries[5] != `avg(hami_device_power{node="worker-1",device_uuid="dev-1"})` {
		t.Fatalf("device sensor queries = %v", seenQueries[4:])
	}

	seenQueries = nil
	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope:    "device",
		DeviceId: "dev-1",
	}); err != nil {
		t.Fatalf("plain device trends failed: %v", err)
	}
	if len(seenQueries) != 6 ||
		seenQueries[4] != `avg(hami_device_temperature{device_uuid="dev-1"})` ||
		seenQueries[5] != `avg(hami_device_power{device_uuid="dev-1"})` {
		t.Fatalf("plain device sensor queries = %v", seenQueries)
	}

	seenQueries = nil
	hcuID, err := resourceview.DeviceID("HCU", "worker-1", "HCU-serial-01")
	if err != nil {
		t.Fatalf("DeviceID(HCU) error = %v", err)
	}
	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope:    "device",
		DeviceId: hcuID,
	}); err != nil {
		t.Fatalf("HCU device trends failed: %v", err)
	}
	if len(seenQueries) != 6 ||
		seenQueries[4] != `avg(hami_device_temperature{node="worker-1",device_uuid="serial-01"})` ||
		seenQueries[5] != `avg(hami_device_power{node="worker-1",device_uuid="serial-01"})` {
		t.Fatalf("HCU device sensor queries = %v", seenQueries)
	}

	seenQueries = nil
	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope: "cluster",
	}); err != nil {
		t.Fatalf("cluster trends failed: %v", err)
	}
	if len(seenQueries) != 4 || !strings.Contains(seenQueries[2], "100 * avg(") || strings.Contains(seenQueries[2], "sum_over_time") {
		t.Fatalf("cluster compute_utilization_percent must evaluate 100 * avg(...) at outer step: %v", seenQueries)
	}

	seenQueries = nil
	if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope:     "container",
		Namespace: "ml",
		Pod:       "trainer-0",
		Container: "worker",
		PodUid:    "pod-uid-1",
	}); err != nil {
		t.Fatalf("container trends failed: %v", err)
	}
	if len(seenQueries) != 2 {
		t.Fatalf("container queries = %d, want 2", len(seenQueries))
	}
	for _, q := range seenQueries {
		if strings.Contains(q, "hami_webui_device_") {
			t.Fatalf("container scope must not query physical metrics: %s", q)
		}
		if !strings.Contains(q, `container_pod_uuid="worker:pod-uid-1"`) {
			t.Fatalf("container query missing canonical container_pod_uuid: %s", q)
		}
		if !strings.Contains(q, "and ignoring (container_pod_uuid)") {
			t.Fatalf("container query must pair target identity before aggregation: %s", q)
		}
		if !strings.Contains(q, "== count(max by (node, provider, device_uuid)") {
			t.Fatalf("container query missing device-set completeness guard: %s", q)
		}
	}

	seenQueries = nil
	req := &obsv1.TrendsRequest{
		Scope:     "container",
		Namespace: "ml",
		Pod:       "trainer-0",
		Container: "worker",
		PodUid:    "pod-uid-1",
		Start:     "2026-01-01T10:00:00Z",
		End:       "2026-01-01T11:00:00Z",
	}
	if _, err := reader.TrendsWithExpectedAllocation(context.Background(), req, 2, 3); err != nil {
		t.Fatalf("container trends with expected allocation failed: %v", err)
	}
	if len(seenQueries) != 2 {
		t.Fatalf("container expected-allocation queries = %d, want 2", len(seenQueries))
	}
	for _, q := range seenQueries {
		if !strings.Contains(q, ") == 2) and on () (sum(") || !strings.Contains(q, ") == 3)") {
			t.Fatalf("expected device/vGPU completeness guard missing in %s", q)
		}
		if !strings.Contains(q, "count(avg by (node, provider, device_uuid)") && !strings.Contains(q, "count(max by (node, provider, device_uuid)") {
			t.Fatalf("expected per-metric count guard missing in %s", q)
		}
	}
	if !strings.Contains(seenQueries[0], "(count(avg by (node, provider, device_uuid)") || !strings.Contains(seenQueries[0], ") == 2") {
		t.Fatalf("compute query missing == 2 guard: %s", seenQueries[0])
	}
	if strings.Count(seenQueries[1], ") == 2") < 3 {
		t.Fatalf("memory query missing device/memAlloc/memUsed == 2 guards: %s", seenQueries[1])
	}

	// Changing expected counts must not reuse cached trends from a different shape.
	seenQueries = nil
	if _, err := reader.TrendsWithExpectedAllocation(context.Background(), req, 1, 2); err != nil {
		t.Fatalf("container trends with updated expected allocation failed: %v", err)
	}
	if len(seenQueries) != 2 || !strings.Contains(seenQueries[0], ") == 1)") || !strings.Contains(seenQueries[0], ") == 2)") {
		t.Fatalf("expected cache separation for different allocation counts, got %v", seenQueries)
	}
}

func TestHistoricalReaderAlignsGridAndPreservesMissingAndConflictStates(t *testing.T) {
	start := time.Unix(1760000100, 0).UTC() // multiple of 60s? Wait: 1760000100 % 60 = 0
	end := start.Add(2 * time.Minute)
	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(_ context.Context, query string, _ promv1.Range) (model.Value, error) {
			switch {
			case strings.Contains(query, "compute_allocated_ratio"):
				return model.Matrix{
					&model.SampleStream{
						Values: []model.SamplePair{
							{Timestamp: model.Time(start.UnixMilli()), Value: 25},
							{Timestamp: model.Time(start.Add(time.Minute).UnixMilli()), Value: model.SampleValue(math.NaN())},
							{Timestamp: model.Time(end.UnixMilli()), Value: 75},
						},
					},
				}, nil
			case strings.Contains(query, "memory_allocated_bytes"):
				return model.Matrix{&model.SampleStream{}, &model.SampleStream{}}, nil
			default:
				return model.Matrix{}, nil
			}
		},
	})

	reply, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
		Scope: "device",
		DeviceId: "dev-opaque-1",
		Start: start.Format(time.RFC3339),
		End:   end.Format(time.RFC3339),
		Step:  "60s",
	})
	if err != nil {
		t.Fatalf("Trends failed: %v", err)
	}
	computeAlloc := reply.Series[0]
	if computeAlloc.State.GetState() != resourceview.Known || !computeAlloc.State.GetObservedAt().AsTime().Equal(end) {
		t.Fatalf("compute_allocation state = %+v, want known at %v", computeAlloc.State, end)
	}
	if len(computeAlloc.Points) != 3 {
		t.Fatalf("points len = %d, want 3", len(computeAlloc.Points))
	}
	if computeAlloc.Points[0].Value == nil || *computeAlloc.Points[0].Value != 25 {
		t.Fatalf("point[0] = %+v, want 25", computeAlloc.Points[0])
	}
	if computeAlloc.Points[1].Value != nil {
		t.Fatalf("NaN/gap point[1] = %v, want nil (never coerced to 0)", *computeAlloc.Points[1].Value)
	}
	if computeAlloc.Points[2].Value == nil || *computeAlloc.Points[2].Value != 75 {
		t.Fatalf("point[2] = %+v, want 75", computeAlloc.Points[2])
	}

	memAlloc := reply.Series[1]
	if memAlloc.State.GetState() != resourceview.Conflict || memAlloc.State.GetReason() != "ambiguous_series" {
		t.Fatalf("ambiguous matrix state = %+v, want conflict", memAlloc.State)
	}
	computeUtil := reply.Series[2]
	if computeUtil.State.GetState() != resourceview.Unknown || computeUtil.State.GetReason() != "no_data" {
		t.Fatalf("empty matrix state = %+v, want unknown/no_data", computeUtil.State)
	}
}

func TestHistoricalReaderSingleflightCancellationTTLCacheAndCircuitShortCircuit(t *testing.T) {
	now := time.Unix(1760000100, 0).UTC()
	var clockMu sync.Mutex
	var calls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var failErr error

	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(ctx context.Context, query string, _ promv1.Range) (model.Value, error) {
			if failErr != nil {
				calls.Add(1)
				return nil, failErr
			}
			if calls.Add(1) == 1 && strings.Contains(query, "compute_allocated_ratio") {
				started <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return model.Matrix{&model.SampleStream{
				Values: []model.SamplePair{{Timestamp: model.Time(now.UnixMilli()), Value: 40}},
			}}, nil
		},
	})
	reader.now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	reader.cacheTTL = 15 * time.Second
	reader.maxCacheEntries = 2

	req := &obsv1.TrendsRequest{
		Scope: "cluster",
		Start: now.Add(-time.Minute).Format(time.RFC3339),
		End:   now.Format(time.RFC3339),
		Step:  "60s",
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := reader.Trends(canceledCtx, req)
		errCh <- err
	}()
	<-started

	// Second concurrent caller shares singleflight; canceling caller 1 must not poison caller 2.
	okCh := make(chan *obsv1.TrendsReply, 1)
	go func() {
		reply, err := reader.Trends(context.Background(), req)
		if err != nil {
			t.Errorf("caller 2 failed: %v", err)
			return
		}
		okCh <- reply
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller err = %v, want Canceled", err)
	}
	close(release)
	reply := <-okCh
	if calls.Load() != 4 {
		t.Fatalf("singleflight upstream calls = %d, want 4 (1 per metric, shared)", calls.Load())
	}

	// Mutating returned reply must not corrupt cached copy.
	reply.Series[0].Points[1].Value = nil
	cached, err := reader.Trends(context.Background(), req)
	if err != nil {
		t.Fatalf("cache hit failed: %v", err)
	}
	if calls.Load() != 4 {
		t.Fatalf("cache miss after hit: calls = %d, want 4", calls.Load())
	}
	if cached.Series[0].Points[1].Value == nil || *cached.Series[0].Points[1].Value != 40 {
		t.Fatalf("cache copy mutated by caller: %+v", cached.Series[0].Points[1])
	}

	// Bound max entries (2): add 2 more distinct keys and verify oldest gets evicted.
	for _, node := range []string{"node-a", "node-b"} {
		if _, err := reader.Trends(context.Background(), &obsv1.TrendsRequest{
			Scope: "node", Node: node, NodeUid: "uid-" + node, Start: req.Start, End: req.End, Step: req.Step,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(reader.cache) != 2 || slices.Contains(reader.cacheOrder, parsedCacheKey(t, req, now)) {
		t.Fatalf("LRU/FIFO cache eviction failed: len=%d order=%v", len(reader.cache), reader.cacheOrder)
	}

	// Circuit open error short-circuits remaining queries and is never cached.
	calls.Store(0)
	failErr = fmt.Errorf("query Prometheus: %w", prom.ErrCircuitOpen)
	degraded, err := reader.Trends(context.Background(), req)
	if err != nil {
		t.Fatalf("circuit open should degrade per series cleanly, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls after ErrCircuitOpen = %d, want 1", calls.Load())
	}
	for _, s := range degraded.Series {
		if s.State.GetState() != resourceview.Unknown || s.State.GetReason() != "circuit_open" {
			t.Fatalf("series %s state = %+v, want circuit_open", s.Metric, s.State)
		}
	}

	// Because circuit-open reply was not cached, recovering succeeds on immediate retry.
	failErr = nil
	recovered, err := reader.Trends(context.Background(), req)
	if err != nil {
		t.Fatalf("retry after failed query failed: %v", err)
	}
	if recovered.Series[0].State.GetState() != resourceview.Known {
		t.Fatalf("failed reply was wrongly cached: %+v", recovered.Series[0].State)
	}
}

func TestHistoricalReaderCancelsUpstreamWhenAllWaitersCancelAndBoundsConcurrency(t *testing.T) {
	now := time.Unix(1760000100, 0).UTC()
	var inFlight, maxInFlight atomic.Int32
	firstStarted := make(chan struct{}, 4)
	firstCanceled := make(chan struct{}, 4)
	unblockAll := make(chan struct{})

	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(ctx context.Context, query string, _ promv1.Range) (model.Value, error) {
			cur := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				prev := maxInFlight.Load()
				if cur <= prev || maxInFlight.CompareAndSwap(prev, cur) {
					break
				}
			}
			if strings.Contains(query, `node="node-1"`) {
				select {
				case firstStarted <- struct{}{}:
				default:
				}
				select {
				case <-ctx.Done():
					select {
					case firstCanceled <- struct{}{}:
					default:
					}
					return nil, ctx.Err()
				case <-unblockAll:
				}
			} else if strings.Contains(query, `node="node-2"`) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-unblockAll:
				}
			}
			return model.Matrix{&model.SampleStream{
				Values: []model.SamplePair{{Timestamp: model.Time(now.UnixMilli()), Value: 10}},
			}}, nil
		},
	})
	reader.now = func() time.Time { return now }

	reqFor := func(node string) *obsv1.TrendsRequest {
		return &obsv1.TrendsRequest{
			Scope: "node", Node: node, NodeUid: "uid-" + node,
			Start: now.Add(-time.Minute).Format(time.RFC3339),
			End:   now.Format(time.RFC3339),
			Step:  "60s",
		}
	}

	// Sole waiter cancellation cancels upstream context immediately.
	ctx1, cancel1 := context.WithCancel(context.Background())
	errCh1 := make(chan error, 1)
	go func() {
		_, err := reader.Trends(ctx1, reqFor("node-1"))
		errCh1 <- err
	}()
	<-firstStarted
	cancel1()
	if err := <-errCh1; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sole waiter err = %v, want Canceled", err)
	}
	select {
	case <-firstCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream QueryRange context was not canceled when sole waiter canceled")
	}

	// 3 distinct slow requests share the 2-slot historical semaphore; canceling one frees a slot.
	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	errChA := make(chan error, 1)
	errChB := make(chan error, 1)
	doneC := make(chan error, 1)
	go func() {
		_, err := reader.Trends(ctxA, reqFor("node-1"))
		errChA <- err
	}()
	go func() {
		_, err := reader.Trends(ctxB, reqFor("node-2"))
		errChB <- err
	}()
	<-firstStarted
	go func() {
		_, err := reader.Trends(context.Background(), reqFor("node-3"))
		doneC <- err
	}()

	time.Sleep(30 * time.Millisecond)
	cancelA()
	if err := <-errChA; !errors.Is(err, context.Canceled) {
		t.Fatalf("reqA err = %v, want Canceled", err)
	}
	if err := <-doneC; err != nil {
		t.Fatalf("reqC failed after reqA freed concurrency slot: %v", err)
	}
	cancelB()
	<-errChB
	close(unblockAll)

	if maxInFlight.Load() > defaultMaxConcurrentFetches {
		t.Fatalf("maxInFlight = %d, want <= %d", maxInFlight.Load(), defaultMaxConcurrentFetches)
	}
}

func TestHistoricalReaderCanceledFlightDoesNotPopulateCacheForReplacement(t *testing.T) {
	baseNow := time.Unix(1760000100, 0).UTC()
	var calls atomic.Int32
	var pauseNextNow atomic.Bool
	pausedBeforePublish := make(chan struct{})
	releasePublish := make(chan struct{})
	flight2Started := make(chan struct{})
	releaseFlight2 := make(chan struct{})

	reader := NewHistoricalObservationReaderWithQuerier(fakeRangeQuerier{
		fn: func(_ context.Context, _ string, _ promv1.Range) (model.Value, error) {
			n := calls.Add(1)
			if n <= 4 {
				if n == 4 {
					pauseNextNow.Store(true)
				}
				return model.Matrix{&model.SampleStream{
					Values: []model.SamplePair{{Timestamp: model.Time(baseNow.UnixMilli()), Value: 10}},
				}}, nil
			}
			if n == 5 {
				close(flight2Started)
				<-releaseFlight2
			}
			return model.Matrix{&model.SampleStream{
				Values: []model.SamplePair{{Timestamp: model.Time(baseNow.UnixMilli()), Value: 99}},
			}}, nil
		},
	})
	reader.now = func() time.Time {
		if pauseNextNow.CompareAndSwap(true, false) {
			close(pausedBeforePublish)
			<-releasePublish
		}
		return baseNow
	}

	req := &obsv1.TrendsRequest{
		Scope: "cluster",
		Start: baseNow.Add(-time.Minute).Format(time.RFC3339),
		End:   baseNow.Format(time.RFC3339),
		Step:  "60s",
	}

	ctx1, cancel1 := context.WithCancel(context.Background())
	errCh1 := make(chan error, 1)
	go func() {
		_, err := reader.Trends(ctx1, req)
		errCh1 <- err
	}()

	<-pausedBeforePublish
	cancel1()
	if err := <-errCh1; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter err = %v, want Canceled", err)
	}

	reply2Ch := make(chan *obsv1.TrendsReply, 1)
	go func() {
		reply, err := reader.Trends(context.Background(), req)
		if err != nil {
			t.Errorf("replacement flight failed: %v", err)
			return
		}
		reply2Ch <- reply
	}()
	<-flight2Started

	close(releasePublish)
	time.Sleep(20 * time.Millisecond)
	reader.cacheMu.Lock()
	cachedCount := len(reader.cache)
	reader.cacheMu.Unlock()
	if cachedCount != 0 {
		t.Fatalf("stale canceled flight populated cache before replacement finished: count=%d", cachedCount)
	}

	close(releaseFlight2)
	reply2 := <-reply2Ch
	if got := reply2.Series[0].Points[1].GetValue(); got != 99 {
		t.Fatalf("replacement value = %v, want 99", got)
	}
	cached, err := reader.Trends(context.Background(), req)
	if err != nil {
		t.Fatalf("cache hit failed: %v", err)
	}
	if got := cached.Series[0].Points[1].GetValue(); got != 99 {
		t.Fatalf("cached value = %v, want 99", got)
	}
}

func parsedCacheKey(t *testing.T, req *obsv1.TrendsRequest, now time.Time) string {
	t.Helper()
	parsed, err := parseTrendsQuery(req, now)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.cacheKey()
}
