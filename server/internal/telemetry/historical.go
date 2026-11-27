package telemetry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	obsv1 "vgpu/api/observability/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/resourceview"
)

const (
	defaultQueryTimeout         = 15 * time.Second
	defaultCacheTTL             = 15 * time.Second
	defaultMaxCacheEntries      = 256
	defaultMaxCacheBytes        = 8 << 20
	defaultMaxConcurrentFetches = 2

	minTrendStep   = 30 * time.Second
	maxTrendStep   = time.Hour
	maxTrendWindow = 14 * 24 * time.Hour
	maxTrendPoints = 360
	maxAutoPoints  = 120
	defaultWindow  = time.Hour
)

// ErrInvalidTrendsQuery indicates invalid scope, entity identity, time window, or step input.
var ErrInvalidTrendsQuery = errors.New("invalid telemetry trends query")

// RangeQuerier executes bounded Prometheus range queries.
type RangeQuerier interface {
	QueryRange(ctx context.Context, query string, r promv1.Range) (model.Value, error)
}

type cachedTrend struct {
	reply     *obsv1.TrendsReply
	size      int
	expiresAt time.Time
}

type trendFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	reply   *obsv1.TrendsReply
}

// HistoricalObservationReader serves bounded, named PromQL trends from historical
// Prometheus metrics without exposing arbitrary PromQL to callers.
type HistoricalObservationReader struct {
	querier         RangeQuerier
	now             func() time.Time
	timeout         time.Duration
	cacheTTL        time.Duration
	maxCacheEntries int
	maxCacheBytes   int

	fetchSem   chan struct{}
	cacheMu    sync.Mutex
	flights    map[string]*trendFlight
	cache      map[string]cachedTrend
	cacheOrder []string
	cacheBytes int
}

// NewHistoricalObservationReader creates a reader backed by the bounded Prometheus client.
func NewHistoricalObservationReader(client *prom.Client) *HistoricalObservationReader {
	var querier RangeQuerier
	if client != nil {
		querier = client
	}
	return NewHistoricalObservationReaderWithQuerier(querier)
}

// NewHistoricalObservationReaderWithQuerier creates a reader backed by a custom RangeQuerier.
func NewHistoricalObservationReaderWithQuerier(querier RangeQuerier) *HistoricalObservationReader {
	return &HistoricalObservationReader{
		querier:         querier,
		now:             time.Now,
		timeout:         defaultQueryTimeout,
		cacheTTL:        defaultCacheTTL,
		maxCacheEntries: defaultMaxCacheEntries,
		maxCacheBytes:   defaultMaxCacheBytes,
		fetchSem:        make(chan struct{}, defaultMaxConcurrentFetches),
		flights:         make(map[string]*trendFlight),
		cache:           make(map[string]cachedTrend),
	}
}

type parsedTrendsQuery struct {
	scope       string
	entityScope string
	node        string
	nodeUID     string
	deviceID    string
	namespace   string
	pod         string
	container   string
	podUID      string
	start       time.Time
	end         time.Time
	step        time.Duration
	specs       []namedTrendSpec
}

type namedTrendSpec struct {
	metric string
	unit   string
	query  string
}

func (r *HistoricalObservationReader) clockNow() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *HistoricalObservationReader) effectiveTimeout() time.Duration {
	if r != nil && r.timeout > 0 {
		return r.timeout
	}
	return defaultQueryTimeout
}

func (r *HistoricalObservationReader) effectiveTTL() time.Duration {
	if r != nil && r.cacheTTL > 0 {
		return r.cacheTTL
	}
	return defaultCacheTTL
}

