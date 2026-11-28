package exporter

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

const sampleTimePrefix = "hami_webui_original_timestamp_"

var (
	errTelemetryQuery       = errors.New("physical telemetry query failed")
	errTelemetrySeriesLimit = errors.New("physical telemetry batch exceeded series limit")
)

type queryAtFunc func(context.Context, string, time.Time) (model.Value, error)

// Apply timestamp directly to each raw selector. An expression such as
// timestamp(label_replace(selector, ...)) reports the query evaluation time.
// Distinct names on the timestamp rows let families share one round trip even
// when two raw families have identical labels.
func queryFamilies(ctx context.Context, query queryAtFunc, families []string, filter string, at time.Time) (provider.TelemetryRawFamilies, error) {
	var parts []string
	for _, family := range families {
		selector := family + filter
		timestamp := fmt.Sprintf("label_replace(timestamp(%s), %q, %q, %q, %q)", selector, "__name__", sampleTimePrefix+family, "", "")
		parts = append(parts, selector, timestamp)
	}
	expr := strings.Join(parts, " or on(__name__) ")
	value, err := query(ctx, expr, at)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errTelemetryQuery, err)
	}
	rows, ok := value.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("%w: expected an instant vector", errTelemetryQuery)
	}
	type pair struct{ values, timestamps model.Vector }
	byName := map[string]*pair{}
	seriesCount := 0
	for _, r := range rows {
		if r == nil || r.Histogram != nil {
			return nil, provider.ErrTelemetryInvalid
		}
		name := string(r.Metric[model.MetricNameLabel])
		family, isTime := strings.CutPrefix(name, sampleTimePrefix)
		if family == "" {
			return nil, provider.ErrTelemetryInvalid
		}
		p := byName[family]
		if p == nil {
			p = &pair{}
			byName[family] = p
		}
		if isTime {
			row := *r
			row.Metric = r.Metric.Clone()
			delete(row.Metric, model.MetricNameLabel)
			p.timestamps = append(p.timestamps, &row)
		} else {
			seriesCount++
			if seriesCount > telemetryMaxBatchSeries {
				return nil, fmt.Errorf("%w: %w", errTelemetryQuery, errTelemetrySeriesLimit)
			}
			p.values = append(p.values, r)
		}
	}
	result := provider.TelemetryRawFamilies{}
	for name, p := range byName {
		samples, err := provider.TelemetrySamplesWithTimes(name, p.values, p.timestamps)
		if err != nil {
			return nil, err
		}
		result[name] = samples
	}
	return result, nil
}

func targetEvidence(samples []provider.TelemetrySample, at time.Time) map[provider.TelemetryTarget]provider.TelemetryTargetEvidence {
	result := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{}
	for _, s := range samples {
		l := s.Labels
		target := provider.TelemetryTarget{Cluster: l["cluster"], Job: l["job"], Instance: l["instance"]}
		if target.Job == "" || target.Instance == "" {
			continue
		}
		if _, seen := result[target]; seen {
			result[target] = provider.TelemetryTargetEvidence{Ambiguous: true}
			continue
		}
		evidence := provider.TelemetryTargetEvidence{TargetLabels: l}
		switch {
		case s.SampleAt.IsZero() || s.SampleAt.After(at) || (s.Value != 0 && s.Value != 1):
			evidence.Unavailable = provider.ErrTelemetryInvalid
		case at.Sub(s.SampleAt) > provider.TelemetryMaxSampleAge:
			evidence.Unavailable = provider.ErrTelemetryStale
		case s.Value == 0:
			evidence.Unavailable = provider.ErrTelemetryTargetDown
		}
		result[target] = evidence
	}
	return result
}

// Issue one scoped selection per chunk instead of one request per card or
// workload. A failed chunk cannot make unrelated nodes or vendors disappear.
const (
	telemetryIDsPerQuery         = 64
	telemetryFallbackIDsPerQuery = 16
	telemetryMaxBatchSeries      = 20_000
	telemetryBatchTimeout        = 3 * time.Second
)

type telemetryKeyGroup struct {
	key     string
	devices []resourceview.Device
}

