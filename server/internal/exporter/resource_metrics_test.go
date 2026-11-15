package exporter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func knownResource(value, unit string) resourceview.Quantity {
	q := resource.MustParse(value)
	return resourceview.Quantity{Value: &q, Unit: unit, State: &viewv1.FieldState{State: resourceview.Known}}
}
func completeCoverage(entity, source string, count int) *viewv1.Coverage {
	n := int32(count)
	return &viewv1.Coverage{Complete: true, Observed: n, Total: &n, Entity: entity, Sources: []*viewv1.SourceState{{Source: source, State: resourceview.Known}}}
}
func gatherOverview(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]float64{}
	for _, family := range families {
		if len(family.Metric) != 1 {
			t.Fatalf("expected one card for %s: %d", family.GetName(), len(family.Metric))
		}
		values[family.GetName()] = family.Metric[0].GetGauge().GetValue()
	}
	return values
}
func TestAllocationRequiresCompleteEvidencePerDimension(t *testing.T) {
	at := time.Now()
	now := at
	id := resourceview.DeviceIdentity{Node: "node", NodeUID: "uid", Provider: "NVIDIA", DeviceID: "gpu"}
	card := resourceview.Device{Identity: id, Kind: "physical", States: map[string]*viewv1.FieldState{"kind": {State: resourceview.Known}}, Configured: proto.Bool(true), ComputeCapacityShare: knownResource("1", "share"), MemoryCapacityBytes: knownResource("16Gi", "bytes")}
	known := resourceview.EffectiveOccupancy{Device: id, PodUID: "p1", Source: "hami", ComputeAllocatedShare: knownResource(".2", "share"), MemoryAllocatedBytes: knownResource("1Gi", "bytes"), SlotsAllocated: knownResource("1", "slots")}
	second := known
	second.PodUID = "p2"
	s := &MetricsGenerator{now: func() time.Time { return now }}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(newResourceMetricCollector(s))
	set := func(rows ...resourceview.EffectiveOccupancy) {
		card.EffectiveOccupancies = rows
		card.AllocationCoverage = completeCoverage("occupancy", "hami", len(rows))
		s.resourceSnapshot = &resourceview.Snapshot{At: at, Nodes: []resourceview.Node{{Name: id.Node, UID: id.NodeUID}}, Devices: []resourceview.Device{card}}
		s.physicalGroups = resourceview.GroupPhysicalDevices(*s.resourceSnapshot).Groups
	}
	expect := func(metric string, value float64, present bool) {
		t.Helper()
		data := gatherOverview(t, registry)
		actual, exists := data["hami_webui_device_"+metric]
		if exists != present || (present && actual != value) {
			t.Fatalf("%s = %v (present %t), want %v (%t)", metric, actual, exists, value, present)
		}
	}
	set()
	expect("memory_allocated_bytes", 0, true)
	expect("compute_allocated_ratio", 0, true)
	set(known, second)
	expect("memory_allocated_bytes", 2*bytesPerMiB*1024, true)
	second.MemoryAllocatedBytes = resourceview.Quantity{}
	set(known, second)
	expect("memory_allocated_bytes", 0, false)
	expect("compute_allocated_ratio", .4, true)
	second.ComputeAllocatedShare = resourceview.Quantity{}
	set(known, second)
	expect("compute_allocated_ratio", 0, false)
	expect("compute_unknown_occupancies", 1, true)
	set(known, known)
	expect("memory_allocated_bytes", 0, false)
	expect("compute_allocated_ratio", 0, false)
	set(known)
	s.resourceSnapshot.Devices[0].AllocationCoverage.Complete = false
	expect("memory_allocated_bytes", 0, false)
	set()
	card.Kind = "unknown"
	set(known)
	expect("memory_allocated_bytes", 0, false)
	card.Kind = "physical"
	set(known)
	now = at.Add(minimumInventoryMaxAge + time.Millisecond)
	expect("memory_allocated_bytes", 0, false)
	expect("memory_capacity_bytes", 0, false)
	expect("info", 0, false)
	s.interval, s.timeout = 10*time.Minute, time.Minute
	now = at.Add(10*time.Minute + 30*time.Second)
	expect("info", 1, true)
	expect("memory_allocated_bytes", bytesPerMiB*1024, true)
	now = at.Add(11*time.Minute + time.Millisecond)
	expect("info", 0, false)
}

type sequenceFacts struct {
	reads     int
	available []resourceview.Snapshot
}

