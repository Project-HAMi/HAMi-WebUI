package exporter

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/prometheus/common/model"
)

var (
	errDCGMMissing   = errors.New("DCGM physical sample missing")
	errDCGMAmbiguous = errors.New("DCGM physical sample ambiguous")
	errDCGMStale     = errors.New("DCGM physical sample stale")
	errDCGMInvalid   = errors.New("DCGM physical sample invalid")
	errTargetDown    = errors.New("exporter scrape failed")
)

type dcgmSample struct {
	Labels   map[string]string
	Value    float64
	SampleAt time.Time
}

type dcgmTarget struct {
	Cluster  string
	Job      string
	Instance string
}

// TargetLabels are the actual labels of this Prometheus scrape target, rather
// than labels guessed from an exporter Pod's name.
type dcgmTargetEvidence struct {
	TargetLabels map[string]string
	Ambiguous    bool
	Unavailable  error
}

type dcgmMetricSpec struct {
	LegacyName        string
	CurrentName       string
	LegacyScale       float64
	CurrentScale      float64
	WorkloadSensitive bool
}

type dcgmSelection struct {
	Value    float64
	Source   dcgmTarget
	Node     string
	RawName  string
	SampleAt time.Time
	Labels   map[string]string
}

// DCGM reports MIG instances under the parent UUID. Whole-GPU readings have
// neither an instance ID nor a compute-instance ID.
const dcgmWholeGPUMatchers = `GPU_I_ID="",GPU_CI_ID=""`

// Values retain the existing hami_* units. FB_CAPACITY is deliberately absent:
// its reserved-memory semantics must be verified before changing the denominator.
var dcgmPhysicalMetrics = map[string]dcgmMetricSpec{
	"DCGM_FI_DEV_GPU_UTIL": {"DCGM_FI_DEV_GPU_UTIL", "DCGM_FI_DEV_GPU_UTIL_RATIO", 1, 100, true},
	"DCGM_FI_DEV_FB_USED":  {"DCGM_FI_DEV_FB_USED", "DCGM_FI_DEV_FB_USED_BYTES", 1, 1.0 / (1024 * 1024), true},
	"DCGM_FI_DEV_FB_FREE":  {"DCGM_FI_DEV_FB_FREE", "DCGM_FI_DEV_FB_FREE_BYTES", 1, 1.0 / (1024 * 1024), false},
}

// dcgmSamplesWithTimes pairs one raw selector with timestamp(the same selector).
// Prometheus sets Sample.Timestamp to the evaluation instant; only the companion
// sample's float64 Value contains the original sample time. It must not pass
// through the legacy protobuf float32 value before this conversion.
func dcgmSamplesWithTimes(metricName string, values, timestamps model.Vector) ([]dcgmSample, error) {
	if metricName == "" {
		return nil, fmt.Errorf("%w: missing metric name", errDCGMInvalid)
	}
	times := make(map[string]time.Time, len(timestamps))
	for _, sample := range timestamps {
		if sample == nil || sample.Histogram != nil {
			return nil, fmt.Errorf("%w: timestamp is not a float sample", errDCGMInvalid)
		}
		if name := string(sample.Metric[model.MetricNameLabel]); name != "" && name != metricName {
			return nil, fmt.Errorf("%w: timestamp metric name does not match selector", errDCGMInvalid)
		}
		key := dcgmLabelKey(sample.Metric)
		if _, exists := times[key]; exists {
			return nil, fmt.Errorf("%w: duplicate timestamp label set", errDCGMAmbiguous)
		}
		seconds := float64(sample.Value)
		milliseconds := math.Round(seconds * 1000)
		if !dcgmFinite(seconds) || seconds <= 0 || !dcgmFinite(milliseconds) || milliseconds >= float64(math.MaxInt64) {
			return nil, fmt.Errorf("%w: invalid original sample time", errDCGMInvalid)
		}
		// Prometheus source samples live on a millisecond grid. Rounding back to
		// that grid avoids float64 epoch fractions becoming fake future samples.
		times[key] = time.UnixMilli(int64(milliseconds)).UTC()
	}
	seen := make(map[string]bool, len(values))
	result := make([]dcgmSample, 0, len(values))
	for _, sample := range values {
		if sample == nil || sample.Histogram != nil {
			return nil, fmt.Errorf("%w: metric is not a float sample", errDCGMInvalid)
		}
		if string(sample.Metric[model.MetricNameLabel]) != metricName {
			return nil, fmt.Errorf("%w: metric name does not match selector", errDCGMInvalid)
		}
		key := dcgmLabelKey(sample.Metric)
		if seen[key] {
			return nil, fmt.Errorf("%w: duplicate value label set", errDCGMAmbiguous)
		}
		seen[key] = true
		at, ok := times[key]
		if !ok {
			return nil, fmt.Errorf("%w: sample has no matching timestamp", errDCGMInvalid)
		}
		labels := make(map[string]string, len(sample.Metric))
		for name, value := range sample.Metric {
			labels[string(name)] = string(value)
		}
		result = append(result, dcgmSample{Labels: labels, Value: float64(sample.Value), SampleAt: at})
	}
	if len(seen) != len(times) {
		return nil, fmt.Errorf("%w: timestamp has no matching sample", errDCGMInvalid)
	}
	return result, nil
}