// Trends validates req and returns named historical trend series aligned onto a
// uniform time grid. Missing samples remain nil (omitted), never coerced to zero.
func (r *HistoricalObservationReader) Trends(ctx context.Context, req *obsv1.TrendsRequest) (*obsv1.TrendsReply, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := parseTrendsQuery(req, r.clockNow())
	if err != nil {
		return nil, err
	}
	if r == nil || r.querier == nil {
		return r.unavailableReply(parsed, "prom_unavailable"), nil
	}

	key := parsed.cacheKey()
	now := r.clockNow()

	var f *trendFlight
	r.cacheMu.Lock()
	if entry, ok := r.cache[key]; ok {
		if now.Before(entry.expiresAt) {
			cached := proto.Clone(entry.reply).(*obsv1.TrendsReply)
			r.cacheMu.Unlock()
			return cached, nil
		}
		r.deleteEntryLocked(key)
	}
	if existing, ok := r.flights[key]; ok {
		existing.waiters++
		f = existing
		r.cacheMu.Unlock()
	} else {
		fetchCtx, cancel := context.WithTimeout(context.Background(), r.effectiveTimeout())
		f = &trendFlight{
			done:    make(chan struct{}),
			cancel:  cancel,
			waiters: 1,
		}
		if r.flights == nil {
			r.flights = make(map[string]*trendFlight)
		}
		r.flights[key] = f
		r.cacheMu.Unlock()
		go r.runFlight(fetchCtx, cancel, key, f, parsed)
	}

	select {
	case <-f.done:
		if f.reply == nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, context.Canceled
		}
		return proto.Clone(f.reply).(*obsv1.TrendsReply), nil
	case <-ctx.Done():
		r.cacheMu.Lock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
			if r.flights[key] == f {
				delete(r.flights, key)
			}
		}
		r.cacheMu.Unlock()
		return nil, ctx.Err()
	}
}

func (r *HistoricalObservationReader) runFlight(fetchCtx context.Context, cancel context.CancelFunc, key string, f *trendFlight, parsed parsedTrendsQuery) {
	defer func() {
		cancel()
		r.cacheMu.Lock()
		if r.flights[key] == f {
			delete(r.flights, key)
		}
		r.cacheMu.Unlock()
		close(f.done)
	}()

	if r.fetchSem != nil {
		select {
		case r.fetchSem <- struct{}{}:
			defer func() { <-r.fetchSem }()
		case <-fetchCtx.Done():
			if !errors.Is(fetchCtx.Err(), context.Canceled) {
				f.reply = r.unavailableReply(parsed, "query_failed")
			}
			return
		}
	}

	reply, cacheable := r.fetchTrends(fetchCtx, parsed)
	if errors.Is(fetchCtx.Err(), context.Canceled) {
		return
	}
	f.reply = reply
	if cacheable {
		r.putCached(key, reply, r.clockNow())
	}
}

func (r *HistoricalObservationReader) unavailableReply(q parsedTrendsQuery, reason string) *obsv1.TrendsReply {
	reply := newBaseReply(q)
	for _, spec := range q.specs {
		reply.Series = append(reply.Series, unavailableTrendSeries(spec, q.entityScope, reason, q.grid()))
	}
	return reply
}

func (r *HistoricalObservationReader) fetchTrends(ctx context.Context, q parsedTrendsQuery) (*obsv1.TrendsReply, bool) {
	reply := newBaseReply(q)
	grid := q.grid()
	promRange := promv1.Range{Start: q.start, End: q.end, Step: q.step}

	cacheable := true
	var circuitErr error
	for _, spec := range q.specs {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, false
		}
		if circuitErr != nil {
			cacheable = false
			reply.Series = append(reply.Series, unavailableTrendSeries(spec, q.entityScope, "circuit_open", grid))
			continue
		}
		value, err := r.querier.QueryRange(ctx, spec.query, promRange)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil, false
			}
			cacheable = false
			reason := classifyQueryError(err)
			if errors.Is(err, prom.ErrCircuitOpen) {
				circuitErr = err
			}
			reply.Series = append(reply.Series, unavailableTrendSeries(spec, q.entityScope, reason, grid))
			continue
		}
		matrix, ok := value.(model.Matrix)
		if !ok {
			cacheable = false
			reply.Series = append(reply.Series, unavailableTrendSeries(spec, q.entityScope, "invalid_response", grid))
			continue
		}
		reply.Series = append(reply.Series, buildMatrixSeries(spec, q.entityScope, matrix, grid, q.start, q.end, q.step))
	}
	return reply, cacheable
}

func classifyQueryError(err error) string {
	switch {
	case errors.Is(err, prom.ErrCircuitOpen):
		return "circuit_open"
	case errors.Is(err, prom.ErrResponseTooLarge):
		return "response_too_large"
	default:
		return "query_failed"
	}
}

