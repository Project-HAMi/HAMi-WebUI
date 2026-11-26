package metax

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

const (
	metaxState       = "mx_gpu_state"
	metaxUtilization = "mx_gpu_usage"
	metaxUsed        = "mx_memory_used"
	metaxTotal       = "mx_memory_total"
)

var metaxSchema = provider.NodeMetricSchema{
	DeviceIDLabel:     "uuid",
	IndexLabel:        "deviceId",
	EntityLabel:       "die_id",
	ModelLabel:        "modelName",
	UtilizationMetric: metaxUtilization,
	SkipSample: func(name string, labels map[string]string) bool {
		return (name == metaxUsed || name == metaxTotal) && labels["type"] != "vram"
	},
	ValidateMatch: metaxChipMatches,
}

// NewTelemetryDialect returns the TelemetryDialect for MetaX C500 GPUs via mxExporter.
func NewTelemetryDialect() provider.TelemetryDialect {
	return sgpuDialect{}
}

func (sgpuDialect) ProviderName() string { return metaxProviderName }
func (sgpuDialect) SourceName() string   { return "mx_exporter" }
func (sgpuDialect) MemoryScale() float64 { return float64(provider.BytesPerKiB) }

func (sgpuDialect) BatchKey(device resourceview.Device) (string, bool) {
	if !metaxC500Candidate(device.Model) {
		return "", false
	}
	return device.Identity.DeviceID, true
}

func (sgpuDialect) QuerySpec(escapedKeys []string) ([]string, string) {
	families := []string{metaxState, metaxUtilization, metaxUsed, metaxTotal}
	filter := fmt.Sprintf("uuid=~%q,type=~%q", strings.Join(escapedKeys, "|"), "|vram")
	return families, filter
}

func (sgpuDialect) SampleKey(labels map[string]string) string {
	return labels["uuid"]
}

func (sgpuDialect) DeviceKey(device resourceview.Device) string {
	return device.Identity.DeviceID
}

func metaxChipMatches(d resourceview.Device, l map[string]string) bool {
	return l["die_id"] == "0" && metaxC500(l["modelName"]) && l["Hostname"] == d.Identity.Node &&
		(l["exported_Hostname"] == "" || l["exported_Hostname"] == d.Identity.Node)
}

func metaxC500(model string) bool {
	return strings.EqualFold(model, "C500") || strings.EqualFold(model, "MXC500")
}

// The sGPU plugin can put a sharing mode in the registration's model field.
func metaxC500Candidate(model string) bool {
	return metaxC500(model) || model == "" || model == "sgpu" || model == "native"
}

func (sgpuDialect) ReadDevice(
	device resourceview.Device,
	samples provider.TelemetryRawFamilies,
	at time.Time,
	born *time.Time,
	targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) provider.PhysicalReadings {
	// Without a registered product name, let the exporter's parent-card
	// state identify the model before applying the C500 single-die contract.
	if !metaxC500(device.Model) && !slices.ContainsFunc(samples[metaxState], func(s provider.TelemetrySample) bool {
		return s.Labels["uuid"] == device.Identity.DeviceID && metaxC500(s.Labels["modelName"])
	}) {
		return provider.FailedPhysicalReadings(provider.ErrTelemetryMissing)
	}
	read := func(name string) provider.TelemetryReading {
		return metaxSchema.Reading(name, device, samples[name], at, born, targets)
	}
	// mxExporter enumerates GPU state for each die. Do not promote a lone
	// utilization sample to a whole-card reading when another die is known.
	state := read(metaxState)
	if state.Err != nil {
		return provider.FailedPhysicalReadings(state.Err)
	}
	if state.Selection.Value == 0 {
		return provider.FailedPhysicalReadings(provider.ErrTelemetryMissing)
	}
	if state.Selection.Value != 1 {
		return provider.FailedPhysicalReadings(provider.ErrTelemetryInvalid)
	}
	// A collector can lose one die's state while its other fields are still
	// present. Its siblings must not then look like an entire C500.
	for _, rows := range samples {
		for _, sample := range rows {
			l := sample.Labels
			target := provider.TelemetryTarget{Cluster: l["cluster"], Job: l["job"], Instance: l["instance"]}
			if l["uuid"] == device.Identity.DeviceID && target == state.Selection.Source && sample.SampleAt.Equal(state.Selection.SampleAt) && l["die_id"] != "0" {
				return provider.FailedPhysicalReadings(provider.ErrTelemetryAmbiguous)
			}
		}
	}
	onCard := func(name string) provider.TelemetryReading {
		r := read(name)
		if r.Err == nil && !metaxSchema.SameCollection(state.Selection, r.Selection) {
			return provider.TelemetryReading{Err: provider.ErrTelemetryAmbiguous}
		}
		return r
	}
	used, total := metaxSchema.MemoryPair(onCard(metaxUsed), onCard(metaxTotal))
	return provider.PhysicalReadings{Util: onCard(metaxUtilization), Used: used, Total: total}
}
