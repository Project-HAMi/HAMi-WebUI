package exporter

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

func TestPhysicalExporterObservationsEndToEnd(t *testing.T) {
	at := time.UnixMilli(1760000000123).UTC()
	original := at.Add(-45 * time.Second)
	key := func(provider, id string) resourceview.DeviceIdentity {
		return resourceview.DeviceIdentity{Provider: provider, Node: "host", NodeUID: "host-uid", DeviceID: id}
	}
	gpu := key(biz.NvidiaGPUDevice, "GPU-01")
	chip := key(biz.AscendGPUDevice, "chip-01")
	facts := resourceview.Snapshot{At: at, NodeCoverage: completeCoverage("node", "shared_node_informer", 1), Devices: []resourceview.Device{
		{Identity: gpu, Kind: "physical", Model: "H100"}, {Identity: chip, Kind: "physical", Model: "Ascend310P"},
		{Identity: key(biz.AscendGPUDevice, "910C-module"), Kind: "unknown", Model: "Ascend910C"},
	}}
	f := &sequenceFacts{available: []resourceview.Snapshot{facts, facts, facts}}
	g := &MetricsGenerator{now: func() time.Time { return at }, resourceSource: f}
	targetUp := 1.
	unavailable := false
	query := func(_ context.Context, expr string, evalAt time.Time) (model.Value, error) {
		if !evalAt.Equal(at) {
			t.Fatal("different evaluation times in the same cycle")
		}
		var rows model.Vector
		add := func(name string, l model.Metric, v float64) {
			l = l.Clone()
			l[model.MetricNameLabel] = model.LabelValue(name)
			rows = append(rows, &model.Sample{Metric: l, Value: model.SampleValue(v), Timestamp: model.Time(evalAt.UnixMilli())})
			ts := l.Clone()
			ts[model.MetricNameLabel] = model.LabelValue(sampleTimePrefix + name)
			rows = append(rows, &model.Sample{Metric: ts, Value: model.SampleValue(float64(original.UnixMilli()) / 1000), Timestamp: model.Time(evalAt.UnixMilli())})
		}
		a := model.Metric{"job": "npu", "instance": "h:8082", "node": "host"}
		n := model.Metric{"job": "dcgm", "instance": "h:9400", "node": "host"}
		switch {
		case strings.Contains(expr, "timestamp(up)"):
			if unavailable {
				return nil, errors.New("prometheus timeout")
			}
			add("up", a, targetUp)
			add("up", n, targetUp)
		case strings.Contains(expr, "DCGM_FI_DEV_FB_USED"):
			n["UUID"] = "GPU-01"
			add("DCGM_FI_DEV_GPU_UTIL_RATIO", n, .75)
			add("DCGM_FI_DEV_FB_USED_BYTES", n, 256*bytesPerMiB)
			add("DCGM_FI_DEV_FB_FREE", n, 768)
			n["GPU_I_ID"] = "0"
			add("DCGM_FI_DEV_GPU_UTIL_RATIO", n, .99)
			delete(n, "GPU_I_ID")
			n["exported_pod"] = "worker"
			add("DCGM_FI_DEV_FB_USED_BYTES", n, 100*bytesPerMiB)
		case strings.Contains(expr, "npu_chip_info_hbm_total_memory"):
			a["id"] = "0"
			a["vdie_id"] = "chip-01"
			a["model_name"] = "Ascend310P"
			for _, pod := range []string{"first", "second"} {
				a["pod_name"] = model.LabelValue(pod)
				add(ascendUtilization, a, 0)
			}
			add("npu_chip_info_used_memory", a, 128)
			add("npu_chip_info_total_memory", a, 8192)
			add("npu_chip_info_hbm_used_memory", a, 3000)
			add("npu_chip_info_hbm_total_memory", a, 4096)
			a["v_dev_id"] = "1"
			add(ascendUtilization, a, 99)
		default:
			t.Fatalf("unrecognized selector %s", expr)
		}
		return rows, nil
	}
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	obs := g.DeviceObservations()
	for id, want := range map[resourceview.DeviceIdentity][3]float64{gpu: {.75, 256 * bytesPerMiB, 1024 * bytesPerMiB}, chip: {0, 128 * bytesPerMiB, 8192 * bytesPerMiB}} {
		d := obs[id]
		for i, q := range []resourceview.Quantity{d.UtilizationRatio, d.MemoryUsedBytes, d.MemoryPhysicalBytes} {
			value, ok := q.KnownValue()
			if !ok || value != want[i] || !q.State.ObservedAt.AsTime().Equal(original) {
				t.Fatalf("%s field %d = %v/%v at %v", id.DeviceID, i, value, ok, q.State)
			}
		}
	}
	if _, exists := obs[key(biz.AscendGPUDevice, "910C-module")]; exists {
		t.Fatal("module acquired whole-card telemetry")
	}
	// Callers own their returned quantities, including nested evidence.
	obs[gpu].UtilizationRatio.Value.Set(0)
	obs[gpu].UtilizationRatio.State.Params["instance"] = "changed"
	fresh := g.DeviceObservations()[gpu].UtilizationRatio
	if value, known := fresh.KnownValue(); !known || value != .75 || fresh.State.Params["instance"] != "h:9400" {
		t.Fatal("caller mutated published telemetry")
	}
	unavailable = true
	if err := g.refreshResourceSnapshot(context.Background(), query); !errors.Is(err, errTelemetryQuery) {
		t.Fatal("wrong query outage", err)
	}
	for _, d := range g.DeviceObservations() {
		if d.UtilizationRatio.State.State != resourceview.Stale || !d.UtilizationRatio.State.ObservedAt.AsTime().Equal(original) {
			t.Fatal("old data promoted or retimestamped")
		}
	}
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(newResourceMetricCollector(g))
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "hami_webui_device_utilization_ratio" {
			t.Fatal("stale API quantity still emitted as current")
		}
	}
	unavailable = false
	targetUp = 0
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	for _, d := range g.DeviceObservations() {
		if d.UtilizationRatio.Value != nil || d.UtilizationRatio.State.Reason != "target_down" {
			t.Fatal("down target left a physical reading or reported a conflict")
		}
	}
}
func TestTimestampMismatchAndDuplicateTargets(t *testing.T) {
	now := time.Now().UTC()
	target := dcgmSample{Labels: map[string]string{"job": "j", "instance": "i"}, Value: 1, SampleAt: now}
	targets := targetEvidence([]dcgmSample{target, target}, now)
	if !targets[dcgmTarget{Job: "j", Instance: "i"}].Ambiguous {
		t.Fatal("duplicate target marked usable")
	}
	query := func(context.Context, string, time.Time) (model.Value, error) {
		return model.Vector{{Metric: model.Metric{"__name__": "up", "instance": "a"}, Value: 1},
			{Metric: model.Metric{"__name__": sampleTimePrefix + "up", "instance": "b"}, Value: 1794661258}}, nil
	}
	_, err := queryFamilies(context.Background(), query, []string{"up"}, "", now)
	if !errors.Is(err, errDCGMInvalid) {
		t.Fatalf("mismatched original timestamp (%v)", err)
	}
}

