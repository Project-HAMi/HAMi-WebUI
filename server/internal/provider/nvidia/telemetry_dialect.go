package nvidia

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

type dcgmMetricSpec struct {
	LegacyName        string
	CurrentName       string
	LegacyScale       float64
	CurrentScale      float64
	WorkloadSensitive bool
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

// NewTelemetryDialect returns the TelemetryDialect for NVIDIA GPUs via DCGM.
func NewTelemetryDialect() provider.TelemetryDialect {
	return inventoryDialect{}
}

func (inventoryDialect) SourceName() string   { return "dcgm" }
func (inventoryDialect) MemoryScale() float64 { return float64(provider.BytesPerMiB) }

func (inventoryDialect) BatchKey(device resourceview.Device) (string, bool) {
	return device.Identity.DeviceID, true
}

func (inventoryDialect) QuerySpec(escapedKeys []string) ([]string, string) {
	families := make([]string, 0, len(dcgmPhysicalMetrics)*2)
	for _, spec := range dcgmPhysicalMetrics {
		families = append(families, spec.LegacyName, spec.CurrentName)
	}
	return families, fmt.Sprintf("UUID=~%q,%s", strings.Join(escapedKeys, "|"), dcgmWholeGPUMatchers)
}

func (inventoryDialect) SampleKey(labels map[string]string) string {
	return labels["UUID"]
}

func (inventoryDialect) DeviceKey(device resourceview.Device) string {
	return device.Identity.DeviceID
}

func (inventoryDialect) ReadDevice(
	device resourceview.Device,
	samples provider.TelemetryRawFamilies,
	at time.Time,
	nodeCreated *time.Time,
	targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) provider.PhysicalReadings {
	id := device.Identity
	get := func(name string) provider.TelemetryReading {
		spec := dcgmPhysicalMetrics[name]
		var rows []provider.TelemetrySample
		for _, family := range []string{spec.LegacyName, spec.CurrentName} {
			for _, sample := range samples[family] {
				if nodeCreated == nil || !sample.SampleAt.Before(*nodeCreated) {
					rows = append(rows, sample)
				}
			}
		}
		value, err := selectDCGMPhysical(spec, id.DeviceID, id.Node, at, provider.TelemetryMaxSampleAge, rows, targets)
		return provider.TelemetryReading{Selection: value, Err: err}
	}
	result := provider.PhysicalReadings{
		Util: get("DCGM_FI_DEV_GPU_UTIL"),
		Used: get("DCGM_FI_DEV_FB_USED"),
	}
	free := get("DCGM_FI_DEV_FB_FREE")
	result.Total.Err = errors.Join(result.Used.Err, free.Err)
	if result.Total.Err == nil {
		result.Total.Selection, result.Total.Err = dcgmPhysicalMemoryTotal(result.Used.Selection, free.Selection)
	}
	return result
}

// selectDCGMPhysical selects a physical-card observation, not a workload usage
// source. Multiple scrape targets remain ambiguous even if their values agree.
// Workload-sensitive families require a physical row; only known copy families
// may fold workload-labelled copies from the same entity and original sample.
func selectDCGMPhysical(
	spec dcgmMetricSpec,
	uuid, node string,
	evalAt time.Time,
	maxAge time.Duration,
	candidates []provider.TelemetrySample,
	evidence map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) (provider.TelemetrySelection, error) {
	if uuid == "" || node == "" || evalAt.IsZero() || maxAge <= 0 ||
		spec.LegacyName == "" || spec.CurrentName == "" || spec.LegacyName == spec.CurrentName ||
		!provider.TelemetryFinite(spec.LegacyScale) || !provider.TelemetryFinite(spec.CurrentScale) || spec.LegacyScale <= 0 || spec.CurrentScale <= 0 {
		return provider.TelemetrySelection{}, fmt.Errorf("%w: incomplete selection parameters", provider.ErrTelemetryInvalid)
	}
	if strings.HasPrefix(uuid, "MIG-") {
		return provider.TelemetrySelection{}, fmt.Errorf("%w: requested identity is a MIG instance", provider.ErrTelemetryMissing)
	}
	var selected *provider.TelemetrySelection
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
		if !provider.TelemetryFinite(value) || value < 0 || (spec.LegacyName == "DCGM_FI_DEV_GPU_UTIL" && value > 100) || sample.SampleAt.IsZero() || sample.SampleAt.After(evalAt) {
			rejected = provider.TelemetryRejectReason(rejected, provider.ErrTelemetryInvalid)
			continue
		}
		if evalAt.Sub(sample.SampleAt) > maxAge {
			rejected = provider.TelemetryRejectReason(rejected, provider.ErrTelemetryStale)
			continue
		}
		source := provider.TelemetryTarget{Cluster: sample.Labels["cluster"], Job: sample.Labels["job"], Instance: sample.Labels["instance"]}
		if source.Job == "" || source.Instance == "" {
			rejected = provider.TelemetryRejectReason(rejected, provider.ErrTelemetryInvalid)
			continue
		}
		context, present := evidence[source]
		if !present {
			rejected = provider.TelemetryRejectReason(rejected, provider.ErrTelemetryInvalid)
			continue
		}
		if context.Unavailable != nil {
			rejected = provider.TelemetryRejectReason(rejected, context.Unavailable)
			continue
		}
		if err := provider.ConfirmTelemetryTargetAndNode(sample.Labels, context.TargetLabels, source, node); err != nil {
			rejected = provider.TelemetryRejectReason(rejected, err)
			continue
		}
		if context.Ambiguous {
			return provider.TelemetrySelection{}, fmt.Errorf("%w: multiple active targets share the source identity", provider.ErrTelemetryAmbiguous)
		}
		if spec.WorkloadSensitive && dcgmHasBusinessIdentity(sample.Labels, context) {
			rejected = provider.TelemetryRejectReason(rejected, provider.ErrTelemetryAmbiguous)
			continue
		}
		if err := dcgmMergeIdentity(identities, sample.Labels); err != nil {
			return provider.TelemetrySelection{}, err
		}
		// Same-name copies must agree exactly, even if a previously selected
		// alternate name is within the small unit-conversion rounding tolerance.
		if previous, ok := valuesByName[name]; ok && previous != value {
			return provider.TelemetrySelection{}, fmt.Errorf("%w: same-name copies disagree", provider.ErrTelemetryAmbiguous)
		}
		valuesByName[name] = value
		candidate := provider.TelemetrySelection{Value: value, Source: source, Node: node, RawName: name, SampleAt: sample.SampleAt, Labels: sample.Labels}
		if selected == nil {
			selected = &candidate
			continue
		}
		if selected.Source != candidate.Source || !selected.SampleAt.Equal(candidate.SampleAt) || !sameDCGMValue(*selected, candidate) {
			return provider.TelemetrySelection{}, fmt.Errorf("%w: qualified samples disagree in target, time or value", provider.ErrTelemetryAmbiguous)
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
		return provider.TelemetrySelection{}, rejected
	}
	return provider.TelemetrySelection{}, provider.ErrTelemetryMissing
}

func dcgmHasBusinessIdentity(labels map[string]string, evidence provider.TelemetryTargetEvidence) bool {
	for _, key := range []string{"pod", "exported_pod", "container", "exported_container", "namespace", "exported_namespace", "vgpu", "pod_uid", "exported_pod_uid"} {
		if value := labels[key]; value != "" && evidence.TargetLabels[key] != value {
			return true
		}
	}
	return false
}

func sameDCGMValue(a, b provider.TelemetrySelection) bool {
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
			return fmt.Errorf("%w: %s identity conflicts", provider.ErrTelemetryAmbiguous, field)
		}
		known[field] = value
	}
	return nil
}

