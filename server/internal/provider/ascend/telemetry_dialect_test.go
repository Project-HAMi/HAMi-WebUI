package ascend

import (
	"errors"
	"testing"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func TestAscendPhysicalCopiesRequireOneFreshChipAndTarget(t *testing.T) {
	at := time.Now().UTC()
	device := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: AscendDevice, Node: "node", DeviceID: "chip"}, Model: "Ascend910B2"}
	sample := func(pod string) provider.TelemetrySample {
		return provider.TelemetrySample{SampleAt: at, Value: 50, Labels: map[string]string{"__name__": ascendUtilization, "node": "node", "job": "npu", "instance": "npu:8082", "model_name": device.Model, "vdie_id": "chip", "id": "0", "pod_name": pod}}
	}
	evidence := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{{Job: "npu", Instance: "npu:8082"}: {TargetLabels: map[string]string{"node": "node"}}, {Job: "npu", Instance: "second:8082"}: {TargetLabels: map[string]string{"node": "node"}}}

	read := func(created *time.Time, samples ...provider.TelemetrySample) provider.TelemetryReading {
		return ascendSchema.Reading(ascendUtilization, device, samples, at, created, evidence)
	}
	first, second := sample("p1"), sample("p2")
	if got := read(nil, first, second); got.Err != nil || got.Selection.Value != 50 {
		t.Fatal("identical chip copies", got)
	}
	second.Value = 51
	if got := read(nil, first, second); !errors.Is(got.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatal("conflicting copies", got)
	}
	second = sample("p2")
	second.Labels["instance"] = "second:8082"
	if got := read(nil, first, second); !errors.Is(got.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatal("duplicate targets", got)
	}
	old := provider.TelemetryTarget{Job: "npu", Instance: "second:8082"}
	evidence[old] = provider.TelemetryTargetEvidence{Unavailable: provider.ErrTelemetryTargetDown}
	second.SampleAt = at.Add(-time.Minute)
	for _, rows := range [][]provider.TelemetrySample{{first, second}, {second, first}} {
		if got := read(nil, rows...); got.Err != nil || got.Selection.Source.Instance != "npu:8082" || got.Selection.Value != 50 {
			t.Fatalf("down target masked its healthy replacement: %v", got)
		}
	}
	if got := read(nil, second); !errors.Is(got.Err, provider.ErrTelemetryTargetDown) {
		t.Fatalf("down target alone: %v", got)
	}
	evidence[old] = provider.TelemetryTargetEvidence{TargetLabels: map[string]string{"node": "node"}}
	created := at.Add(-time.Second)
	second.Labels["model_name"] = "Ascend310P"
	if got := read(&created, second, first); got.Err != nil {
		t.Fatalf("a prior node's model must not conflict with its replacement: %v", got)
	}
	delete(evidence, old)
	if got := read(nil, first, second); got.Err != nil {
		t.Fatalf("target without scrape evidence masked the healthy target: %v", got)
	}
	second = sample("")
	second.Labels["model_name"] = "910B-Ascend-V1"
	if got := read(nil, second); got.Err != nil {
		t.Fatal("vendor model spelling", got)
	}
	second.Labels["model_name"] = "Ascend310P"
	if got := read(nil, second); !errors.Is(got.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatal("conflicting accelerator memory bank", got)
	}
	for _, field := range []string{"vdie_id", "node", "__name__", "v_dev_id"} {
		second = sample("p2")
		second.Labels[field] = "different"
		if got := read(nil, second); got.Err == nil {
			t.Fatalf("admitted incompatible %s", field)
		}
	}
	second = sample("")
	second.SampleAt = at.Add(-provider.TelemetryMaxSampleAge - time.Second)
	if got := read(nil, second); !errors.Is(got.Err, provider.ErrTelemetryStale) {
		t.Fatal("stale sample", got)
	}
	second.SampleAt = created.Add(-time.Second)
	if got := read(&created, second); !errors.Is(got.Err, provider.ErrTelemetryStale) {
		t.Fatal("pre-node-recreation sample", got)
	}
	second = sample("")
	second.Value = 101
	if got := read(nil, second); !errors.Is(got.Err, provider.ErrTelemetryInvalid) {
		t.Fatal("invalid utilization", got)
	}
}

