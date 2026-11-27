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
		{Scope: "node"},
		{Scope: "node", Node: "n1", DeviceId: "d1"},
		{Scope: "device"},
		{Scope: "device", DeviceId: "d1", Node: "n1"},
		{Scope: "container", Namespace: "ns", Pod: "p", Container: "c"},
		{Scope: "container", Namespace: "ns", Pod: "p", Container: "c", PodUid: "u1", Node: "n1"},
		{Scope: "cluster", Start: "not-a-time"},
		{Scope: "cluster", End: "not-a-time"},
		{Scope: "cluster", Start: "2026-01-02T00:00:00Z", End: "2026-01-01T00:00:00Z"},
		{Scope: "cluster", Start: "2026-01-01T00:00:00Z", End: "2026-01-16T00:00:00Z"},
		{Scope: "cluster", Step: "15s"},
		{Scope: "cluster", Step: "2h"},
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
		Scope: "node",
		Node:  `worker-1",or="inject`,
	}); err != nil {
		t.Fatalf("node trends failed: %v", err)
	}
	for _, q := range seenQueries {
		if !strings.Contains(q, "hami_webui_device_") || !strings.Contains(q, `node="worker-1\",or=\"inject"`) {
			t.Fatalf("unsafe or unexpected physical query: %s", q)
		}
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
			Scope: "node", Node: node, Start: req.Start, End: req.End, Step: req.Step,
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

func parsedCacheKey(t *testing.T, req *obsv1.TrendsRequest, now time.Time) string {
	t.Helper()
	parsed, err := parseTrendsQuery(req, now)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.cacheKey()
}
