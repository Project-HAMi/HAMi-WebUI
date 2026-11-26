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
	"vgpu/internal/provider"
	"vgpu/internal/provider/hygon"
	"vgpu/internal/provider/nvidia"
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
	hcu := key(biz.HygonHCUDevice, "HCU-board-01")
	dcu := key(biz.HygonGPUDevice, "DCU-board-01")
	metax := key(biz.MetaxGPUDevice, "mx-01")
	mislabelled := key(biz.MetaxGPUDevice, "mx-02")
	facts := resourceview.Snapshot{At: at, NodeCoverage: completeCoverage("node", "shared_node_informer", 1), Devices: []resourceview.Device{
		{Identity: metax, Kind: "physical", Model: "sgpu"},
		{Identity: mislabelled, Kind: "physical", Model: "C500"},
		{Identity: gpu, Kind: "physical", Model: "H100"}, {Identity: chip, Kind: "physical", Model: "Ascend310P"},
		{Identity: hcu, Kind: "physical", Model: "HCU-K100_AI"},
		{Identity: dcu, Kind: "physical", Model: "DCU-Z100L"},
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
		h := model.Metric{"job": "hygon", "instance": "h:8000", "node": "host"}
		m := model.Metric{"job": "mx-exporter", "instance": "h:8081"}
		switch {
		case strings.Contains(expr, "timestamp(up)"):
			if unavailable {
				return nil, errors.New("prometheus timeout")
			}
			add("up", a, targetUp)
			add("up", n, targetUp)
			add("up", m, targetUp)
			add("up", h, targetUp)
		case strings.Contains(expr, "hcu_usedmemory_bytes"):
			h["minor_number"], h["device_id"], h["hcu_pod_name"] = "0", "board-01", "exclusive"
			add("hcu_utilizationrate", h, 10)
			add("hcu_usedmemory_bytes", h, 4<<30)
			add("hcu_memorycap_bytes", h, 64<<30)
			h["vhcu_minor_number"] = "42"
			add("vhcu_utilizationrate", h, 40)
		case strings.Contains(expr, "dcu_usedmemory_bytes"):
			h["minor_number"], h["device_id"] = "1", "board-01"
			add("dcu_utilizationrate", h, 37.5)
			add("dcu_usedmemory_bytes", h, 123456789)
			add("dcu_memorycap_bytes", h, 32<<30)
		case strings.Contains(expr, "mx_gpu_usage"):
			m["Hostname"], m["uuid"], m["deviceId"], m["die_id"], m["modelName"] = "host", "mx-01", "0", "0", "MXC500"
			add("mx_gpu_state", m, 1)
			for _, pod := range []string{"first", "second"} {
				m["exported_pod"] = model.LabelValue(pod)
				add("mx_gpu_usage", m, 0)
			}
			m["type"] = "vram"
			add("mx_memory_used", m, 4*1024*1024)
			add("mx_memory_total", m, 64*1024*1024)
			m["type"] = "xtt"
			add("mx_memory_total", m, 8*1024*1024)
			delete(m, "type")
			m["Hostname"], m["node"], m["uuid"] = "mx-exporter-pod", "host", "mx-02"
			add("mx_gpu_state", m, 1)
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
				add("npu_chip_info_utilization", a, 0)
			}
			add("npu_chip_info_used_memory", a, 128)
			add("npu_chip_info_total_memory", a, 8192)
			add("npu_chip_info_hbm_used_memory", a, 3000)
			add("npu_chip_info_hbm_total_memory", a, 4096)
			a["v_dev_id"] = "1"
			add("npu_chip_info_utilization", a, 99)
		default:
			t.Fatalf("unrecognized selector %s", expr)
		}
		return rows, nil
	}
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	obs := g.DeviceObservations()
	for id, want := range map[resourceview.DeviceIdentity][3]float64{metax: {0, 4 * 1024 * bytesPerMiB, 64 * 1024 * bytesPerMiB}, gpu: {.75, 256 * bytesPerMiB, 1024 * bytesPerMiB}, chip: {0, 128 * bytesPerMiB, 8192 * bytesPerMiB}, hcu: {.10, 4 << 30, 64 << 30}, dcu: {.375, 123456789, 32 << 30}} {
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
	if q := obs[mislabelled].UtilizationRatio; q.State == nil || q.State.State != resourceview.Conflict {
		t.Fatalf("registered UUID with contradictory host labels: %+v", q)
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
	for id, d := range g.DeviceObservations() {
		if id == mislabelled {
			continue // It has never supplied a trustworthy reading.
		}
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
	target := provider.TelemetrySample{Labels: map[string]string{"job": "j", "instance": "i"}, Value: 1, SampleAt: now}
	targets := targetEvidence([]provider.TelemetrySample{target, target}, now)
	if !targets[provider.TelemetryTarget{Job: "j", Instance: "i"}].Ambiguous {
		t.Fatal("duplicate target marked usable")
	}
	query := func(context.Context, string, time.Time) (model.Value, error) {
		return model.Vector{{Metric: model.Metric{"__name__": "up", "instance": "a"}, Value: 1},
			{Metric: model.Metric{"__name__": sampleTimePrefix + "up", "instance": "b"}, Value: 1794661258}}, nil
	}
	_, err := queryFamilies(context.Background(), query, []string{"up"}, "", now)
	if !errors.Is(err, provider.ErrTelemetryInvalid) {
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
	dialect := nvidia.NewTelemetryDialect()
	target := provider.TelemetryTarget{Job: "dcgm", Instance: "host:9400"}
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{target: {}}
	card := resourceview.Device{Identity: resourceview.DeviceIdentity{Node: "host", DeviceID: "GPU-01", Provider: biz.NvidiaGPUDevice}}
	sample := func(name string, value float64) provider.TelemetrySample {
		return provider.TelemetrySample{Value: value, SampleAt: at, Labels: map[string]string{
			"__name__": name, "UUID": "GPU-01", "node": "host", "job": target.Job, "instance": target.Instance,
		}}
	}
	used := sample("DCGM_FI_DEV_FB_USED", 40)
	alias := sample("DCGM_FI_DEV_FB_USED_BYTES", 40*bytesPerMiB)
	free := sample("DCGM_FI_DEV_FB_FREE", 60)
	rows := provider.TelemetryRawFamilies{used.Labels["__name__"]: {used}, alias.Labels["__name__"]: {alias}, free.Labels["__name__"]: {free}}
	for _, field := range []string{"device", "gpu", "modelName", "DRIVER_VERSION"} {
		// Only the non-preferred used-memory alias carries this identity field.
		used.Labels[field], free.Labels[field] = "first", "second"
		if got := dialect.ReadDevice(card, rows, at, nil, targets).Total; !errors.Is(got.Err, provider.ErrTelemetryAmbiguous) {
			t.Fatalf("combined conflicting %s: %v", field, got)
		}
		delete(used.Labels, field)
		delete(free.Labels, field)
	}
	free.Labels["device"] = "nvidia0"
	if got := dialect.ReadDevice(card, rows, at, nil, targets).Total; got.Err != nil || got.Selection.Value != 100 {
		t.Fatalf("absent optional identity must not invalidate a confirmed UUID: %v", got)
	}
}

func TestDCUReadingRequiresCurrentNodeAndUnambiguousBoardSerial(t *testing.T) {
	at := time.Unix(1760000000, 0)
	created := at.Add(-time.Minute)
	dialect := hygon.NewDCUTelemetryDialect()
	card := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: biz.HygonGPUDevice, Node: "worker", NodeUID: "new", DeviceID: "DCU-board-A"}}
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{{Job: "hygon", Instance: "node:8000"}: {}, {Job: "hygon", Instance: "duplicate:8000"}: {}}
	sample := func(serial string) provider.TelemetrySample {
		return provider.TelemetrySample{Value: 0, SampleAt: at, Labels: map[string]string{"__name__": "dcu_utilizationrate", "node": "worker", "job": "hygon", "instance": "node:8000", "minor_number": "1", "device_id": serial}}
	}
	read := func(card resourceview.Device, rows ...provider.TelemetrySample) provider.PhysicalReadings {
		return dialect.ReadDevice(card, provider.TelemetryRawFamilies{"dcu_utilizationrate": rows}, at, &created, targets)
	}
	if r := read(card, sample("board-A")); r.Util.Err != nil || r.Util.Selection.Value != 0 || !errors.Is(r.Used.Err, provider.ErrTelemetryMissing) || !errors.Is(r.Total.Err, provider.ErrTelemetryMissing) {
		t.Fatalf("observed zero and missing memory: %+v", r)
	}
	for _, id := range []string{"1", "DCU-1", "DCU-N/A", "DCU-"} {
		candidate := card
		candidate.Identity.DeviceID = id
		if r := read(candidate, sample(strings.TrimPrefix(id, "DCU-"))); !errors.Is(r.Util.Err, provider.ErrTelemetryMissing) {
			t.Errorf("ambiguous registration %q: %v", id, r.Util.Err)
		}
	}
	for _, node := range []string{"", "another-worker"} {
		other := sample("board-A")
		other.Labels["node"] = node
		if r := read(card, other); !errors.Is(r.Util.Err, provider.ErrTelemetryMissing) {
			t.Errorf("node %q: %v", node, r.Util.Err)
		}
	}
	relabeled := sample("board-A")
	relabeled.Labels["exported_node"] = "original-worker"
	if r := read(card, relabeled); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Errorf("conflicting exporter and target nodes: %v", r.Util.Err)
	}
	old := sample("board-A")
	old.SampleAt = created.Add(-time.Second)
	if r := read(card, old); !errors.Is(r.Util.Err, provider.ErrTelemetryStale) {
		t.Errorf("previous same-name node: %v", r.Util.Err)
	}
	second := sample("board-A")
	second.Labels["instance"] = "duplicate:8000"
	if r := read(card, sample("board-A"), second); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Errorf("two active sources for board: %v", r.Util.Err)
	}
	for _, v := range []float64{-1, 101} {
		invalid := sample("board-A")
		invalid.Value = v
		if r := read(card, invalid); !errors.Is(r.Util.Err, provider.ErrTelemetryInvalid) {
			t.Errorf("stored invalid percent %v: %v", v, r.Util.Err)
		}
	}
}