func TestAscendMemoryPairRejectsDifferentTimeAndOverCapacity(t *testing.T) {
	now := time.Now()
	used := provider.TelemetryReading{Selection: provider.TelemetrySelection{SampleAt: now, Value: 1}}
	total := provider.TelemetryReading{Selection: provider.TelemetrySelection{SampleAt: now.Add(-time.Minute), Value: 8}}
	if u, c := ascendSchema.MemoryPair(used, total); !errors.Is(u.Err, provider.ErrTelemetryAmbiguous) || !errors.Is(c.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("mixed times: %v %v", u, c)
	}
	total.Selection.SampleAt = now
	used.Selection.Labels = map[string]string{"model_name": "910B2"}
	total.Selection.Labels = map[string]string{"model_name": "910B3"}
	if u, c := ascendSchema.MemoryPair(used, total); !errors.Is(u.Err, provider.ErrTelemetryAmbiguous) || !errors.Is(c.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("mixed models: %v %v", u, c)
	}
	total.Selection.Labels = used.Selection.Labels
	used.Selection.Value = 9
	if u, c := ascendSchema.MemoryPair(used, total); !errors.Is(u.Err, provider.ErrTelemetryInvalid) || !errors.Is(c.Err, provider.ErrTelemetryInvalid) {
		t.Fatalf("used > total: %v %v", u, c)
	}
}

func TestAscendModelEvidenceAppliesToAllPhysicalMetrics(t *testing.T) {
	at := time.Now()
	dialect := NewTelemetryDialect()
	device := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: AscendDevice, Node: "worker", NodeUID: "live-uid", DeviceID: "chip-0"}}
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{{Cluster: "foreign", Job: "npu", Instance: "worker:8080"}: {TargetLabels: map[string]string{"node": "worker"}}}
	for _, tc := range []struct {
		registered, observed string
		conflict             bool
	}{
		{"Ascend910B2", "Ascend910B3", true},
		{"Ascend910B4", "910B4-1", true},
		{"Ascend910B3", "910B3-Ascend-V1", false},
		{"Ascend910B4-1", "910B4-1-Ascend-V2", false},
		{"Ascend910B2", "910B-Ascend-V1", false},
		{"Ascend910A", "910Awn-Ascend-V1", false},
		{"Ascend310P", "310P3", false},
	} {
		t.Run(tc.registered+"/"+tc.observed, func(t *testing.T) {
			device.Model = tc.registered
			u, total := ascendMemoryFamilies(tc.registered)
			samples := provider.TelemetryRawFamilies{}
			for metric, value := range map[string]float64{ascendUtilization: 83, u: 128, total: 512} {
				samples[metric] = []provider.TelemetrySample{{Value: value, SampleAt: at, Labels: map[string]string{
					"__name__": metric, "node": "worker", "vdie_id": "chip-0", "id": "0", "model_name": tc.observed,
					"job": "npu", "instance": "worker:8080", "cluster": "foreign",
				}}}
			}
			got := dialect.ReadDevice(device, samples, at, nil, targets)
			for _, reading := range []provider.TelemetryReading{got.Util, got.Used, got.Total} {
				if tc.conflict {
					if !errors.Is(reading.Err, provider.ErrTelemetryAmbiguous) {
						t.Fatalf("accepted conflicting model: %+v", reading)
					}
				} else if reading.Err != nil {
					t.Fatal(reading.Err)
				}
			}
			if !tc.conflict && (got.Util.Selection.Value != 83 || got.Used.Selection.Value != 128 || got.Total.Selection.Value != 512) {
				t.Fatalf("lost matching observations: %+v", got)
			}
		})
	}
}