func newBaseReply(q parsedTrendsQuery) *obsv1.TrendsReply {
	return &obsv1.TrendsReply{
		Scope:       q.scope,
		Start:       timestamppb.New(q.start),
		End:         timestamppb.New(q.end),
		StepSeconds: int64(q.step / time.Second),
		Series:      make([]*obsv1.TrendSeries, 0, len(q.specs)),
	}
}

func unavailableTrendSeries(spec namedTrendSpec, scope, reason string, grid []time.Time) *obsv1.TrendSeries {
	return &obsv1.TrendSeries{
		Metric: spec.metric,
		Unit:   spec.unit,
		State: &obsv1.FieldState{
			State:  resourceview.Unknown,
			Reason: reason,
			Source: "prometheus",
			Scope:  scope,
		},
		Points: emptyGridPoints(grid),
	}
}

func buildMatrixSeries(
	spec namedTrendSpec,
	scope string,
	matrix model.Matrix,
	grid []time.Time,
	start, end time.Time,
	step time.Duration,
) *obsv1.TrendSeries {
	if len(matrix) == 0 {
		return unavailableTrendSeries(spec, scope, "no_data", grid)
	}
	if len(matrix) > 1 {
		return &obsv1.TrendSeries{
			Metric: spec.metric,
			Unit:   spec.unit,
			State: &obsv1.FieldState{
				State:  resourceview.Conflict,
				Reason: "ambiguous_series",
				Source: "prometheus",
				Scope:  scope,
			},
			Points: emptyGridPoints(grid),
		}
	}
	points := emptyGridPoints(grid)
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()
	stepMs := step.Milliseconds()
	var latest time.Time
	for _, sample := range matrix[0].Values {
		tsMs := int64(sample.Timestamp)
		if tsMs < startMs || tsMs > endMs {
			continue
		}
		offset := tsMs - startMs
		if offset%stepMs != 0 {
			continue
		}
		v := float64(sample.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		idx := int(offset / stepMs)
		points[idx].Value = proto.Float64(v)
		sampleAt := time.UnixMilli(tsMs).UTC()
		if sampleAt.After(latest) {
			latest = sampleAt
		}
	}
	if latest.IsZero() {
		return unavailableTrendSeries(spec, scope, "no_data", grid)
	}
	return &obsv1.TrendSeries{
		Metric: spec.metric,
		Unit:   spec.unit,
		State: &obsv1.FieldState{
			State:      resourceview.Known,
			Source:     "prometheus",
			Scope:      scope,
			ObservedAt: timestamppb.New(latest),
		},
		Points: points,
	}
}

func emptyGridPoints(grid []time.Time) []*obsv1.TrendPoint {
	points := make([]*obsv1.TrendPoint, len(grid))
	for i, ts := range grid {
		points[i] = &obsv1.TrendPoint{At: timestamppb.New(ts)}
	}
	return points
}

func (q parsedTrendsQuery) grid() []time.Time {
	count := int(q.end.Sub(q.start)/q.step) + 1
	grid := make([]time.Time, count)
	for i := range count {
		grid[i] = q.start.Add(time.Duration(i) * q.step)
	}
	return grid
}

func (q parsedTrendsQuery) cacheKey() string {
	return strings.Join([]string{
		q.scope,
		q.node,
		q.nodeUID,
		q.deviceID,
		q.namespace,
		q.pod,
		q.container,
		q.podUID,
		strconv.FormatInt(q.start.Unix(), 10),
		strconv.FormatInt(q.end.Unix(), 10),
		strconv.FormatInt(int64(q.step/time.Second), 10),
	}, "\x00")
}

func (r *HistoricalObservationReader) putCached(key string, reply *obsv1.TrendsReply, now time.Time) {
	if r == nil || reply == nil {
		return
	}
	maxEntries := r.maxCacheEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxCacheEntries
	}
	maxBytes := r.maxCacheBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxCacheBytes
	}
	stored := proto.Clone(reply).(*obsv1.TrendsReply)
	size := proto.Size(stored)
	if size <= 0 {
		size = 1
	}
	if size > maxBytes {
		return
	}

	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	for _, existing := range slicesClone(r.cacheOrder) {
		if entry, ok := r.cache[existing]; ok && !now.Before(entry.expiresAt) {
			r.deleteEntryLocked(existing)
		}
	}
	if _, exists := r.cache[key]; exists {
		r.deleteEntryLocked(key)
	}
	for len(r.cacheOrder) > 0 && (len(r.cache) >= maxEntries || r.cacheBytes+size > maxBytes) {
		r.deleteEntryLocked(r.cacheOrder[0])
	}
	if len(r.cache) >= maxEntries || r.cacheBytes+size > maxBytes {
		return
	}
	r.cache[key] = cachedTrend{
		reply:     stored,
		size:      size,
		expiresAt: now.Add(r.effectiveTTL()),
	}
	r.cacheOrder = append(r.cacheOrder, key)
	r.cacheBytes += size
}