func dcgmLabelKey(metric model.Metric) string {
	labels := metric.Clone()
	delete(labels, model.MetricNameLabel)
	// model.Metric.String quotes values and sorts labels.
	return labels.String()
}

// selectDCGMPhysical selects a physical-card observation, not a workload usage
// source. Multiple scrape targets remain ambiguous even if their values agree.
// Workload-sensitive families require a physical row; only known copy families
// may fold workload-labelled copies from the same entity and original sample.
func selectDCGMPhysical(spec dcgmMetricSpec, uuid, node string, evalAt time.Time, maxAge time.Duration, candidates []dcgmSample, evidence map[dcgmTarget]dcgmTargetEvidence) (dcgmSelection, error) {
	if uuid == "" || node == "" || evalAt.IsZero() || maxAge <= 0 ||
		spec.LegacyName == "" || spec.CurrentName == "" || spec.LegacyName == spec.CurrentName ||
		!dcgmFinite(spec.LegacyScale) || !dcgmFinite(spec.CurrentScale) || spec.LegacyScale <= 0 || spec.CurrentScale <= 0 {
		return dcgmSelection{}, fmt.Errorf("%w: incomplete selection parameters", errDCGMInvalid)
	}
	if strings.HasPrefix(uuid, "MIG-") {
		return dcgmSelection{}, fmt.Errorf("%w: requested identity is a MIG instance", errDCGMMissing)
	}
	var selected *dcgmSelection
	var rejected error
	valuesByName := map[string]float64{}
	identities := map[string]string{}
	for _, sample := range candidates {
		name := sample.Labels["__name__"]
		scale := spec.LegacyScale
		if name == spec.CurrentName {
			scale = spec.CurrentScale
		} else if name != spec.LegacyName {
			continue
		}
		if sample.Labels["UUID"] != uuid || sample.Labels["GPU_I_ID"] != "" || sample.Labels["GPU_CI_ID"] != "" {
			continue
		}
		value := sample.Value * scale
		if !dcgmFinite(value) || value < 0 || (spec.LegacyName == "DCGM_FI_DEV_GPU_UTIL" && value > 100) || sample.SampleAt.IsZero() || sample.SampleAt.After(evalAt) {
			rejected = dcgmRejectReason(rejected, errDCGMInvalid)
			continue
		}
		if evalAt.Sub(sample.SampleAt) > maxAge {
			rejected = dcgmRejectReason(rejected, errDCGMStale)
			continue
		}
		source := dcgmTarget{Cluster: sample.Labels["cluster"], Job: sample.Labels["job"], Instance: sample.Labels["instance"]}
		if source.Job == "" || source.Instance == "" {
			rejected = dcgmRejectReason(rejected, errDCGMInvalid)
			continue
		}
		context, present := evidence[source]
		if !present {
			rejected = dcgmRejectReason(rejected, errDCGMInvalid)
			continue
		}
		if context.Unavailable != nil {
			rejected = dcgmRejectReason(rejected, context.Unavailable)
			continue
		}
		if err := dcgmConfirmTargetAndNode(sample.Labels, context.TargetLabels, source, node); err != nil {
			rejected = dcgmRejectReason(rejected, err)
			continue
		}
		if context.Ambiguous {
			return dcgmSelection{}, fmt.Errorf("%w: multiple active targets share the source identity", errDCGMAmbiguous)
		}
		if spec.WorkloadSensitive && dcgmHasBusinessIdentity(sample.Labels, context) {
			rejected = dcgmRejectReason(rejected, errDCGMAmbiguous)
			continue
		}
		if err := dcgmMergeIdentity(identities, sample.Labels); err != nil {
			return dcgmSelection{}, err
		}
		// Same-name copies must agree exactly, even if a previously selected
		// alternate name is within the small unit-conversion rounding tolerance.
		if previous, ok := valuesByName[name]; ok && previous != value {
			return dcgmSelection{}, fmt.Errorf("%w: same-name copies disagree", errDCGMAmbiguous)
		}
		valuesByName[name] = value
		candidate := dcgmSelection{Value: value, Source: source, Node: node, RawName: name, SampleAt: sample.SampleAt, Labels: sample.Labels}
		if selected == nil {
			selected = &candidate
			continue
		}
		if selected.Source != candidate.Source || !selected.SampleAt.Equal(candidate.SampleAt) || !sameDCGMValue(*selected, candidate) {
			return dcgmSelection{}, fmt.Errorf("%w: qualified samples disagree in target, time or value", errDCGMAmbiguous)
		}
		// A deterministic preference is applied only after proving equivalence.
		if candidate.RawName == spec.CurrentName && selected.RawName != spec.CurrentName {
			selected = &candidate
		}
	}
	if selected != nil {
		// Retain identity from every equivalent copy, including labels omitted by the preferred alias.
		selected.Labels = maps.Clone(selected.Labels)
		maps.Copy(selected.Labels, identities)
		return *selected, nil
	}
	if rejected != nil {
		return dcgmSelection{}, rejected
	}
	return dcgmSelection{}, errDCGMMissing
}

