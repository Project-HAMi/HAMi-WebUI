package exporter

import (
	"math"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"vgpu/internal/resourceview"
)

// Inventory normally expires after five minutes. A deliberately slower collector
// has time to reach its next scheduled refresh and finish the bounded query.
const minimumInventoryMaxAge = 5 * time.Minute

// NewMetricsGatherer combines this server's resource snapshot with existing
// process-wide HAMi, Go runtime and scrape-handler metrics.
func NewMetricsGatherer(generator *MetricsGenerator) prometheus.Gatherer {
	registry := prometheus.NewRegistry()
	registry.MustRegister(newResourceMetricCollector(generator))
	return prometheus.Gatherers{prometheus.DefaultGatherer, registry}
}

var resourceDeviceLabels = []string{"node", "node_uid", "provider", "vendor", "model", "device_id", "device_kind", "parent_device_id", "device_uuid"}

type resourceMetricCollector struct {
	generator   *MetricsGenerator
	descriptors map[string]*prometheus.Desc
}

func newResourceMetricCollector(generator *MetricsGenerator) *resourceMetricCollector {
	c := &resourceMetricCollector{generator: generator, descriptors: map[string]*prometheus.Desc{}}
	for name, help := range map[string]string{
		"device_slots_capacity":              "Known device allocation slot capacity; not a physical device count.",
		"device_compute_capacity_ratio":      "Known configured compute capacity in normalized whole-device shares.",
		"device_memory_capacity_bytes":       "Known registered schedulable memory capacity in bytes.",
		"device_slots_allocated":             "Known observed effective HAMi Pod occupancy in slots; empty incomplete inventory is omitted, not zero.",
		"device_compute_allocated_ratio":     "Effective HAMi Pod compute occupancy for a device with complete allocation evidence, in whole-device shares.",
		"device_memory_allocated_bytes":      "Effective HAMi Pod memory occupancy in bytes for a device with complete allocation evidence.",
		"device_compute_unknown_occupancies": "Observed HAMi occupancies with unknown compute shares; an incomplete zero is omitted and does not prove allocator coverage.",
	} {
		c.descriptors[name] = prometheus.NewDesc("hami_webui_"+name, help, resourceDeviceLabels, nil)
	}
	for name, help := range map[string]string{
		"device_utilization_ratio":     "Source-native physical-device busy ratio. Compare only equivalent provider, source and model; not quota consumption.",
		"device_memory_used_bytes":     "Verified physical-device memory usage in bytes with current source identity and sample time.",
		"device_memory_physical_bytes": "Verified physical-device memory capacity in bytes from the vendor exporter.",
	} {
		c.descriptors[name] = prometheus.NewDesc("hami_webui_"+name, help, append(append([]string{}, resourceDeviceLabels...), "source"), nil)
	}
	c.descriptors["device_info"] = prometheus.NewDesc("hami_webui_device_info", "Known physical device identity. Resource and mode require agreement across registrations.", append(append([]string{}, resourceDeviceLabels...), "resource", "mode", "configured", "device_no", "driver_version"), nil)
	c.descriptors["metrics_schema_info"] = prometheus.NewDesc("hami_webui_metrics_schema_info", "This target emits schema 2 identities and fields. Presence alone does not validate historical field equivalence.", []string{"schema"}, nil)
	return c
}

func (c *resourceMetricCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range c.descriptors {
		ch <- desc
	}
}
func (c *resourceMetricCollector) Collect(ch chan<- prometheus.Metric) {
	c.generator.observationsMu.RLock()
	snapshot := c.generator.resourceSnapshot
	groups := c.generator.physicalGroups
	observations := c.generator.observations
	c.generator.observationsMu.RUnlock()
	now := c.generator.clockNow()
	if snapshot == nil || snapshot.At.IsZero() || snapshot.At.After(now) || now.Sub(snapshot.At) > max(minimumInventoryMaxAge, c.generator.interval+c.generator.timeout) {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.descriptors["metrics_schema_info"], prometheus.GaugeValue, 1, "2")
	for _, group := range groups {
		device := group.Members[0]
		model, _ := group.Model()
		labels, ok := resourceLabels(device, group.UUID, model)
		if !ok {
			continue
		}
		emit := func(name string, value float64, extra ...string) {
			if !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 {
				ch <- prometheus.MustNewConstMetric(c.descriptors[name], prometheus.GaugeValue, value, append(append([]string{}, labels...), extra...)...)
			}
		}
		resource, mode, configured := registrationLabels(group.Members)
		emit("device_info", 1, resource, mode, configured, "", "")
		// Several registrations can identify the same physical hardware while
		// exposing independent scheduling capacities. Their reservations cannot be
		// added, or attributed to an arbitrary representative.
		if len(group.Members) == 1 {
			for name, quantity := range map[string]resourceview.Quantity{"device_slots_capacity": device.SlotsCapacity, "device_compute_capacity_ratio": device.ComputeCapacityShare, "device_memory_capacity_bytes": device.MemoryCapacityBytes} {
				if value, known := quantity.KnownValue(); known {
					emit(name, value)
				}
			}
		}
		observation := group.Observation(func(member resourceview.Device) resourceview.DeviceObservation {
			return resourceview.AgeObservation(observations[member.Identity], now, dcgmMaxSampleAge)
		})
		for name, quantity := range map[string]resourceview.Quantity{"device_utilization_ratio": observation.UtilizationRatio, "device_memory_used_bytes": observation.MemoryUsedBytes, "device_memory_physical_bytes": observation.MemoryPhysicalBytes} {
			if value, known := quantity.KnownValue(); known && quantity.State.Source != "" {
				emit(name, value, quantity.State.Source)
			}
		}
		totals, valid := group.Occupancy()
		if !valid {
			continue
		}
		if totals.Slots.Unknown == 0 {
			emit("device_slots_allocated", totals.Slots.Sum.AsApproximateFloat64())
		}
		if totals.Compute.Unknown == 0 {
			emit("device_compute_allocated_ratio", totals.Compute.Sum.AsApproximateFloat64())
		}
		if totals.Memory.Unknown == 0 {
			emit("device_memory_allocated_bytes", totals.Memory.Sum.AsApproximateFloat64())
		}
		emit("device_compute_unknown_occupancies", float64(totals.Compute.Unknown))
	}
}

// Resource and mode describe registrations. A value applies to the physical
// device only when each registration confirms it.
func registrationLabels(members []resourceview.Device) (resource, mode, configured string) {
	first := members[0]
	resource, mode, configured = first.Resource, first.Mode, "unknown"
	for _, member := range members {
		if member.States["resource"].GetState() != resourceview.Known || member.Resource != resource {
			resource = ""
		}
		if member.States["mode"].GetState() != resourceview.Known || member.Mode != mode {
			mode = ""
		}
	}
	if len(members) == 1 && first.Configured != nil {
		configured = strconv.FormatBool(*first.Configured)
	}
	return
}

func resourceLabels(device resourceview.Device, uuid, model string) ([]string, bool) {
	id, err := resourceview.DeviceID(device.Identity.Provider, device.Identity.Node, device.Identity.DeviceID)
	if err != nil || device.Identity.NodeUID == "" {
		return nil, false
	}
	// device_id is the stable API link. node_uid distinguishes node incarnations
	// in history even if a node-local device ID is reused.
	return []string{device.Identity.Node, string(device.Identity.NodeUID), device.Identity.Provider, device.Vendor, model, id, "physical", "", uuid}, true
}
