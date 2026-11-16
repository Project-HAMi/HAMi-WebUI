package exporter

import (
	"strconv"

	"time"

	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

// The node exporters expose physical measurements under vendor-specific labels.
// Pod labels can repeat a chip reading; accept copies only when the physical
// identity, scrape, target and value agree.
type nodeMetricLabels struct{ deviceID, index, pci, model, utilization string }

var (
	ascendLabels = nodeMetricLabels{"vdie_id", "id", "pcie_bus_info", "model_name", ascendUtilization}
	hygonLabels  = nodeMetricLabels{deviceID: "device_id", index: "minor_number", pci: "pcieBus_number", model: "name"}
)

func (schema nodeMetricLabels) reading(name string, device resourceview.Device, samples []dcgmSample, at time.Time, nodeCreated *time.Time, targets map[dcgmTarget]dcgmTargetEvidence) dcgmReading {
	id := device.Identity
	exportedID := id.DeviceID
	if id.Provider == biz.HygonHCUDevice || id.Provider == biz.HygonGPUDevice {
		var ok bool
		exportedID, ok = id.HygonBoardSerial()
		if !ok {
			return dcgmReading{err: errDCGMMissing}
		}
	}

	var result dcgmSelection
	var rejected error
	for _, sample := range samples {
		labels := sample.Labels
		if labels["__name__"] != name || labels[schema.deviceID] != exportedID || labels["node"] != id.Node {
			continue
		}
		// A template's own VDev ID identifies a different entity. The named chip
		// metrics repeat the physical reading under each associated Pod's labels.
		if id.Provider == biz.HygonHCUDevice && labels["vhcu_minor_number"] != "" {
			continue
		}
		if id.Provider == biz.AscendGPUDevice && (labels["v_dev_id"] != "" || labels["is_virtual"] == "true") {
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
		// With honor_labels disabled, a target's node label overrides the
		// exporter's node; Prometheus preserves the latter as exported_node.
		if exporterNode := labels["exported_node"]; exporterNode != "" && exporterNode != id.Node {
			return dcgmReading{err: errDCGMAmbiguous}
		}
		if evidence.Ambiguous || dcgmConfirmTargetAndNode(labels, evidence.TargetLabels, source, id.Node) != nil ||
			(id.Provider == biz.AscendGPUDevice && !ascendModelsCompatible(device.Model, labels[schema.model])) {
			return dcgmReading{err: errDCGMAmbiguous}
		}
		chip, err := strconv.Atoi(labels[schema.index])
		if err != nil || chip < 0 || !dcgmFinite(sample.Value) || sample.Value < 0 || (name == schema.utilization && sample.Value > 100) {
			return dcgmReading{err: errDCGMInvalid}
		}
		if !result.SampleAt.IsZero() && (source != result.Source || !sample.SampleAt.Equal(result.SampleAt) ||
			sample.Value != result.Value || labels[schema.index] != result.Labels[schema.index] || labels[schema.pci] != result.Labels[schema.pci] || labels[schema.model] != result.Labels[schema.model]) {
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
func (schema nodeMetricLabels) memoryPair(used, total dcgmReading) (dcgmReading, dcgmReading) {
	if total.err == nil && total.selection.Value <= 0 {
		return dcgmReading{err: errDCGMInvalid}, dcgmReading{err: errDCGMInvalid}
	}
	if used.err == nil && total.err == nil {
		u, t := used.selection, total.selection
		if u.Source != t.Source || !u.SampleAt.Equal(t.SampleAt) || u.Labels[schema.index] != t.Labels[schema.index] || u.Labels[schema.pci] != t.Labels[schema.pci] ||
			(u.Labels[schema.model] != "" && t.Labels[schema.model] != "" && u.Labels[schema.model] != t.Labels[schema.model]) {
			return dcgmReading{err: errDCGMAmbiguous}, dcgmReading{err: errDCGMAmbiguous}
		}
		if u.Value > t.Value {
			return dcgmReading{err: errDCGMInvalid}, dcgmReading{err: errDCGMInvalid}
		}
	}
	return used, total
}
