package exporter

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"k8s.io/apimachinery/pkg/types"

	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

const dcgmMaxSampleAge = 5 * time.Minute
const sampleTimePrefix = "hami_webui_original_timestamp_"

var errTelemetryQuery = errors.New("physical telemetry query failed")

type dcgmReading struct {
	selection dcgmSelection
	err       error
}
type rawFamilies map[string][]dcgmSample
type queryAtFunc func(context.Context, string, time.Time) (model.Value, error)

// Apply timestamp directly to each raw selector. An expression such as
// timestamp(label_replace(selector, ...)) reports the query evaluation time.
// Distinct names on the timestamp rows let families share one round trip even
// when two raw families have identical labels.
func queryFamilies(ctx context.Context, query queryAtFunc, families []string, filter string, at time.Time) (rawFamilies, error) {
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
			return nil, errDCGMInvalid
		}
		name := string(r.Metric[model.MetricNameLabel])
		family, isTime := strings.CutPrefix(name, sampleTimePrefix)
		if family == "" {
			return nil, errDCGMInvalid
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
	result := rawFamilies{}
	for name, p := range byName {
		samples, err := dcgmSamplesWithTimes(name, p.values, p.timestamps)
		if err != nil {
			return nil, err
		}
		result[name] = samples
	}
	return result, nil
}

func targetEvidence(samples []dcgmSample, at time.Time) map[dcgmTarget]dcgmTargetEvidence {
	result := map[dcgmTarget]dcgmTargetEvidence{}
	for _, s := range samples {
		l := s.Labels
		target := dcgmTarget{Cluster: l["cluster"], Job: l["job"], Instance: l["instance"]}
		if target.Job == "" || target.Instance == "" {
			continue
		}
		if _, seen := result[target]; seen {
			result[target] = dcgmTargetEvidence{Ambiguous: true}
			continue
		}
		evidence := dcgmTargetEvidence{TargetLabels: l}
		switch {
		case s.SampleAt.IsZero() || s.SampleAt.After(at) || (s.Value != 0 && s.Value != 1):
			evidence.Unavailable = errDCGMInvalid
		case at.Sub(s.SampleAt) > dcgmMaxSampleAge:
			evidence.Unavailable = errDCGMStale
		case s.Value == 0:
			evidence.Unavailable = errTargetDown
		}
		result[target] = evidence
	}
	return result
}

// Issue one scoped selection per chunk instead of one request per card or
// workload. A failed chunk cannot make unrelated nodes or vendors disappear.
const telemetryIDsPerQuery = 64

type telemetryBatch struct {
	families []string
	filter   string
	devices  []resourceview.Device
}