type telemetryBatch struct {
	dialect provider.TelemetryDialect
	groups  []telemetryKeyGroup
}

func telemetryPlan(devices []resourceview.Device, dialects []provider.TelemetryDialect) []telemetryBatch {
	var plan []telemetryBatch
	for _, dialect := range dialects {
		if dialect == nil {
			continue
		}
		providerName := dialect.Name()
		byKey := map[string][]resourceview.Device{}
		for _, d := range devices {
			id := d.Identity
			if d.Kind != "physical" || id.NodeUID == "" || id.Node == "" || id.DeviceID == "" || id.Provider != providerName {
				continue
			}
			key, ok := dialect.BatchKey(d)
			if !ok {
				continue
			}
			byKey[key] = append(byKey[key], d)
		}
		var keys []string
		for key := range byKey {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for start := 0; start < len(keys); start += telemetryIDsPerQuery {
			chunk := keys[start:min(start+telemetryIDsPerQuery, len(keys))]
			groups := make([]telemetryKeyGroup, 0, len(chunk))
			for _, key := range chunk {
				groups = append(groups, telemetryKeyGroup{key: key, devices: byKey[key]})
			}
			plan = append(plan, telemetryBatch{dialect: dialect, groups: groups})
		}
	}
	return plan
}

func buildBatchSpec(dialect provider.TelemetryDialect, groups []telemetryKeyGroup) (families []string, filter string, devices []resourceview.Device) {
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, regexp.QuoteMeta(g.key))
		devices = append(devices, g.devices...)
	}
	rawFamilies, rawFilter := dialect.QuerySpec(ids)
	families = slices.Clone(rawFamilies)
	sort.Strings(families)
	return families, "{" + rawFilter + "}", devices
}

func groupDevices(groups []telemetryKeyGroup) []resourceview.Device {
	var devices []resourceview.Device
	for _, g := range groups {
		devices = append(devices, g.devices...)
	}
	return devices
}

func batchSplitReason(groups []telemetryKeyGroup, parentCtx, chunkCtx context.Context, err error) (int, bool) {
	if err == nil || len(groups) <= telemetryFallbackIDsPerQuery || parentCtx.Err() != nil || errors.Is(err, prom.ErrCircuitOpen) {
		return 0, false
	}
	switch {
	case errors.Is(err, prom.ErrResponseTooLarge):
		return splitReasonOverflow, true
	case errors.Is(err, errTelemetrySeriesLimit):
		return splitReasonSeriesLimit, true
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(chunkCtx.Err(), context.DeadlineExceeded):
		return splitReasonTimeout, true
	default:
		return 0, false
	}
}

func queryChunk(
	parent context.Context,
	query queryAtFunc,
	dialect provider.TelemetryDialect,
	groups []telemetryKeyGroup,
	at time.Time,
	timeout time.Duration,
) (provider.TelemetryRawFamilies, []resourceview.Device, context.Context, error) {
	families, filter, devices := buildBatchSpec(dialect, groups)
	chunkCtx, cancel := context.WithTimeout(parent, timeout)
	raw, err := queryFamilies(chunkCtx, query, families, filter, at)
	cancel()
	return raw, devices, chunkCtx, err
}

const (
	snapshotRetryAttempts      = 12
	allocSnapshotRetryAttempts = 2
	snapshotRetryDelay         = 200 * time.Millisecond
)

