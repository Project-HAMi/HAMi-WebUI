package ascend

import (
	"fmt"
	"strings"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

const ascendUtilization = "npu_chip_info_utilization"

var ascendSchema = provider.NodeMetricSchema{
	DeviceIDLabel:     "vdie_id",
	IndexLabel:        "id",
	EntityLabel:       "pcie_bus_info",
	ModelLabel:        "model_name",
	UtilizationMetric: ascendUtilization,
	RequireNodeLabel:  true,
	SkipSample: func(_ string, labels map[string]string) bool {
		return labels["v_dev_id"] != "" || labels["is_virtual"] == "true"
	},
	ValidateMatch: func(device resourceview.Device, labels map[string]string) bool {
		return ascendModelsCompatible(device.Model, labels["model_name"])
	},
}

// NewTelemetryDialect returns the TelemetryDialect for Huawei Ascend NPU exporters.
func NewTelemetryDialect() provider.TelemetryDialect {
	return inventoryDialect{}
}

func (inventoryDialect) ProviderName() string { return AscendDevice }
func (inventoryDialect) SourceName() string   { return "npu_exporter" }
func (inventoryDialect) MemoryScale() float64 { return float64(provider.BytesPerMiB) }

func (inventoryDialect) BatchKey(device resourceview.Device) (string, bool) {
	return device.Identity.Node, true
}

func (inventoryDialect) QuerySpec(escapedKeys []string) ([]string, string) {
	families := []string{
		ascendUtilization,
		"npu_chip_info_hbm_used_memory",
		"npu_chip_info_hbm_total_memory",
		"npu_chip_info_used_memory",
		"npu_chip_info_total_memory",
	}
	filter := fmt.Sprintf("node=~%q,v_dev_id=\"\",is_virtual!=\"true\"", strings.Join(escapedKeys, "|"))
	return families, filter
}

func (inventoryDialect) SampleKey(labels map[string]string) string {
	return labels["node"] + "\x00" + labels["vdie_id"]
}

func (inventoryDialect) DeviceKey(device resourceview.Device) string {
	return device.Identity.Node + "\x00" + device.Identity.DeviceID
}

func (inventoryDialect) ReadDevice(
	device resourceview.Device,
	samples provider.TelemetryRawFamilies,
	at time.Time,
	nodeCreated *time.Time,
	targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) provider.PhysicalReadings {
	get := func(name string) provider.TelemetryReading {
		return ascendSchema.Reading(name, device, samples[name], at, nodeCreated, targets)
	}
	readings := provider.FailedPhysicalReadings(provider.ErrTelemetryMissing)
	readings.Util = get(ascendUtilization)
	if used, total := ascendMemoryFamilies(device.Model); used != "" {
		readings.Used, readings.Total = ascendSchema.MemoryPair(get(used), get(total))
	}
	return readings
}

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
