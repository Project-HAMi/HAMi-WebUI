package provider

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/prometheus/common/model"

	"vgpu/internal/resourceview"
)

const (
	// TelemetryMaxSampleAge bounds how old a physical exporter sample can be
	// before it is treated as stale.
	TelemetryMaxSampleAge = 5 * time.Minute
	BytesPerKiB           = 1024
	BytesPerMiB           = BytesPerKiB * 1024
)

var (
	ErrTelemetryMissing    = errors.New("physical telemetry sample missing")
	ErrTelemetryAmbiguous  = errors.New("physical telemetry sample ambiguous")
	ErrTelemetryStale      = errors.New("physical telemetry sample stale")
	ErrTelemetryInvalid    = errors.New("physical telemetry sample invalid")
	ErrTelemetryTargetDown = errors.New("exporter scrape failed")
)

type TelemetrySample struct {
	Labels   map[string]string
	Value    float64
	SampleAt time.Time
}

type TelemetryTarget struct {
	Cluster  string
	Job      string
	Instance string
}

// TelemetryTargetEvidence holds the verified scrape-target labels and status
// from Prometheus up{} queries rather than guessing from pod names.
type TelemetryTargetEvidence struct {
	TargetLabels map[string]string
	Ambiguous    bool
	Unavailable  error
}

type TelemetrySelection struct {
	Value    float64
	Source   TelemetryTarget
	Node     string
	RawName  string
	SampleAt time.Time
	Labels   map[string]string
}

type TelemetryReading struct {
	Selection TelemetrySelection
	Err       error
}

type TelemetryRawFamilies map[string][]TelemetrySample

type PhysicalReadings struct {
	Util  TelemetryReading
	Used  TelemetryReading
	Total TelemetryReading
}

// TelemetryDialect encapsulates vendor-specific Prometheus batching, sample
// indexing, and physical reading verification behind a uniform contract.
type TelemetryDialect interface {
	// Name returns the stable dialect identifier.
	Name() string
	// ProviderName returns the device Identity.Provider value handled by this dialect.
	ProviderName() string
	// SourceName returns the provenance Source string emitted on telemetry quantities.
	SourceName() string
	// MemoryScale returns the multiplier converting raw memory readings to bytes.
	MemoryScale() float64
	// BatchKey returns the vendor-specific batch grouping key (e.g. DeviceID or Node)
	// and true if device should be queried for physical telemetry.
	BatchKey(device resourceview.Device) (string, bool)
	// QuerySpec returns the metric families and braced-selector inner filter for
	// one chunk of escaped batch keys.
	QuerySpec(escapedKeys []string) (families []string, filter string)
	// SampleKey extracts the indexing key from a raw Prometheus sample's labels.
	SampleKey(labels map[string]string) string
	// DeviceKey computes the matching indexing key from a device's identity.
	DeviceKey(device resourceview.Device) string
	// ReadDevice selects utilization and memory readings for device from its indexed samples.
	ReadDevice(
		device resourceview.Device,
		samples TelemetryRawFamilies,
		at time.Time,
		nodeCreated *time.Time,
		targets map[TelemetryTarget]TelemetryTargetEvidence,
	) PhysicalReadings
}

func FailedPhysicalReadings(err error) PhysicalReadings {
	r := TelemetryReading{Err: err}
	return PhysicalReadings{Util: r, Used: r, Total: r}
}

func TelemetryFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func TelemetryRejectReason(previous, next error) error {
	if previous == nil || errors.Is(next, ErrTelemetryAmbiguous) ||
		(errors.Is(next, ErrTelemetryInvalid) && !errors.Is(previous, ErrTelemetryAmbiguous)) ||
		(errors.Is(next, ErrTelemetryTargetDown) && errors.Is(previous, ErrTelemetryStale)) {
		return next
	}
	return previous
}

func ConfirmTelemetryTargetAndNode(labels, targetLabels map[string]string, source TelemetryTarget, node string) error {
	for key, expected := range map[string]string{"job": source.Job, "instance": source.Instance, "cluster": source.Cluster} {
		if actual := targetLabels[key]; actual != "" && actual != expected {
			return fmt.Errorf("%w: target %s conflicts", ErrTelemetryAmbiguous, key)
		}
	}
	found := ""
	for _, set := range []map[string]string{labels, targetLabels} {
		for _, key := range []string{"Hostname", "hostname", "node"} {
			if value := set[key]; value != "" {
				if value != node || (found != "" && value != found) {
					return fmt.Errorf("%w: node identity conflicts", ErrTelemetryAmbiguous)
				}
				found = value
			}
		}
	}
	if found == "" {
		return fmt.Errorf("%w: node identity missing", ErrTelemetryInvalid)
	}
	return nil
}

