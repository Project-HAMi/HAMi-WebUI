package exporter

import (
	"errors"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

func TestAscendPhysicalCopiesRequireOneFreshChipAndTarget(t *testing.T) {
	at := time.Now().UTC()
	device := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: biz.AscendGPUDevice, Node: "node", DeviceID: "chip"}, Model: "Ascend910B2"}
	sample := func(pod string) dcgmSample {
		return dcgmSample{SampleAt: at, Value: 50, Labels: map[string]string{"__name__": ascendUtilization, "node": "node", "job": "npu", "instance": "npu:8082", "model_name": device.Model, "vdie_id": "chip", "id": "0", "pod_name": pod}}
	}
	evidence := map[dcgmTarget]dcgmTargetEvidence{{Job: "npu", Instance: "npu:8082"}: {TargetLabels: map[string]string{"node": "node"}}, {Job: "npu", Instance: "second:8082"}: {TargetLabels: map[string]string{"node": "node"}}}

	read := func(created *time.Time, samples ...dcgmSample) dcgmReading {
		return ascendLabels.reading(ascendUtilization, device, samples, at, created, evidence)
	}
	first, second := sample("p1"), sample("p2")
	if got := read(nil, first, second); got.err != nil || got.selection.Value != 50 {
		t.Fatal("identical chip copies", got)
	}
	second.Value = 51
	if got := read(nil, first, second); !errors.Is(got.err, errDCGMAmbiguous) {
		t.Fatal("conflicting copies", got)
	}
	second = sample("p2")
	second.Labels["instance"] = "second:8082"
	if got := read(nil, first, second); !errors.Is(got.err, errDCGMAmbiguous) {
		t.Fatal("duplicate targets", got)
	}
	old := dcgmTarget{Job: "npu", Instance: "second:8082"}
	evidence[old] = dcgmTargetEvidence{Unavailable: errTargetDown}
	second.SampleAt = at.Add(-time.Minute)
	for _, rows := range [][]dcgmSample{{first, second}, {second, first}} {
		if got := read(nil, rows...); got.err != nil || got.selection.Source.Instance != "npu:8082" || got.selection.Value != 50 {
			t.Fatalf("down target masked its healthy replacement: %v", got)
		}
	}
	if got := read(nil, second); !errors.Is(got.err, errTargetDown) {
		t.Fatalf("down target alone: %v", got)
	}
	evidence[old] = dcgmTargetEvidence{TargetLabels: map[string]string{"node": "node"}}
	created := at.Add(-time.Second)
	second.Labels["model_name"] = "Ascend310P"
	if got := read(&created, second, first); got.err != nil {
		t.Fatalf("a prior node's model must not conflict with its replacement: %v", got)
	}
	delete(evidence, old)
	if got := read(nil, first, second); got.err != nil {
		t.Fatalf("target without scrape evidence masked the healthy target: %v", got)
	}
	second = sample("")
	second.Labels["model_name"] = "910B-Ascend-V1"
	if got := read(nil, second); got.err != nil {
		t.Fatal("vendor model spelling", got)
	}
	second.Labels["model_name"] = "Ascend310P"
	if got := read(nil, second); !errors.Is(got.err, errDCGMAmbiguous) {
		t.Fatal("conflicting accelerator memory bank", got)
	}
	for _, field := range []string{"vdie_id", "node", "__name__", "v_dev_id"} {
		second = sample("p2")
		second.Labels[field] = "different"
		if got := read(nil, second); got.err == nil {
			t.Fatalf("admitted incompatible %s", field)
		}
	}
	second = sample("")
	second.SampleAt = at.Add(-dcgmMaxSampleAge - time.Second)
	if got := read(nil, second); !errors.Is(got.err, errDCGMStale) {
		t.Fatal("stale sample", got)
	}
	second.SampleAt = created.Add(-time.Second)
	if got := read(&created, second); !errors.Is(got.err, errDCGMStale) {
		t.Fatal("pre-node-recreation sample", got)
	}
	second = sample("")
	second.Value = 101
	if got := read(nil, second); !errors.Is(got.err, errDCGMInvalid) {
		t.Fatal("invalid utilization", got)
	}
}

func TestAscendMemoryPairRejectsDifferentTimeAndOverCapacity(t *testing.T) {
	now := time.Now()
	used := dcgmReading{selection: dcgmSelection{SampleAt: now, Value: 1}}
	total := dcgmReading{selection: dcgmSelection{SampleAt: now.Add(-time.Minute), Value: 8}}
	if u, c := ascendLabels.memoryPair(used, total); !errors.Is(u.err, errDCGMAmbiguous) || !errors.Is(c.err, errDCGMAmbiguous) {
		t.Fatalf("mixed times: %v %v", u, c)
	}
	total.selection.SampleAt = now
	used.selection.Labels = map[string]string{"model_name": "910B2"}
	total.selection.Labels = map[string]string{"model_name": "910B3"}
	if u, c := ascendLabels.memoryPair(used, total); !errors.Is(u.err, errDCGMAmbiguous) || !errors.Is(c.err, errDCGMAmbiguous) {
		t.Fatalf("mixed models: %v %v", u, c)
	}
	total.selection.Labels = used.selection.Labels
	used.selection.Value = 9
	if u, c := ascendLabels.memoryPair(used, total); !errors.Is(u.err, errDCGMInvalid) || !errors.Is(c.err, errDCGMInvalid) {
		t.Fatalf("used > total: %v %v", u, c)
	}
}

func TestAscendModelEvidenceAppliesToAllPhysicalMetrics(t *testing.T) {
	at := time.Now()
	device := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: "Ascend", Node: "worker", NodeUID: "live-uid", DeviceID: "chip-0"}}
	targets := map[dcgmTarget]dcgmTargetEvidence{{Cluster: "foreign", Job: "npu", Instance: "worker:8080"}: {TargetLabels: map[string]string{"node": "worker"}}}
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
			samples := rawFamilies{}
			for metric, value := range map[string]float64{ascendUtilization: 83, u: 128, total: 512} {
				samples[metric] = []dcgmSample{{Value: value, SampleAt: at, Labels: map[string]string{
					"__name__": metric, "node": "worker", "vdie_id": "chip-0", "id": "0", "model_name": tc.observed,
					"job": "npu", "instance": "worker:8080", "cluster": "foreign",
				}}}
			}
			got := deviceReadings(device, samples, at, nil, targets)
			for _, reading := range []dcgmReading{got.util, got.used, got.total} {
				if tc.conflict {
					if !errors.Is(reading.err, errDCGMAmbiguous) {
						t.Fatalf("accepted conflicting model: %+v", reading)
					}
				} else if reading.err != nil {
					t.Fatal(reading.err)
				}
			}
			if !tc.conflict && (got.util.selection.Value != 83 || got.used.selection.Value != 128 || got.total.selection.Value != 512) {
				t.Fatalf("lost matching observations: %+v", got)
			}
		})
	}
}
