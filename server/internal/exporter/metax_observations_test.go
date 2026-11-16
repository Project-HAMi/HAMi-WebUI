package exporter

import (
	"errors"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

func TestMetaXC500RequiresWholeCardReadings(t *testing.T) {
	at := time.Now().UTC()
	d := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: biz.MetaxGPUDevice, Node: "host", DeviceID: "mx-01"}, Model: "C500"}
	targets := map[dcgmTarget]dcgmTargetEvidence{{Job: "mx", Instance: "exporter"}: {}}
	sample := func(name, die string, v float64) dcgmSample {
		l := map[string]string{"__name__": name, "job": "mx", "instance": "exporter", "Hostname": "host", "uuid": "mx-01", "deviceId": "0", "die_id": die, "modelName": "C500"}
		return dcgmSample{Labels: l, Value: v, SampleAt: at}
	}
	data := rawFamilies{metaxState: {sample(metaxState, "0", 1)}, metaxUtilization: {sample(metaxUtilization, "0", 25)}, metaxTotal: {sample(metaxTotal, "0", 100)}}
	data[metaxTotal][0].Labels["type"] = "xtt"
	read := func() physicalReadings { return metaxReadings(d, data, at, nil, targets) }
	if r := read(); r.util.err != nil || r.util.selection.Value != 25 || !errors.Is(r.used.err, errDCGMMissing) || !errors.Is(r.total.err, errDCGMMissing) {
		t.Fatalf("XTT or missing memory confused with VRAM: %+v", r)
	}
	other := sample(metaxState, "1", 1)
	other.Labels["uuid"] = "mx-02"
	data[metaxState] = append(data[metaxState], other)
	if r := read(); r.util.err != nil {
		t.Fatalf("another card on the same target invalidated the C500: %v", r.util.err)
	}
	data[metaxState][1] = sample(metaxState, "1", 1)
	if r := read(); !errors.Is(r.util.err, errDCGMAmbiguous) {
		t.Fatalf("single reporting die of multi-die card accepted: %+v", r)
	}
	data[metaxState] = data[metaxState][:1]
	extra := sample(metaxTotal, "1", 100)
	extra.Labels["type"] = "vram"
	data[metaxTotal] = append(data[metaxTotal], extra)
	if r := read(); !errors.Is(r.util.err, errDCGMAmbiguous) {
		t.Fatalf("second die in memory was hidden by incomplete state: %+v", r)
	}
	data[metaxTotal] = data[metaxTotal][:1]
	data[metaxUtilization][0].SampleAt = at.Add(-time.Second)
	if r := read(); !errors.Is(r.util.err, errDCGMAmbiguous) {
		t.Fatalf("reading joined to a different scrape: %+v", r)
	}
	data[metaxUtilization][0].SampleAt = at
	data[metaxUtilization][0].Labels["exported_Hostname"] = "another-host"
	if r := read(); !errors.Is(r.util.err, errDCGMAmbiguous) {
		t.Fatalf("reading joined to a different node: %+v", r)
	}
}

func TestMetaXRegistrationModeAndExporterModel(t *testing.T) {
	at := time.Now().UTC()
	id := resourceview.DeviceIdentity{Provider: biz.MetaxGPUDevice, Node: "host", NodeUID: "uid", DeviceID: "GPU-123"}
	targets := map[dcgmTarget]dcgmTargetEvidence{{Job: "mx", Instance: "exporter"}: {}}
	for _, tc := range []struct {
		registration, observed, die string
		want                        error
	}{
		{"sgpu", "MXC500", "0", nil},
		{"native", "C500", "0", nil},
		{"", "MXC500", "0", nil},
		{"sgpu", "MXC600", "0", errDCGMMissing},
		{"C500", "MXC600", "0", errDCGMAmbiguous},
		{"native", "MXC500", "", errDCGMAmbiguous},
	} {
		d := resourceview.Device{Identity: id, Kind: "physical", Model: tc.registration}
		plan := telemetryPlan([]resourceview.Device{d})
		if len(plan) != 1 {
			t.Errorf("registration %q skipped the exporter query", tc.registration)
			continue
		}
		sample := func(name string, value float64) dcgmSample {
			return dcgmSample{Labels: map[string]string{"__name__": name, "job": "mx", "instance": "exporter", "uuid": id.DeviceID, "Hostname": id.Node, "deviceId": "0", "die_id": tc.die, "modelName": tc.observed}, Value: value, SampleAt: at}
		}
		got := deviceReadings(d, rawFamilies{metaxState: {sample(metaxState, 1)}, metaxUtilization: {sample(metaxUtilization, 42)}}, at, nil, targets).util
		if !errors.Is(got.err, tc.want) || tc.want == nil && got.selection.Value != 42 {
			t.Errorf("registration %q, observed %q, die %q: got %v, %v", tc.registration, tc.observed, tc.die, got.selection.Value, got.err)
		}
	}
}