func dcgmConfirmTargetAndNode(labels, targetLabels map[string]string, source dcgmTarget, node string) error {
	for key, expected := range map[string]string{"job": source.Job, "instance": source.Instance, "cluster": source.Cluster} {
		if actual := targetLabels[key]; actual != "" && actual != expected {
			return fmt.Errorf("%w: target %s conflicts", errDCGMAmbiguous, key)
		}
	}
	found := ""
	for _, set := range []map[string]string{labels, targetLabels} {
		for _, key := range []string{"Hostname", "hostname", "node"} {
			if value := set[key]; value != "" {
				if value != node || (found != "" && value != found) {
					return fmt.Errorf("%w: node identity conflicts", errDCGMAmbiguous)
				}
				found = value
			}
		}
	}
	if found == "" {
		return fmt.Errorf("%w: node identity missing", errDCGMInvalid)
	}
	return nil
}

func dcgmHasBusinessIdentity(labels map[string]string, evidence dcgmTargetEvidence) bool {
	for _, key := range []string{"pod", "exported_pod", "container", "exported_container", "namespace", "exported_namespace", "vgpu", "pod_uid", "exported_pod_uid"} {
		if value := labels[key]; value != "" && evidence.TargetLabels[key] != value {
			return true
		}
	}
	return false
}

