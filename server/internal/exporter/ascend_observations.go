package exporter

import "strings"

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