// TelemetrySamplesWithTimes pairs one raw selector with timestamp(the same selector).
// Prometheus sets Sample.Timestamp to the evaluation instant; only the companion
// sample's float64 Value contains the original sample time. It must not pass
// through the legacy protobuf float32 value before this conversion.
func TelemetrySamplesWithTimes(metricName string, values, timestamps model.Vector) ([]TelemetrySample, error) {
	if metricName == "" {
		return nil, fmt.Errorf("%w: missing metric name", ErrTelemetryInvalid)
	}
	times := make(map[string]time.Time, len(timestamps))
	for _, sample := range timestamps {
		if sample == nil || sample.Histogram != nil {
			return nil, fmt.Errorf("%w: timestamp is not a float sample", ErrTelemetryInvalid)
		}
		if name := string(sample.Metric[model.MetricNameLabel]); name != "" && name != metricName {
			return nil, fmt.Errorf("%w: timestamp metric name does not match selector", ErrTelemetryInvalid)
		}
		key := telemetryLabelKey(sample.Metric)
		if _, exists := times[key]; exists {
			return nil, fmt.Errorf("%w: duplicate timestamp label set", ErrTelemetryAmbiguous)
		}
		seconds := float64(sample.Value)
		milliseconds := math.Round(seconds * 1000)
		if !TelemetryFinite(seconds) || seconds <= 0 || !TelemetryFinite(milliseconds) || milliseconds >= float64(math.MaxInt64) {
			return nil, fmt.Errorf("%w: invalid original sample time", ErrTelemetryInvalid)
		}
		// Prometheus source samples live on a millisecond grid. Rounding back to
		// that grid avoids float64 epoch fractions becoming fake future samples.
		times[key] = time.UnixMilli(int64(milliseconds)).UTC()
	}
	seen := make(map[string]bool, len(values))
	result := make([]TelemetrySample, 0, len(values))
	for _, sample := range values {
		if sample == nil || sample.Histogram != nil {
			return nil, fmt.Errorf("%w: metric is not a float sample", ErrTelemetryInvalid)
		}
		if string(sample.Metric[model.MetricNameLabel]) != metricName {
			return nil, fmt.Errorf("%w: metric name does not match selector", ErrTelemetryInvalid)
		}
		key := telemetryLabelKey(sample.Metric)
		if seen[key] {
			return nil, fmt.Errorf("%w: duplicate value label set", ErrTelemetryAmbiguous)
		}
		seen[key] = true
		at, ok := times[key]
		if !ok {
			return nil, fmt.Errorf("%w: sample has no matching timestamp", ErrTelemetryInvalid)
		}
		labels := make(map[string]string, len(sample.Metric))
		for name, value := range sample.Metric {
			labels[string(name)] = string(value)
		}
		result = append(result, TelemetrySample{Labels: labels, Value: float64(sample.Value), SampleAt: at})
	}
	if len(seen) != len(times) {
		return nil, fmt.Errorf("%w: timestamp has no matching sample", ErrTelemetryInvalid)
	}
	return result, nil
}

func telemetryLabelKey(metric model.Metric) string {
	labels := metric.Clone()
	delete(labels, model.MetricNameLabel)
	// model.Metric.String quotes values and sorts labels.
	return labels.String()
}

// NodeMetricSchema parameterizes physical sample verification for node exporters
// whose pod-labelled metric copies repeat the same underlying chip measurement.
type NodeMetricSchema struct {
	DeviceIDLabel     string
	IndexLabel        string
	EntityLabel       string
	ModelLabel        string
	UtilizationMetric string
	RequireNodeLabel  bool
	ResolveExportedID func(device resourceview.Device) (string, bool)
	SkipSample        func(metricName string, labels map[string]string) bool
	ValidateMatch     func(device resourceview.Device, labels map[string]string) bool
}