func (r *HistoricalObservationReader) deleteEntryLocked(key string) {
	entry, ok := r.cache[key]
	if !ok {
		return
	}
	delete(r.cache, key)
	r.cacheBytes -= entry.size
	if r.cacheBytes < 0 {
		r.cacheBytes = 0
	}
	for i, existing := range r.cacheOrder {
		if existing == key {
			r.cacheOrder = append(r.cacheOrder[:i], r.cacheOrder[i+1:]...)
			break
		}
	}
}

func slicesClone(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]string, len(items))
	copy(cloned, items)
	return cloned
}

func parseTrendsQuery(req *obsv1.TrendsRequest, now time.Time) (parsedTrendsQuery, error) {
	if req == nil {
		return parsedTrendsQuery{}, fmt.Errorf("%w: request is required", ErrInvalidTrendsQuery)
	}
	scope := strings.TrimSpace(req.GetScope())
	node := strings.TrimSpace(req.GetNode())
	nodeUID := strings.TrimSpace(req.GetNodeUid())
	deviceID := strings.TrimSpace(req.GetDeviceId())
	namespace := strings.TrimSpace(req.GetNamespace())
	pod := strings.TrimSpace(req.GetPod())
	container := strings.TrimSpace(req.GetContainer())
	podUID := strings.TrimSpace(req.GetPodUid())

	var entityScope string
	switch scope {
	case "cluster":
		if node != "" || nodeUID != "" || deviceID != "" || namespace != "" || pod != "" || container != "" || podUID != "" {
			return parsedTrendsQuery{}, fmt.Errorf("%w: cluster scope does not accept entity filters", ErrInvalidTrendsQuery)
		}
		entityScope = "device"
	case "node":
		if node == "" || nodeUID == "" || deviceID != "" || namespace != "" || pod != "" || container != "" || podUID != "" {
			return parsedTrendsQuery{}, fmt.Errorf("%w: node scope requires node and node_uid", ErrInvalidTrendsQuery)
		}
		entityScope = "device"
	case "device":
		if deviceID == "" || node != "" || namespace != "" || pod != "" || container != "" || podUID != "" {
			return parsedTrendsQuery{}, fmt.Errorf("%w: device scope requires device_id", ErrInvalidTrendsQuery)
		}
		entityScope = "device"
	case "container":
		if namespace == "" || pod == "" || container == "" || podUID == "" || node != "" || nodeUID != "" || deviceID != "" {
			return parsedTrendsQuery{}, fmt.Errorf("%w: container scope requires namespace, pod, container, and pod_uid", ErrInvalidTrendsQuery)
		}
		entityScope = "container"
	default:
		return parsedTrendsQuery{}, fmt.Errorf("%w: unsupported scope %q", ErrInvalidTrendsQuery, scope)
	}

	end := now.UTC()
	if rawEnd := strings.TrimSpace(req.GetEnd()); rawEnd != "" {
		parsedEnd, err := time.Parse(time.RFC3339Nano, rawEnd)
		if err != nil {
			return parsedTrendsQuery{}, fmt.Errorf("%w: end must be an RFC3339 timestamp", ErrInvalidTrendsQuery)
		}
		end = parsedEnd.UTC()
	}

	start := end.Add(-defaultWindow)
	if rawStart := strings.TrimSpace(req.GetStart()); rawStart != "" {
		parsedStart, err := time.Parse(time.RFC3339Nano, rawStart)
		if err != nil {
			return parsedTrendsQuery{}, fmt.Errorf("%w: start must be an RFC3339 timestamp", ErrInvalidTrendsQuery)
		}
		start = parsedStart.UTC()
	}

	if !end.After(start) {
		return parsedTrendsQuery{}, fmt.Errorf("%w: end must be after start", ErrInvalidTrendsQuery)
	}
	if end.Sub(start) > maxTrendWindow {
		return parsedTrendsQuery{}, fmt.Errorf("%w: window cannot exceed %s", ErrInvalidTrendsQuery, maxTrendWindow)
	}

	var step time.Duration
	if rawStep := strings.TrimSpace(req.GetStep()); rawStep != "" {
		parsedStep, err := time.ParseDuration(rawStep)
		if err != nil || parsedStep < minTrendStep || parsedStep > maxTrendStep || parsedStep%time.Second != 0 {
			return parsedTrendsQuery{}, fmt.Errorf("%w: step must be a whole-second duration between %s and %s", ErrInvalidTrendsQuery, minTrendStep, maxTrendStep)
		}
		step = parsedStep
	} else {
		step = selectAutoStep(end.Sub(start))
	}

	alignedEnd := end.Truncate(step)
	alignedStart := start.Truncate(step)
	if alignedStart.Before(start) {
		alignedStart = alignedStart.Add(step)
	}
	if alignedEnd.Before(alignedStart) {
		return parsedTrendsQuery{}, fmt.Errorf("%w: window contains no step-aligned sample points", ErrInvalidTrendsQuery)
	}
	start, end = alignedStart, alignedEnd
	if points := int(end.Sub(start)/step) + 1; points > maxTrendPoints {
		return parsedTrendsQuery{}, fmt.Errorf("%w: requested %d points exceeds maximum of %d", ErrInvalidTrendsQuery, points, maxTrendPoints)
	}

	q := parsedTrendsQuery{
		scope:       scope,
		entityScope: entityScope,
		node:        node,
		nodeUID:     nodeUID,
		deviceID:    deviceID,
		namespace:   namespace,
		pod:         pod,
		container:   container,
		podUID:      podUID,
		start:       start,
		end:         end,
		step:        step,
	}
	q.specs = buildTrendSpecs(q)
	return q, nil
}

