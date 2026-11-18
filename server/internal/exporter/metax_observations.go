package exporter

import (
	"slices"
	"strings"
	"time"

	"vgpu/internal/resourceview"
)

const (
	metaxState       = "mx_gpu_state"
	metaxUtilization = "mx_gpu_usage"
	metaxUsed        = "mx_memory_used"
	metaxTotal       = "mx_memory_total"
)

var metaxLabels = nodeMetricLabels{
	deviceID: "uuid", index: "deviceId", entity: "die_id", model: "modelName", utilization: metaxUtilization,
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

func metaxReadings(device resourceview.Device, samples rawFamilies, at time.Time, born *time.Time, targets map[dcgmTarget]dcgmTargetEvidence) physicalReadings {
	// Without a registered product name, let the exporter's parent-card
	// state identify the model before applying the C500 single-die contract.
	if !metaxC500(device.Model) && !slices.ContainsFunc(samples[metaxState], func(s dcgmSample) bool {
		return s.Labels["uuid"] == device.Identity.DeviceID && metaxC500(s.Labels["modelName"])
	}) {
		return failedReadings(errDCGMMissing)
	}
	read := func(name string) dcgmReading {
		return metaxLabels.reading(name, device, samples[name], at, born, targets)
	}
	// mxExporter enumerates GPU state for each die. Do not promote a lone
	// utilization sample to a whole-card reading when another die is known.
	state := read(metaxState)
	if state.err != nil {
		return failedReadings(state.err)
	}
	if state.selection.Value == 0 {
		return failedReadings(errDCGMMissing)
	}
	if state.selection.Value != 1 {
		return failedReadings(errDCGMInvalid)
	}
	// A collector can lose one die's state while its other fields are still
	// present. Its siblings must not then look like an entire C500.
	for _, rows := range samples {
		for _, sample := range rows {
			l := sample.Labels
			target := dcgmTarget{Cluster: l["cluster"], Job: l["job"], Instance: l["instance"]}
			if l["uuid"] == device.Identity.DeviceID && target == state.selection.Source && sample.SampleAt.Equal(state.selection.SampleAt) && l["die_id"] != "0" {
				return failedReadings(errDCGMAmbiguous)
			}
		}
	}
	onCard := func(name string) dcgmReading {
		r := read(name)
		if r.err == nil && !metaxLabels.sameCollection(state.selection, r.selection) {
			return dcgmReading{err: errDCGMAmbiguous}
		}
		return r
	}
	used, total := metaxLabels.memoryPair(onCard(metaxUsed), onCard(metaxTotal))
	return physicalReadings{util: onCard(metaxUtilization), used: used, total: total}
}
