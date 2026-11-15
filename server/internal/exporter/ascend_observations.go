package exporter

import (
	"strconv"
	"strings"
	"time"

	"vgpu/internal/resourceview"
)

const ascendUtilization = "npu_chip_info_utilization"
const ascendSource = "npu_exporter"

// Registrations use e.g. "Ascend910B2" while npu-exporter can report
// "910B2" or a family name such as "910B-Ascend-V1".
func ascendModelCode(model string) string {
	model = strings.ToUpper(strings.TrimSpace(model))
	// GetNpuName builds <chip name>-Ascend-<version>; the chip name
	// itself can contain a dash (e.g. 910B4-1).
	code, _, _ := strings.Cut(model, "-ASCEND-")
	return strings.TrimPrefix(code, "ASCEND")
}

func ascendModelsCompatible(registered, observed string) bool {
	a, b := ascendModelCode(registered), ascendModelCode(observed)
	if a == "" || b == "" || a == b {
		return true
	}
	// A family-only label cannot distinguish its variants. Once both labels
	// identify a variant, sharing a family or an HBM bank is insufficient.
	for _, family := range []string{"910", "910A", "910B", "310P"} {
		if (a == family && strings.HasPrefix(b, family)) || (b == family && strings.HasPrefix(a, family)) {
			return true
		}
	}
	return false
}

// On the 910 series the exporter reads accelerator memory from its HBM collector.
// 310P uses DDR. The two banks must not be combined or substituted for each other.
func ascendMemoryFamilies(model string) (used, total string) {
	model = ascendModelCode(model)
	switch {
	case strings.HasPrefix(model, "910"):
		return "npu_chip_info_hbm_used_memory", "npu_chip_info_hbm_total_memory"
	case strings.HasPrefix(model, "310P"):
		return "npu_chip_info_used_memory", "npu_chip_info_total_memory"
	}
	return "", ""
}

func ascendReading(name string, device resourceview.Device, samples []dcgmSample, at time.Time, nodeCreated *time.Time, targets map[dcgmTarget]dcgmTargetEvidence) dcgmReading {
	id := device.Identity

	var result dcgmSelection
	var rejected error
	for _, sample := range samples {
		labels := sample.Labels
		if labels["__name__"] != name || labels["vdie_id"] != id.DeviceID || labels["node"] != id.Node {
			continue
		}
		// A template's own VDev ID identifies a different entity. The named chip
		// metrics repeat the physical reading under each associated Pod's labels.
		if labels["v_dev_id"] != "" || labels["is_virtual"] == "true" {
			continue
		}
		source := dcgmTarget{Cluster: labels["cluster"], Job: labels["job"], Instance: labels["instance"]}
		evidence, ok := targets[source]
		if !ok || source.Job == "" || source.Instance == "" {
			rejected = dcgmRejectReason(rejected, errDCGMInvalid)
			continue
		}
		if evidence.Unavailable != nil {
			rejected = dcgmRejectReason(rejected, evidence.Unavailable)
			continue
		}
		if sample.SampleAt.IsZero() || sample.SampleAt.After(at) {
			return dcgmReading{err: errDCGMInvalid}
		}
		if at.Sub(sample.SampleAt) > dcgmMaxSampleAge || (nodeCreated != nil && sample.SampleAt.Before(*nodeCreated)) {
			rejected = dcgmRejectReason(rejected, errDCGMStale)
			continue
		}
		if evidence.Ambiguous || dcgmConfirmTargetAndNode(labels, evidence.TargetLabels, source, id.Node) != nil ||
			!ascendModelsCompatible(device.Model, labels["model_name"]) {
			return dcgmReading{err: errDCGMAmbiguous}
		}
		chip, err := strconv.Atoi(labels["id"])
		if err != nil || chip < 0 || !dcgmFinite(sample.Value) || sample.Value < 0 || (name == ascendUtilization && sample.Value > 100) {
			return dcgmReading{err: errDCGMInvalid}
		}
		if !result.SampleAt.IsZero() && (source != result.Source || !sample.SampleAt.Equal(result.SampleAt) ||
			sample.Value != result.Value || labels["id"] != result.Labels["id"] || labels["pcie_bus_info"] != result.Labels["pcie_bus_info"] || labels["model_name"] != result.Labels["model_name"]) {
			return dcgmReading{err: errDCGMAmbiguous}
		}
		result = dcgmSelection{Value: sample.Value, Source: source, Node: id.Node, RawName: name, SampleAt: sample.SampleAt, Labels: labels}
	}
	if result.SampleAt.IsZero() {
		if rejected == nil {
			rejected = errDCGMMissing
		}
		return dcgmReading{err: rejected}
	}
	return dcgmReading{selection: result}
}

// An occupancy ratio may only pair the same physical memory bank, target and collection.
func ascendMemoryPair(used, total dcgmReading) (dcgmReading, dcgmReading) {
	if total.err == nil && total.selection.Value <= 0 {
		return dcgmReading{err: errDCGMInvalid}, dcgmReading{err: errDCGMInvalid}
	}
	if used.err == nil && total.err == nil {
		u, t := used.selection, total.selection
		if u.Source != t.Source || !u.SampleAt.Equal(t.SampleAt) || u.Labels["id"] != t.Labels["id"] || u.Labels["pcie_bus_info"] != t.Labels["pcie_bus_info"] ||
			(u.Labels["model_name"] != "" && t.Labels["model_name"] != "" && u.Labels["model_name"] != t.Labels["model_name"]) {
			return dcgmReading{err: errDCGMAmbiguous}, dcgmReading{err: errDCGMAmbiguous}
		}
		if u.Value > t.Value {
			return dcgmReading{err: errDCGMInvalid}, dcgmReading{err: errDCGMInvalid}
		}
	}
	return used, total
}