func sameDCGMValue(a, b dcgmSelection) bool {
	if a.Value == b.Value {
		return true
	}
	if a.RawName == b.RawName {
		return false
	}
	// Unit conversions such as 0.29*100 introduce a few float64 ULPs. This
	// tolerance only reconciles new/old names, never disagreeing raw copies.
	aULP := math.Abs(math.Nextafter(a.Value, 0) - a.Value)
	bULP := math.Abs(math.Nextafter(b.Value, 0) - b.Value)
	return math.Abs(a.Value-b.Value) <= 4*math.Max(aULP, bULP)
}

func dcgmMergeIdentity(known, labels map[string]string) error {
	for _, key := range []string{"device", "gpu", "modelName", "DRIVER_VERSION", "DCGM_FI_DRIVER_VERSION", "DCGM_FI_SYSTEM_DRIVER_VERSION"} {
		value := labels[key]
		if value == "" {
			continue
		}
		field := key
		if key == "DCGM_FI_DRIVER_VERSION" || key == "DCGM_FI_SYSTEM_DRIVER_VERSION" {
			field = "DRIVER_VERSION"
		}
		if previous := known[field]; previous != "" && previous != value {
			return fmt.Errorf("%w: %s identity conflicts", errDCGMAmbiguous, field)
		}
		known[field] = value
	}
	return nil
}

func dcgmRejectReason(previous, next error) error {
	if previous == nil || errors.Is(next, errDCGMAmbiguous) ||
		(errors.Is(next, errDCGMInvalid) && !errors.Is(previous, errDCGMAmbiguous)) ||
		(errors.Is(next, errTargetDown) && errors.Is(previous, errDCGMStale)) {
		return next
	}
	return previous
}

func dcgmFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// dcgmPhysicalMemoryTotal preserves the existing FREE+USED denominator, and
// never mixes scrape targets, original sample times, nodes or physical devices.
func dcgmPhysicalMemoryTotal(used, free dcgmSelection) (dcgmSelection, error) {
	usedSpec, freeSpec := dcgmPhysicalMetrics["DCGM_FI_DEV_FB_USED"], dcgmPhysicalMetrics["DCGM_FI_DEV_FB_FREE"]
	if (used.RawName != usedSpec.LegacyName && used.RawName != usedSpec.CurrentName) ||
		(free.RawName != freeSpec.LegacyName && free.RawName != freeSpec.CurrentName) ||
		!dcgmFinite(used.Value) || !dcgmFinite(free.Value) || used.Value < 0 || free.Value < 0 ||
		used.Source.Job == "" || used.Source.Instance == "" || used.SampleAt.IsZero() || used.Labels["UUID"] == "" || used.Node == "" || free.Node == "" {
		return dcgmSelection{}, errDCGMInvalid
	}
	if used.Source != free.Source || used.Node != free.Node || !used.SampleAt.Equal(free.SampleAt) || used.Labels["UUID"] != free.Labels["UUID"] {
		return dcgmSelection{}, errDCGMAmbiguous
	}
	// Keep the separately confirmed node when raw samples only had it in their
	// target evidence; do not insert an invented node label into raw labels.
	node := used.Node
	identity := map[string]string{}
	for _, labels := range []map[string]string{used.Labels, free.Labels} {
		if err := dcgmMergeIdentity(identity, labels); err != nil {
			return dcgmSelection{}, err
		}
		for _, key := range []string{"Hostname", "hostname", "node"} {
			if value := labels[key]; value != "" {
				if node != "" && node != value {
					return dcgmSelection{}, errDCGMAmbiguous
				}
				node = value
			}
		}
	}
	total := used.Value + free.Value
	if !dcgmFinite(total) || total <= 0 {
		return dcgmSelection{}, errDCGMInvalid
	}
	result := used
	result.Value = total
	result.RawName = free.RawName + "+" + used.RawName
	result.Labels = maps.Clone(used.Labels)
	delete(result.Labels, "__name__")
	return result, nil
}