func (f *sequenceFacts) Snapshot(context.Context) (resourceview.Snapshot, error) {
	i := f.reads
	f.reads++
	if i >= len(f.available) {
		return resourceview.Snapshot{}, errors.New("cache unavailable")
	}
	return f.available[i], nil
}
func TestInterruptedInventoryIsRetriedAndIncompleteBatchWithdrawn(t *testing.T) {
	at := time.Now()
	partial := resourceview.Snapshot{At: at, NodeCoverage: &viewv1.Coverage{Sources: []*viewv1.SourceState{{State: resourceview.Unknown, Params: map[string]string{"continuity": "source_changed_during_snapshot"}}}}}
	good := resourceview.Snapshot{At: at, NodeCoverage: completeCoverage("node", "shared_node_informer", 0)}
	f := &sequenceFacts{available: []resourceview.Snapshot{partial, good, partial, partial}}
	g := &MetricsGenerator{resourceSource: f, now: func() time.Time { return at }}
	if err := g.refreshResourceSnapshot(context.Background(), nil); err != nil || f.reads != 2 || g.resourceSnapshot == nil {
		t.Fatal("did not recover from local retry", err)
	}
	if err := g.refreshResourceSnapshot(context.Background(), nil); err == nil || f.reads != 4 || g.resourceSnapshot != nil {
		t.Fatal("republished unstable inventory", err)
	}
	// A disconnected watch continues serving old informer objects. A fresh
	// assembly time must not turn those objects back into live inventory.
	f.available = append(f.available, resourceview.Snapshot{At: at.Add(time.Minute), NodeCoverage: &viewv1.Coverage{
		Entity: "node", Sources: []*viewv1.SourceState{{Source: "shared_node_informer", State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"continuity": "watch_closed"}}},
	}})
	g.resourceSnapshot = &good
	if err := g.refreshResourceSnapshot(context.Background(), nil); err == nil || f.reads != 5 || g.resourceSnapshot != nil {
		t.Fatal("lost watch published an apparently fresh inventory", err)
	}
	g.resourceSnapshot = &good
	if err := g.refreshResourceSnapshot(context.Background(), nil); err == nil || g.resourceSnapshot != nil {
		t.Fatal("cache outage republished earlier allocation")
	}
}

func TestMetricIdentitiesMatchPhysicalInventoryWhenProvidersShareAnUUID(t *testing.T) {
	at := time.Now()
	first := resourceview.Device{Identity: resourceview.DeviceIdentity{Node: "host", NodeUID: "uid", Provider: "A", DeviceID: "first"},
		Kind: "physical", Model: "model A", UUID: proto.String("common"), Configured: proto.Bool(true), AllocationCoverage: completeCoverage("occupancy", "hami", 0),
		States:              map[string]*viewv1.FieldState{"model": {State: resourceview.Known}, "kind": {State: resourceview.Known}, "uuid": {State: resourceview.Known}},
		MemoryCapacityBytes: knownResource("40Gi", "bytes")}
	second := first
	second.Identity.Provider = "B"
	second.Identity.DeviceID = "second"
	second.Model = "model B"
	snapshot := resourceview.Snapshot{At: at, Nodes: []resourceview.Node{{Name: "host", UID: "uid"}}, Devices: []resourceview.Device{second, first}}
	g := &MetricsGenerator{physicalGroups: resourceview.GroupPhysicalDevices(snapshot).Groups, resourceSnapshot: &snapshot, observations: resourceview.DeviceObservations{}, now: func() time.Time { return at }}
	known := func(v float64, source string) resourceview.Quantity {
		return observationQuantity(dcgmReading{selection: dcgmSelection{Value: v, SampleAt: at, Source: dcgmTarget{Job: "exporter", Instance: "node"}}}, 1, "ratio", source, resourceview.Quantity{}, at)
	}
	g.observations[second.Identity] = resourceview.DeviceObservation{UtilizationRatio: known(.25, "dcgm")}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(newResourceMetricCollector(g))
	data := gatherOverview(t, registry)
	if val, reported := data["hami_webui_device_utilization_ratio"]; !reported || val != .25 {
		t.Fatal("single confirmed alternate source lost")
	}
	if _, found := data["hami_webui_device_memory_allocated_bytes"]; found {
		t.Fatal("guessed which allocation authority owns a shared device")
	}
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, m := range family.Metric {
			for _, l := range m.Label {
				if l.GetName() == "model" && l.GetValue() != "" {
					t.Fatal("conflicting aliases must have an unknown model")
				}
				if l.GetName() == "provider" && l.GetValue() != "A" {
					t.Fatal("metric uses a different identity from the API")
				}
			}
		}
	}
	g.observations[first.Identity] = resourceview.DeviceObservation{UtilizationRatio: known(.5, "dcgm")}
	if _, conflict := gatherOverview(t, registry)["hami_webui_device_utilization_ratio"]; conflict {
		t.Fatal("conflicting alias reading emitted")
	}
}