func telemetryPlan(devices []resourceview.Device) []telemetryBatch {
	var plan []telemetryBatch
	for _, provider := range []string{biz.NvidiaGPUDevice, biz.AscendGPUDevice, biz.HygonHCUDevice, biz.HygonGPUDevice} {
		byKey := map[string][]resourceview.Device{}
		for _, d := range devices {
			id := d.Identity
			if d.Kind != "physical" || id.NodeUID == "" || id.Node == "" || id.DeviceID == "" || id.Provider != provider {
				continue
			}
			key := id.DeviceID
			if provider != biz.NvidiaGPUDevice {
				key = id.Node
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
			var families []string
			filter := fmt.Sprintf("UUID=~%q,%s", strings.Join(ids, "|"), dcgmWholeGPUMatchers)
			switch provider {
			case biz.NvidiaGPUDevice:
				for _, spec := range dcgmPhysicalMetrics {
					families = append(families, spec.LegacyName, spec.CurrentName)
				}
			case biz.AscendGPUDevice:
				families = []string{ascendUtilization, "npu_chip_info_hbm_used_memory", "npu_chip_info_hbm_total_memory", "npu_chip_info_used_memory", "npu_chip_info_total_memory"}
				filter = fmt.Sprintf("node=~%q,v_dev_id=\"\",is_virtual!=\"true\"", strings.Join(ids, "|"))
			case biz.HygonHCUDevice, biz.HygonGPUDevice:
				util, used, total := hygonMetricNames(provider)
				families = []string{util, used, total}
				filter = fmt.Sprintf("node=~%q", strings.Join(ids, "|"))
			}
			sort.Strings(families)
			plan = append(plan, telemetryBatch{families, "{" + filter + "}", rows})
		}
	}
	return plan
}

func hygonMetricNames(provider string) (utilization, used, total string) {
	prefix := strings.ToLower(provider)
	return prefix + "_utilizationrate", prefix + "_usedmemory_bytes", prefix + "_memorycap_bytes"
}

type physicalReadings struct{ util, used, total dcgmReading }

func failedReadings(err error) physicalReadings {
	r := dcgmReading{err: err}
	return physicalReadings{r, r, r}
}
func deviceReadings(d resourceview.Device, samples rawFamilies, at time.Time, born *time.Time, targets map[dcgmTarget]dcgmTargetEvidence) physicalReadings {
	id := d.Identity
	if id.Provider == biz.AscendGPUDevice {
		get := func(name string) dcgmReading { return ascendLabels.reading(name, d, samples[name], at, born, targets) }
		readings := failedReadings(errDCGMMissing)
		readings.util = get(ascendUtilization)
		if used, total := ascendMemoryFamilies(d.Model); used != "" {
			readings.used, readings.total = ascendLabels.memoryPair(get(used), get(total))
		}
		return readings
	}
	if id.Provider == biz.HygonHCUDevice || id.Provider == biz.HygonGPUDevice {
		utilName, usedName, totalName := hygonMetricNames(id.Provider)
		schema := hygonLabels
		schema.utilization = utilName
		get := func(name string) dcgmReading { return schema.reading(name, d, samples[name], at, born, targets) }
		used, total := schema.memoryPair(get(usedName), get(totalName))
		return physicalReadings{get(utilName), used, total}
	}
	get := func(name string) dcgmReading {
		spec := dcgmPhysicalMetrics[name]
		var rows []dcgmSample
		for _, family := range []string{spec.LegacyName, spec.CurrentName} {
			for _, sample := range samples[family] {
				if born == nil || !sample.SampleAt.Before(*born) {
					rows = append(rows, sample)
				}
			}
		}
		value, err := selectDCGMPhysical(spec, id.DeviceID, id.Node, at, dcgmMaxSampleAge, rows, targets)
		return dcgmReading{value, err}
	}
	result := physicalReadings{util: get("DCGM_FI_DEV_GPU_UTIL"), used: get("DCGM_FI_DEV_FB_USED")}
	free := get("DCGM_FI_DEV_FB_FREE")
	result.total.err = errors.Join(result.used.err, free.err)
	if result.total.err == nil {
		result.total.selection, result.total.err = dcgmPhysicalMemoryTotal(result.used.selection, free.selection)
	}
	return result
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
	plan := telemetryPlan(snapshot.Devices)
	var errs []error
	var targets map[dcgmTarget]dcgmTargetEvidence
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
		raw := rawFamilies{}
		batchErr := targetErr
		if batchErr == nil {
			raw, batchErr = queryFamilies(ctx, query, batch.families, batch.filter, at)
			if batchErr != nil {
				errs = append(errs, batchErr)
			}
		}
		// Index by the exporter's entity before resolving per-field aliases.
		byID := map[string]rawFamilies{}
		for family, rows := range raw {
			for _, sample := range rows {
				key := sample.Labels["UUID"]
				switch provider := batch.devices[0].Identity.Provider; provider {
				case biz.HygonHCUDevice, biz.HygonGPUDevice:
					key = sample.Labels["node"] + "\x00" + provider + "-" + sample.Labels["device_id"]
				case biz.AscendGPUDevice:
					key = sample.Labels["node"] + "\x00" + sample.Labels["vdie_id"]
				}
				if byID[key] == nil {
					byID[key] = rawFamilies{}
				}
				byID[key][family] = append(byID[key][family], sample)
			}
		}
		for _, device := range batch.devices {
			id := device.Identity
			source := "dcgm"
			key := id.DeviceID
			memoryScale := float64(bytesPerMiB)
			switch id.Provider {
			case biz.HygonHCUDevice, biz.HygonGPUDevice:
				source = strings.ToLower(id.Provider) + "_exporter"
				key = id.Node + "\x00" + id.DeviceID
				memoryScale = 1
			case biz.AscendGPUDevice:
				source = ascendSource
				key = id.Node + "\x00" + id.DeviceID
			}
			r := failedReadings(batchErr)
			if batchErr == nil {
				r = deviceReadings(device, byID[key], at, born[id.NodeUID], targets)
			}
			old := previous[id]
			observations[id] = resourceview.DeviceObservation{
				UtilizationRatio:    observationQuantity(r.util, .01, "ratio", source, old.UtilizationRatio, at),
				MemoryUsedBytes:     observationQuantity(r.used, memoryScale, "bytes", source, old.MemoryUsedBytes, at),
				MemoryPhysicalBytes: observationQuantity(r.total, memoryScale, "bytes", source, old.MemoryPhysicalBytes, at),
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