func TestTelemetryTimestampsAreTakenAtTheRawSelector(t *testing.T) {
	// timestamp of a function result defaults to the evaluation timestamp in
	// Prometheus, even when that function changed only labels.
	filter := `{node="test"}`
	names := []string{"util", "used"}
	query := func(_ context.Context, expr string, _ time.Time) (model.Value, error) {
		for _, name := range names {
			selector := name + filter
			if !strings.Contains(expr, "timestamp("+selector+")") || !strings.Contains(expr, sampleTimePrefix+name) {
				t.Fatalf("original sample time lost: %s", expr)
			}
		}
		if strings.Count(expr, "timestamp(") != len(names) {
			t.Fatal("unexpected timestamp evaluations", expr)
		}
		return model.Vector{}, nil
	}
	if _, err := queryFamilies(context.Background(), query, names, filter, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTelemetryEvaluationSharesPrometheusMillisecondBoundary(t *testing.T) {
	now := time.Unix(1760000000, 123999900).UTC()
	boundary := now.Truncate(time.Millisecond)
	id := resourceview.DeviceIdentity{Node: "host", NodeUID: "uid", Provider: biz.NvidiaGPUDevice, DeviceID: "GPU-01"}
	facts := resourceview.Snapshot{At: now, NodeCoverage: completeCoverage("node", "shared_node_informer", 1), Devices: []resourceview.Device{{Identity: id, Kind: "physical"}}}
	g := &MetricsGenerator{resourceSource: &sequenceFacts{available: []resourceview.Snapshot{facts, facts}}, now: func() time.Time { return now }}
	sampleAt := boundary
	query := func(_ context.Context, expr string, at time.Time) (model.Value, error) {
		if !at.Equal(boundary) {
			t.Fatalf("unrounded query evaluation %v; expected %v", at, boundary)
		}
		name := "up"
		labels := model.Metric{"node": "host", "job": "dcgm", "instance": "h:9400"}
		sampled := boundary
		if !strings.Contains(expr, "timestamp(up)") {
			name = "DCGM_FI_DEV_GPU_UTIL"
			labels["UUID"] = "GPU-01"
			sampled = sampleAt
		}
		labels[model.MetricNameLabel] = model.LabelValue(name)
		ts := labels.Clone()
		ts[model.MetricNameLabel] = model.LabelValue(sampleTimePrefix + name)
		return model.Vector{{Metric: labels, Value: 1}, {Metric: ts, Value: model.SampleValue(float64(sampled.UnixMilli()) / 1000)}}, nil
	}
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	row := g.DeviceObservations()[id].UtilizationRatio
	if value, known := row.KnownValue(); !known || value != .01 || !row.State.GetObservedAt().AsTime().Equal(boundary) {
		t.Fatalf("valid boundary sample lost: %v", row)
	}
	sampleAt = boundary.Add(time.Millisecond)
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if q := g.DeviceObservations()[id].UtilizationRatio; q.Value != nil || q.State.GetReason() != "invalid_sample" {
		t.Fatalf("accepted a future timestamp: %+v", q)
	}
}

func TestDCGMMemoryRejectsConflictingDeviceIdentityAcrossAliases(t *testing.T) {
	at := time.Unix(1_760_000_000, 0)
	target := dcgmTarget{Job: "dcgm", Instance: "host:9400"}
	targets := map[dcgmTarget]dcgmTargetEvidence{target: {}}
	card := resourceview.Device{Identity: resourceview.DeviceIdentity{Node: "host", DeviceID: "GPU-01", Provider: biz.NvidiaGPUDevice}}
	sample := func(name string, value float64) dcgmSample {
		return dcgmSample{Value: value, SampleAt: at, Labels: map[string]string{
			"__name__": name, "UUID": "GPU-01", "node": "host", "job": target.Job, "instance": target.Instance,
		}}
	}
	used := sample("DCGM_FI_DEV_FB_USED", 40)
	alias := sample("DCGM_FI_DEV_FB_USED_BYTES", 40*bytesPerMiB)
	free := sample("DCGM_FI_DEV_FB_FREE", 60)
	rows := rawFamilies{used.Labels["__name__"]: {used}, alias.Labels["__name__"]: {alias}, free.Labels["__name__"]: {free}}
	for _, field := range []string{"device", "gpu", "modelName", "DRIVER_VERSION"} {
		// Only the non-preferred used-memory alias carries this identity field.
		used.Labels[field], free.Labels[field] = "first", "second"
		if got := deviceReadings(card, rows, at, nil, targets).total; !errors.Is(got.err, errDCGMAmbiguous) {
			t.Fatalf("combined conflicting %s: %v", field, got)
		}
		delete(used.Labels, field)
		delete(free.Labels, field)
	}
	free.Labels["device"] = "nvidia0"
	if got := deviceReadings(card, rows, at, nil, targets).total; got.err != nil || got.selection.Value != 100 {
		t.Fatalf("absent optional identity must not invalidate a confirmed UUID: %v", got)
	}
}
