package hygon

import (
	"fmt"
	"strings"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

// NewHCUTelemetryDialect returns the TelemetryDialect for Hygon HCU exporters.
func NewHCUTelemetryDialect() provider.TelemetryDialect {
	return inventoryDialect{hcu: true}
}

// NewDCUTelemetryDialect returns the TelemetryDialect for legacy Hygon DCU exporters.
func NewDCUTelemetryDialect() provider.TelemetryDialect {
	return inventoryDialect{hcu: false}
}

func hygonMetricNames(providerName string) (utilization, used, total string) {
	prefix := strings.ToLower(providerName)
	return prefix + "_utilizationrate", prefix + "_usedmemory_bytes", prefix + "_memorycap_bytes"
}

func (d inventoryDialect) ProviderName() string { return d.Name() }
func (d inventoryDialect) SourceName() string   { return strings.ToLower(d.Name()) + "_exporter" }
func (inventoryDialect) MemoryScale() float64   { return 1 }

func (inventoryDialect) BatchKey(device resourceview.Device) (string, bool) {
	return device.Identity.Node, true
}

func (d inventoryDialect) QuerySpec(escapedKeys []string) ([]string, string) {
	utilName, usedName, totalName := hygonMetricNames(d.Name())
	return []string{utilName, usedName, totalName}, fmt.Sprintf("node=~%q", strings.Join(escapedKeys, "|"))
}

func (d inventoryDialect) SampleKey(labels map[string]string) string {
	return labels["node"] + "\x00" + d.Name() + "-" + labels["device_id"]
}

func (inventoryDialect) DeviceKey(device resourceview.Device) string {
	return device.Identity.Node + "\x00" + device.Identity.DeviceID
}

func (d inventoryDialect) ReadDevice(
	device resourceview.Device,
	samples provider.TelemetryRawFamilies,
	at time.Time,
	nodeCreated *time.Time,
	targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) provider.PhysicalReadings {
	utilName, usedName, totalName := hygonMetricNames(d.Name())
	schema := provider.NodeMetricSchema{
		DeviceIDLabel:     "device_id",
		IndexLabel:        "minor_number",
		EntityLabel:       "pcieBus_number",
		ModelLabel:        "name",
		UtilizationMetric: utilName,
		RequireNodeLabel:  true,
		ResolveExportedID: func(dev resourceview.Device) (string, bool) {
			return BoardSerial(dev.Identity.Provider, dev.Identity.DeviceID)
		},
	}
	if d.hcu {
		// Template VDev IDs identify a virtual entity rather than the physical board.
		schema.SkipSample = func(_ string, labels map[string]string) bool {
			return labels["vhcu_minor_number"] != ""
		}
	}
	get := func(name string) provider.TelemetryReading {
		return schema.Reading(name, device, samples[name], at, nodeCreated, targets)
	}
	used, total := schema.MemoryPair(get(usedName), get(totalName))
	return provider.PhysicalReadings{Util: get(utilName), Used: used, Total: total}
}
