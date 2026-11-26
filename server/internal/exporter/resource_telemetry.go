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

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

const sampleTimePrefix = "hami_webui_original_timestamp_"

var errTelemetryQuery = errors.New("physical telemetry query failed")

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
const telemetryIDsPerQuery = 64

type telemetryBatch struct {
	dialect  provider.TelemetryDialect
	families []string
	filter   string
	devices  []resourceview.Device
}

func telemetryPlan(devices []resourceview.Device, dialects []provider.TelemetryDialect) []telemetryBatch {
	var plan []telemetryBatch
	for _, dialect := range dialects {
		if dialect == nil {
			continue
		}
		providerName := dialect.ProviderName()
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
			var ids []string
			var rows []resourceview.Device
			for _, key := range chunk {
				ids = append(ids, regexp.QuoteMeta(key))
				rows = append(rows, byKey[key]...)
			}
			families, filter := dialect.QuerySpec(ids)
			sortedFamilies := slices.Clone(families)
			sort.Strings(sortedFamilies)
			plan = append(plan, telemetryBatch{dialect: dialect, families: sortedFamilies, filter: "{" + filter + "}", devices: rows})
		}
	}
	return plan
}

func (s *MetricsGenerator) refreshResources(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	if err := s.refreshResourceSnapshot(ctx, s.promClient.QueryAt); err != nil && parent.Err() == nil {
		s.log.Warnw("msg", "resource metrics refresh incomplete", "err", err)
	}
}

func (s *MetricsGenerator) refreshResourceSnapshot(ctx context.Context, query queryAtFunc) error {
	snapshot, err := s.resourceSource.Snapshot(ctx)
	if err == nil && snapshotChanged(snapshot) {
		// A Node, Pod or configuration changed during assembly. Retry once on the
		// local informer cache before allowing a gap in this collection.
		snapshot, err = s.resourceSource.Snapshot(ctx)
	}
	if err == nil && snapshotChanged(snapshot) {
		err = errors.New("inventory changed during both reads")
	}
	// Listing a disconnected informer still succeeds against its old cache.
	// The assembly time cannot establish when those nodes were last current.
	// Pod completeness is assessed separately for each card's allocations.
	if err == nil && (!snapshot.NodeCoverage.GetComplete() || resourceview.ValidateCoverage(snapshot.NodeCoverage) != nil) {
		err = errors.New("node inventory continuity is unverified")
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
	var errs []error
	var targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence
	var targetErr error
	if len(plan) > 0 {
		up, err := queryFamilies(ctx, query, []string{"up"}, "", at)
		targetErr = err
		if err == nil {
			targets = targetEvidence(up["up"], at)
		} else {
			errs = append(errs, err)
		}
	}
	for _, batch := range plan {
		raw := provider.TelemetryRawFamilies{}
		batchErr := targetErr
		if batchErr == nil {
			raw, batchErr = queryFamilies(ctx, query, batch.families, batch.filter, at)
			if batchErr != nil {
				errs = append(errs, batchErr)
			}
		}
		// Index by the exporter's entity before resolving per-field aliases.
		byID := map[string]provider.TelemetryRawFamilies{}
		for family, rows := range raw {
			for _, sample := range rows {
				key := batch.dialect.SampleKey(sample.Labels)
				if byID[key] == nil {
					byID[key] = provider.TelemetryRawFamilies{}
				}
				byID[key][family] = append(byID[key][family], sample)
			}
		}
		source := batch.dialect.SourceName()
		memoryScale := batch.dialect.MemoryScale()
		for _, device := range batch.devices {
			id := device.Identity
			key := batch.dialect.DeviceKey(device)
			r := provider.FailedPhysicalReadings(batchErr)
			if batchErr == nil {
				r = batch.dialect.ReadDevice(device, byID[key], at, born[id.NodeUID], targets)
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

func snapshotChanged(snapshot resourceview.Snapshot) bool {
	for _, source := range snapshot.NodeCoverage.GetSources() {
		if source.GetParams()["continuity"] == "source_changed_during_snapshot" {
			return true
		}
	}
	for _, source := range snapshot.DeviceCoverage.GetSources() {
		if source.GetParams()["continuity"] == "source_changed_during_snapshot" {
			return true
		}
	}
	return false
}