func (s NodeMetricSchema) Reading(
	name string,
	device resourceview.Device,
	samples []TelemetrySample,
	at time.Time,
	nodeCreated *time.Time,
	targets map[TelemetryTarget]TelemetryTargetEvidence,
) TelemetryReading {
	id := device.Identity
	exportedID := id.DeviceID
	if s.ResolveExportedID != nil {
		var ok bool
		exportedID, ok = s.ResolveExportedID(device)
		if !ok {
			return TelemetryReading{Err: ErrTelemetryMissing}
		}
	}

	var result TelemetrySelection
	var rejected error
	for _, sample := range samples {
		labels := sample.Labels
		if labels["__name__"] != name || labels[s.DeviceIDLabel] != exportedID {
			continue
		}
		if s.RequireNodeLabel && labels["node"] != id.Node {
			continue
		}
		if s.SkipSample != nil && s.SkipSample(name, labels) {
			continue
		}
		source := TelemetryTarget{Cluster: labels["cluster"], Job: labels["job"], Instance: labels["instance"]}
		evidence, ok := targets[source]
		if !ok || source.Job == "" || source.Instance == "" {
			rejected = TelemetryRejectReason(rejected, ErrTelemetryInvalid)
			continue
		}
		if evidence.Unavailable != nil {
			rejected = TelemetryRejectReason(rejected, evidence.Unavailable)
			continue
		}
		if sample.SampleAt.IsZero() || sample.SampleAt.After(at) {
			return TelemetryReading{Err: ErrTelemetryInvalid}
		}
		if at.Sub(sample.SampleAt) > TelemetryMaxSampleAge || (nodeCreated != nil && sample.SampleAt.Before(*nodeCreated)) {
			rejected = TelemetryRejectReason(rejected, ErrTelemetryStale)
			continue
		}
		// With honor_labels disabled, a target's node label overrides the
		// exporter's node; Prometheus preserves the latter as exported_node.
		if exporterNode := labels["exported_node"]; exporterNode != "" && exporterNode != id.Node {
			return TelemetryReading{Err: ErrTelemetryAmbiguous}
		}
		if evidence.Ambiguous || ConfirmTelemetryTargetAndNode(labels, evidence.TargetLabels, source, id.Node) != nil ||
			(s.ValidateMatch != nil && !s.ValidateMatch(device, labels)) {
			return TelemetryReading{Err: ErrTelemetryAmbiguous}
		}
		chip, err := strconv.Atoi(labels[s.IndexLabel])
		if err != nil || chip < 0 || !TelemetryFinite(sample.Value) || sample.Value < 0 || (name == s.UtilizationMetric && sample.Value > 100) {
			return TelemetryReading{Err: ErrTelemetryInvalid}
		}
		if !result.SampleAt.IsZero() && (source != result.Source || !sample.SampleAt.Equal(result.SampleAt) ||
			sample.Value != result.Value || labels[s.IndexLabel] != result.Labels[s.IndexLabel] || labels[s.EntityLabel] != result.Labels[s.EntityLabel] || labels[s.ModelLabel] != result.Labels[s.ModelLabel]) {
			return TelemetryReading{Err: ErrTelemetryAmbiguous}
		}
		result = TelemetrySelection{Value: sample.Value, Source: source, Node: id.Node, RawName: name, SampleAt: sample.SampleAt, Labels: labels}
	}
	if result.SampleAt.IsZero() {
		if rejected == nil {
			rejected = ErrTelemetryMissing
		}
		return TelemetryReading{Err: rejected}
	}
	return TelemetryReading{Selection: result}
}

// MemoryPair validates that used and total memory belong to the same physical bank,
// target, and sample collection, and that used does not exceed total.
func (s NodeMetricSchema) MemoryPair(used, total TelemetryReading) (TelemetryReading, TelemetryReading) {
	if total.Err == nil && total.Selection.Value <= 0 {
		return TelemetryReading{Err: ErrTelemetryInvalid}, TelemetryReading{Err: ErrTelemetryInvalid}
	}
	if used.Err == nil && total.Err == nil {
		if !s.SameCollection(used.Selection, total.Selection) {
			return TelemetryReading{Err: ErrTelemetryAmbiguous}, TelemetryReading{Err: ErrTelemetryAmbiguous}
		}
		if used.Selection.Value > total.Selection.Value {
			return TelemetryReading{Err: ErrTelemetryInvalid}, TelemetryReading{Err: ErrTelemetryInvalid}
		}
	}
	return used, total
}

func (s NodeMetricSchema) SameCollection(a, b TelemetrySelection) bool {
	return a.Source == b.Source && a.SampleAt.Equal(b.SampleAt) &&
		a.Labels[s.IndexLabel] == b.Labels[s.IndexLabel] &&
		a.Labels[s.EntityLabel] == b.Labels[s.EntityLabel] &&
		(a.Labels[s.ModelLabel] == "" || b.Labels[s.ModelLabel] == "" || a.Labels[s.ModelLabel] == b.Labels[s.ModelLabel])
}