// Each server owns the resource collector; legacy and process metrics remain
// available from the same gatherer. Constructing another server is independent.
func TestMetricsGatherersAreIsolated(t *testing.T) {
	first := NewMetricsGenerator(nil, nil, nil, nil, nil, log.DefaultLogger, nil)
	second := NewMetricsGenerator(nil, nil, nil, nil, nil, log.DefaultLogger, nil)
	first.resourceSnapshot = &resourceview.Snapshot{At: time.Now()}
	for _, generator := range []*MetricsGenerator{first, second} {
		families, err := NewMetricsGatherer(generator).Gather()
		if err != nil {
			t.Fatal(err)
		}
		var schema, process bool
		for _, family := range families {
			switch family.GetName() {
			case "hami_webui_metrics_schema_info":
				schema = true
			case "go_goroutines":
				process = true
			}
		}
		if !process || schema != (generator == first) {
			t.Fatalf("go_goroutines=%t schema=%t", process, schema)
		}
	}
}

func scrapePhysicalLabels(t *testing.T, cards ...resourceview.Device) map[string]map[string]string {
	t.Helper()
	at := time.Now()
	id := cards[0].Identity
	snapshot := resourceview.Snapshot{At: at, Nodes: []resourceview.Node{{Name: id.Node, UID: id.NodeUID}}, Devices: cards}
	busy := observationQuantity(dcgmReading{selection: dcgmSelection{Value: .83, SampleAt: at,
		Source: dcgmTarget{Job: "exporter", Instance: "worker"}}}, 1, "ratio", "exporter", resourceview.Quantity{}, at)
	g := &MetricsGenerator{now: func() time.Time { return at }, resourceSnapshot: &snapshot, physicalGroups: resourceview.GroupPhysicalDevices(snapshot).Groups,
		observations: resourceview.DeviceObservations{id: {UtilizationRatio: busy}}}
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(newResourceMetricCollector(g))
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	metrics := map[string]map[string]string{}
	for _, f := range families {
		if len(f.Metric) != 1 {
			t.Fatalf("%s emitted %d rows for one card", f.GetName(), len(f.Metric))
		}
		labels := map[string]string{}
		for _, l := range f.Metric[0].Label {
			labels[l.GetName()] = l.GetValue()
		}
		metrics[f.GetName()] = labels
	}
	return metrics
}

func TestDeviceInfoRequiresAgreementAcrossRegistrations(t *testing.T) {
	states := map[string]*viewv1.FieldState{}
	for _, field := range []string{"model", "kind", "uuid", "mode", "resource"} {
		states[field] = &viewv1.FieldState{State: resourceview.Known}
	}
	first := resourceview.Device{
		Identity: resourceview.DeviceIdentity{Node: "n", NodeUID: "u", Provider: "A", DeviceID: "a"},
		Kind:     "physical", UUID: proto.String("shared"), States: states,
		Resource: "a.com/foo", Mode: "mig", Configured: proto.Bool(true), MemoryCapacityBytes: knownResource("1Gi", "bytes"),
	}
	second := first
	second.Identity.Provider, second.Identity.DeviceID = "Z", "z"
	info := "hami_webui_device_info"
	agreed := scrapePhysicalLabels(t, second, first)[info]
	if agreed["resource"] != first.Resource || agreed["mode"] != first.Mode || agreed["configured"] != "unknown" {
		t.Fatalf("agreed aliases: %v", agreed)
	}
	second.Mode = "hami-core"
	conflict := scrapePhysicalLabels(t, first, second)[info]
	if conflict["mode"] != "" || conflict["resource"] != first.Resource {
		t.Fatalf("independent mode conflict: %v", conflict)
	}
	second.Resource = "z.com/bar"
	conflict = scrapePhysicalLabels(t, second, first)[info]
	if conflict["resource"] != "" || conflict["mode"] != "" || conflict["configured"] != "unknown" {
		t.Fatalf("conflicting aliases: %v", conflict)
	}
}

func TestPhysicalMetricSeriesDistinguishNodeRecreation(t *testing.T) {
	// The API link can be reused after a node replacement. Raw measurements
	// and registered capacities must retain their own node incarnation.
	card := resourceview.Device{Identity: resourceview.DeviceIdentity{Node: "worker", NodeUID: "live-uid", Provider: "Ascend", DeviceID: "chip-0"},
		Kind: "physical", States: map[string]*viewv1.FieldState{"kind": {State: resourceview.Known}}, MemoryCapacityBytes: knownResource("1Gi", "bytes")}
	old := scrapePhysicalLabels(t, card)
	card.Identity.NodeUID = "replacement-uid"
	fresh := scrapePhysicalLabels(t, card)
	for _, metric := range []string{"hami_webui_device_info", "hami_webui_device_memory_capacity_bytes", "hami_webui_device_utilization_ratio"} {
		a, b := old[metric], fresh[metric]
		if a["node_uid"] != "live-uid" || b["node_uid"] != "replacement-uid" || a["device_id"] == "" || a["device_id"] != b["device_id"] {
			t.Fatalf("%s old=%v new=%v", metric, a, b)
		}
	}
}