func (s *MetricsGenerator) refreshResourceSnapshot(ctx context.Context, query queryAtFunc) error {
	snapshot, err := s.resourceSource.Snapshot(ctx)
	allocRetries := 0
	for range snapshotRetryAttempts {
		if err != nil {
			break
		}
		nodeValid := snapshot.NodeCoverage.GetComplete() && resourceview.ValidateCoverage(snapshot.NodeCoverage) == nil
		transientNode, transientAlloc := transientContinuityScope(snapshot)
		if !nodeValid {
			if !transientNode {
				break
			}
		} else {
			if !transientAlloc || allocRetries >= allocSnapshotRetryAttempts {
				break
			}
			if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= snapshotRetryDelay*3 {
				break
			}
			allocRetries++
		}
		timer := time.NewTimer(snapshotRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			err = ctx.Err()
		case <-timer.C:
			snapshot, err = s.resourceSource.Snapshot(ctx)
			if err == nil {
				continue
			}
		}
		break
	}
	// Listing a disconnected informer still succeeds against its old cache.
	// The assembly time cannot establish when those nodes were last current.
	// Pod completeness is assessed separately for each card's allocations.
	if err == nil && (!snapshot.NodeCoverage.GetComplete() || resourceview.ValidateCoverage(snapshot.NodeCoverage) != nil) {
		if transientNode, _ := transientContinuityScope(snapshot); transientNode {
			err = errors.New("inventory changed during retries")
		} else {
			err = errors.New("node inventory continuity is unverified")
		}
	}
	if err != nil {
		s.observationsMu.Lock()
		s.resourceSnapshot = nil
		s.physicalGroups = nil
		s.observations = nil
		s.observationsMu.Unlock()
		return err
	}
	// Prometheus stores and evaluates at millisecond precision. Select the same
	// boundary for upstream queries and local freshness checks.
	at := s.clockNow().Truncate(time.Millisecond)
	observations := resourceview.DeviceObservations{}
	previous := s.publishedObservations()
	born := map[types.UID]*time.Time{}
	for _, node := range snapshot.Nodes {
		born[node.UID] = node.CreatedAt
	}
	plan := telemetryPlan(snapshot.Devices, s.activeTelemetryDialects())
	type executedChunk struct {
		dialect provider.TelemetryDialect
		devices []resourceview.Device
		byID    map[string]provider.TelemetryRawFamilies
		err     error
	}
	var (
		executed    []executedChunk
		errs        []error
		circuitErr  error
		instanceSet = map[string]struct{}{}
	)
	chunkTimeout := s.telemetryChunkTimeout()
	recordChunk := func(dialect provider.TelemetryDialect, devices []resourceview.Device, raw provider.TelemetryRawFamilies, batchErr error) {
		if batchErr != nil {
			errs = append(errs, batchErr)
			if errors.Is(batchErr, prom.ErrCircuitOpen) && circuitErr == nil {
				circuitErr = batchErr
			}
			executed = append(executed, executedChunk{dialect: dialect, devices: devices, err: batchErr})
			return
		}
		// Index by the exporter's entity before resolving per-field aliases.
		byID := map[string]provider.TelemetryRawFamilies{}
		for family, rows := range raw {
			for _, sample := range rows {
				if inst := sample.Labels["instance"]; inst != "" {
					instanceSet[inst] = struct{}{}
				}
				key := dialect.SampleKey(sample.Labels)
				if byID[key] == nil {
					byID[key] = provider.TelemetryRawFamilies{}
				}
				byID[key][family] = append(byID[key][family], sample)
			}
		}
		executed = append(executed, executedChunk{dialect: dialect, devices: devices, byID: byID})
	}
	for _, batch := range plan {
		if circuitErr != nil {
			executed = append(executed, executedChunk{dialect: batch.dialect, devices: groupDevices(batch.groups), err: circuitErr})
			continue
		}
		raw, devices, chunkCtx, batchErr := queryChunk(ctx, query, batch.dialect, batch.groups, at, chunkTimeout)
		if reason, split := batchSplitReason(batch.groups, ctx, chunkCtx, batchErr); split {
			s.splitCounts[reason].Add(1)
			for start := 0; start < len(batch.groups); start += telemetryFallbackIDsPerQuery {
				subgroups := batch.groups[start:min(start+telemetryFallbackIDsPerQuery, len(batch.groups))]
				if circuitErr != nil {
					executed = append(executed, executedChunk{dialect: batch.dialect, devices: groupDevices(subgroups), err: circuitErr})
					continue
				}
				subRaw, subDevices, _, subErr := queryChunk(ctx, query, batch.dialect, subgroups, at, chunkTimeout)
				recordChunk(batch.dialect, subDevices, subRaw, subErr)
			}
			continue
		}
		recordChunk(batch.dialect, devices, raw, batchErr)
	}
	instances := make([]string, 0, len(instanceSet))
	for inst := range instanceSet {
		instances = append(instances, inst)
	}
	sort.Strings(instances)
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{}
	failedInstances := map[string]error{}
	for start := 0; start < len(instances); start += telemetryIDsPerQuery {
		chunk := instances[start:min(start+telemetryIDsPerQuery, len(instances))]
		if circuitErr != nil {
			for _, inst := range chunk {
				failedInstances[inst] = circuitErr
			}
			continue
		}
		escaped := make([]string, len(chunk))
		for i, inst := range chunk {
			escaped[i] = regexp.QuoteMeta(inst)
		}
		filter := fmt.Sprintf("{instance=~%q}", strings.Join(escaped, "|"))
		upCtx, upCancel := context.WithTimeout(ctx, chunkTimeout)
		up, upErr := queryFamilies(upCtx, query, []string{"up"}, filter, at)
		upCancel()
		if upErr != nil {
			errs = append(errs, upErr)
			if errors.Is(upErr, prom.ErrCircuitOpen) {
				circuitErr = upErr
			}
			for _, inst := range chunk {
				failedInstances[inst] = upErr
			}
			continue
		}
		for target, evidence := range targetEvidence(up["up"], at) {
			targets[target] = evidence
		}
	}
	for _, chunk := range executed {
		source := chunk.dialect.SourceName()
		memoryScale := chunk.dialect.MemoryScale()
		for _, device := range chunk.devices {
			id := device.Identity
			key := chunk.dialect.DeviceKey(device)
			var r provider.PhysicalReadings
			if chunk.err != nil {
				r = provider.FailedPhysicalReadings(chunk.err)
			} else if instErr := sampleInstanceError(chunk.byID[key], failedInstances); instErr != nil {
				r = provider.FailedPhysicalReadings(instErr)
			} else {
				r = chunk.dialect.ReadDevice(device, chunk.byID[key], at, born[id.NodeUID], targets)
			}
			old := previous[id]
			observations[id] = resourceview.DeviceObservation{
				UtilizationRatio:    observationQuantity(r.Util, .01, "ratio", source, old.UtilizationRatio, at),
				MemoryUsedBytes:     observationQuantity(r.Used, memoryScale, "bytes", source, old.MemoryUsedBytes, at),
				MemoryPhysicalBytes: observationQuantity(r.Total, memoryScale, "bytes", source, old.MemoryPhysicalBytes, at),
			}
		}
	}
	groups := resourceview.GroupPhysicalDevices(snapshot).Groups
	s.observationsMu.Lock()
	s.physicalGroups = groups
	s.resourceSnapshot = &snapshot
	s.observations = observations
	s.observationsMu.Unlock()
	return errors.Join(errs...)
}

func sampleInstanceError(samples provider.TelemetryRawFamilies, failedInstances map[string]error) error {
	if len(failedInstances) == 0 {
		return nil
	}
	for _, rows := range samples {
		for _, sample := range rows {
			if err, failed := failedInstances[sample.Labels["instance"]]; failed {
				return err
			}
		}
	}
	return nil
}

func transientContinuityScope(snapshot resourceview.Snapshot) (nodeTransient, allocTransient bool) {
	hasTransientSource := func(c *viewv1.Coverage) bool {
		if c == nil || c.GetComplete() {
			return false
		}
		for _, source := range c.GetSources() {
			switch source.GetParams()["continuity"] {
			case "source_changed_during_snapshot",
				"consumer_pending_or_changed",
				"relist_required",
				"relisting",
				"awaiting_watch",
				"dependent_fact_replay_pending":
				return true
			}
		}
		return false
	}
	nodeTransient = hasTransientSource(snapshot.NodeCoverage) || hasTransientSource(snapshot.DeviceCoverage)
	for i := range snapshot.Devices {
		if hasTransientSource(snapshot.Devices[i].AllocationCoverage) {
			allocTransient = true
			break
		}
	}
	return nodeTransient, allocTransient
}