// dcgmPhysicalMemoryTotal preserves the existing FREE+USED denominator, and
// never mixes scrape targets, original sample times, nodes or physical devices.
func dcgmPhysicalMemoryTotal(used, free provider.TelemetrySelection) (provider.TelemetrySelection, error) {
	usedSpec, freeSpec := dcgmPhysicalMetrics["DCGM_FI_DEV_FB_USED"], dcgmPhysicalMetrics["DCGM_FI_DEV_FB_FREE"]
	if (used.RawName != usedSpec.LegacyName && used.RawName != usedSpec.CurrentName) ||
		(free.RawName != freeSpec.LegacyName && free.RawName != freeSpec.CurrentName) ||
		!provider.TelemetryFinite(used.Value) || !provider.TelemetryFinite(free.Value) || used.Value < 0 || free.Value < 0 ||
		used.Source.Job == "" || used.Source.Instance == "" || used.SampleAt.IsZero() || used.Labels["UUID"] == "" || used.Node == "" || free.Node == "" {
		return provider.TelemetrySelection{}, provider.ErrTelemetryInvalid
	}
	if used.Source != free.Source || used.Node != free.Node || !used.SampleAt.Equal(free.SampleAt) || used.Labels["UUID"] != free.Labels["UUID"] {
		return provider.TelemetrySelection{}, provider.ErrTelemetryAmbiguous
	}
	// Keep the separately confirmed node when raw samples only had it in their
	// target evidence; do not insert an invented node label into raw labels.
	node := used.Node
	identity := map[string]string{}
	for _, labels := range []map[string]string{used.Labels, free.Labels} {
		if err := dcgmMergeIdentity(identity, labels); err != nil {
			return provider.TelemetrySelection{}, err
		}
		for _, key := range []string{"Hostname", "hostname", "node"} {
			if value := labels[key]; value != "" {
				if node != "" && node != value {
					return provider.TelemetrySelection{}, provider.ErrTelemetryAmbiguous
				}
				node = value
			}
		}
	}
	total := used.Value + free.Value
	if !provider.TelemetryFinite(total) || total <= 0 {
		return provider.TelemetrySelection{}, provider.ErrTelemetryInvalid
	}
	result := used
	result.Value = total
	result.RawName = free.RawName + "+" + used.RawName
	result.Labels = maps.Clone(used.Labels)
	delete(result.Labels, "__name__")
	return result, nil
}