func selectAutoStep(window time.Duration) time.Duration {
	candidates := []time.Duration{
		30 * time.Second,
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		time.Hour,
	}
	for _, candidate := range candidates {
		if int(window/candidate)+1 <= maxAutoPoints {
			return candidate
		}
	}
	return maxTrendStep
}

func buildTrendSpecs(q parsedTrendsQuery) []namedTrendSpec {
	if q.scope == "container" {
		return buildContainerTrendSpecs(q.namespace, q.pod, q.container, q.podUID)
	}
	var extraFilter string
	switch q.scope {
	case "node":
		extraFilter = fmt.Sprintf("node=%s,node_uid=%s", strconv.Quote(q.node), strconv.Quote(q.nodeUID))
	case "device":
		extraFilter = fmt.Sprintf("device_id=%s", strconv.Quote(q.deviceID))
		if q.nodeUID != "" {
			extraFilter += fmt.Sprintf(",node_uid=%s", strconv.Quote(q.nodeUID))
		}
	}
	return buildPhysicalTrendSpecs(extraFilter)
}

func buildPhysicalTrendSpecs(extraSelector string) []namedTrendSpec {
	const groupLabels = "device_id, node, provider"
	const deviceMatch = "on (device_id, node, provider)"

	metricSeries := func(field string, extra ...string) string {
		parts := []string{`device_kind="physical"`}
		if extraSelector != "" {
			parts = append(parts, extraSelector)
		}
		for _, item := range extra {
			if item != "" {
				parts = append(parts, item)
			}
		}
		return fmt.Sprintf("hami_webui_device_%s{%s}", field, strings.Join(parts, ","))
	}
	agreed := func(series string) string {
		return fmt.Sprintf("(min by (%s) (%s) == max by (%s) (%s))", groupLabels, series, groupLabels, series)
	}
	singleSource := func(series string) string {
		sources := fmt.Sprintf("group by (%s, source) (%s)", groupLabels, series)
		return fmt.Sprintf("(count by (%s) (%s) == 1)", groupLabels, sources)
	}
	pairedPercent := func(numerator, capacity, eligibility string) string {
		fullPair := fmt.Sprintf("(%s >= 0) and (%s > 0)", numerator, capacity)
		if eligibility != "" {
			fullPair = fullPair + " and " + eligibility
		}
		paired := fmt.Sprintf("group by (%s) (%s)", groupLabels, fullPair)
		used := fmt.Sprintf("(%s >= 0)", agreed(numerator))
		total := fmt.Sprintf("(%s > 0)", agreed(capacity))
		amount := fmt.Sprintf("sum(%s and %s and %s)", used, total, paired)
		size := fmt.Sprintf("sum(%s and %s and %s)", total, used, paired)
		return fmt.Sprintf("100 * (%s) / (%s)", amount, size)
	}

	unknownOcc := metricSeries("compute_unknown_occupancies")
	configured := metricSeries("info", `configured="true"`)
	computeElig := fmt.Sprintf("(%s == 0) and ignoring (configured, mode, resource, device_no, driver_version) %s and %s (%s == 0)",
		unknownOcc, configured, deviceMatch, agreed(unknownOcc))

	usage := metricSeries("utilization_ratio")
	observedUsage := fmt.Sprintf("(%s and %s)", agreed(usage), singleSource(usage))

	memUsed := metricSeries("memory_used_bytes")
	memPhysical := metricSeries("memory_physical_bytes")
	memElig := fmt.Sprintf("(%s <= %s) and %s %s", memUsed, memPhysical, deviceMatch, singleSource(memUsed+" or "+memPhysical))

	return []namedTrendSpec{
		{
			metric: "compute_allocation_percent",
			unit:   "percent",
			query:  pairedPercent(metricSeries("compute_allocated_ratio"), metricSeries("compute_capacity_ratio"), computeElig),
		},
		{
			metric: "memory_allocation_percent",
			unit:   "percent",
			query:  pairedPercent(metricSeries("memory_allocated_bytes"), metricSeries("memory_capacity_bytes"), ""),
		},
		{
			metric: "compute_utilization_percent",
			unit:   "percent",
			query:  fmt.Sprintf("100 * avg(%s)", observedUsage),
		},
		{
			metric: "memory_utilization_percent",
			unit:   "percent",
			query:  pairedPercent(memUsed, memPhysical, memElig),
		},
	}
}

func buildContainerTrendSpecs(namespace, pod, container, podUID string) []namedTrendSpec {
	const groupLabels = "node, provider, device_uuid"

	baseSel := fmt.Sprintf("namespace_name=%s,pod_name=%s,container_name=%s",
		strconv.Quote(namespace), strconv.Quote(pod), strconv.Quote(container))
	allocSel := fmt.Sprintf("%s,container_pod_uuid=%s", baseSel, strconv.Quote(container+":"+podUID))

	allocVgpu := fmt.Sprintf("(hami_container_vgpu_allocated{%s} > 0)", allocSel)
	allocMem := fmt.Sprintf("(hami_container_vmemory_allocated{%s} and %s)", allocSel, allocVgpu)
	pairedCore := fmt.Sprintf("(hami_container_core_util{%s} and ignoring (container_pod_uuid) %s)", baseSel, allocVgpu)
	pairedMemUsed := fmt.Sprintf("(hami_container_memory_used{%s} and ignoring (container_pod_uuid) %s)", baseSel, allocMem)

	devices := fmt.Sprintf("max by (%s) (%s)", groupLabels, allocVgpu)
	compute := fmt.Sprintf("avg by (%s) (%s)", groupLabels, pairedCore)
	memAlloc := fmt.Sprintf("max by (%s) (%s)", groupLabels, allocMem)
	memUsed := fmt.Sprintf("avg by (%s) (%s)", groupLabels, pairedMemUsed)

	return []namedTrendSpec{
		{
			metric: "compute_utilization_percent",
			unit:   "percent",
			query: fmt.Sprintf(
				"(avg(%s)) and on () (count(%s) == count(%s))",
				compute, compute, devices,
			),
		},
		{
			metric: "memory_utilization_percent",
			unit:   "percent",
			query: fmt.Sprintf(
				"(100 * sum(%s) / clamp_min(sum(%s), 1)) and on () (count(%s) == count(%s)) and on () (count(%s) == count(%s))",
				memUsed, memAlloc, memAlloc, devices, memUsed, devices,
			),
		},
	}
}
