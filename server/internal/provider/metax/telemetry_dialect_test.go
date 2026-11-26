package metax

import (
	"errors"
	"testing"
	"time"

	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func TestMetaXC500RequiresWholeCardReadings(t *testing.T) {
	at := time.Now().UTC()
	dialect := NewTelemetryDialect()
	d := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: metaxProviderName, Node: "host", DeviceID: "mx-01"}, Model: "C500"}
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{{Job: "mx", Instance: "exporter"}: {}}
	sample := func(name, die string, v float64) provider.TelemetrySample {
		l := map[string]string{"__name__": name, "job": "mx", "instance": "exporter", "Hostname": "host", "uuid": "mx-01", "deviceId": "0", "die_id": die, "modelName": "C500"}
		return provider.TelemetrySample{Labels: l, Value: v, SampleAt: at}
	}
	data := provider.TelemetryRawFamilies{metaxState: {sample(metaxState, "0", 1)}, metaxUtilization: {sample(metaxUtilization, "0", 25)}, metaxTotal: {sample(metaxTotal, "0", 100)}}
	data[metaxTotal][0].Labels["type"] = "xtt"
	read := func() provider.PhysicalReadings { return dialect.ReadDevice(d, data, at, nil, targets) }
	if r := read(); r.Util.Err != nil || r.Util.Selection.Value != 25 || !errors.Is(r.Used.Err, provider.ErrTelemetryMissing) || !errors.Is(r.Total.Err, provider.ErrTelemetryMissing) {
		t.Fatalf("XTT or missing memory confused with VRAM: %+v", r)
	}
	other := sample(metaxState, "1", 1)
	other.Labels["uuid"] = "mx-02"
	data[metaxState] = append(data[metaxState], other)
	if r := read(); r.Util.Err != nil {
		t.Fatalf("another card on the same target invalidated the C500: %v", r.Util.Err)
	}
	data[metaxState][1] = sample(metaxState, "1", 1)
	if r := read(); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("single reporting die of multi-die card accepted: %+v", r)
	}
	data[metaxState] = data[metaxState][:1]
	extra := sample(metaxTotal, "1", 100)
	extra.Labels["type"] = "vram"
	data[metaxTotal] = append(data[metaxTotal], extra)
	if r := read(); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("second die in memory was hidden by incomplete state: %+v", r)
	}
	data[metaxTotal] = data[metaxTotal][:1]
	data[metaxUtilization][0].SampleAt = at.Add(-time.Second)
	if r := read(); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("reading joined to a different scrape: %+v", r)
	}
	data[metaxUtilization][0].SampleAt = at
	data[metaxUtilization][0].Labels["exported_Hostname"] = "another-host"
	if r := read(); !errors.Is(r.Util.Err, provider.ErrTelemetryAmbiguous) {
		t.Fatalf("reading joined to a different node: %+v", r)
	}
}

func TestMetaXRegistrationModeAndExporterModel(t *testing.T) {
	at := time.Now().UTC()
	dialect := NewTelemetryDialect()
	id := resourceview.DeviceIdentity{Provider: metaxProviderName, Node: "host", NodeUID: "uid", DeviceID: "GPU-123"}
	targets := map[provider.TelemetryTarget]provider.TelemetryTargetEvidence{{Job: "mx", Instance: "exporter"}: {}}
	for _, tc := range []struct {
		registration, observed, die string
		want                        error
	}{
		{"sgpu", "MXC500", "0", nil},
		{"native", "C500", "0", nil},
		{"", "MXC500", "0", nil},
		{"sgpu", "MXC600", "0", provider.ErrTelemetryMissing},
		{"C500", "MXC600", "0", provider.ErrTelemetryAmbiguous},
		{"native", "MXC500", "", provider.ErrTelemetryAmbiguous},
	} {
		d := resourceview.Device{Identity: id, Kind: "physical", Model: tc.registration}
		if _, ok := dialect.BatchKey(d); !ok {
			t.Errorf("registration %q skipped the exporter query", tc.registration)
			continue
		}
		sample := func(name string, value float64) provider.TelemetrySample {
			return provider.TelemetrySample{Labels: map[string]string{"__name__": name, "job": "mx", "instance": "exporter", "uuid": id.DeviceID, "Hostname": id.Node, "deviceId": "0", "die_id": tc.die, "modelName": tc.observed}, Value: value, SampleAt: at}
		}
		got := dialect.ReadDevice(d, provider.TelemetryRawFamilies{metaxState: {sample(metaxState, 1)}, metaxUtilization: {sample(metaxUtilization, 42)}}, at, nil, targets).Util
		if !errors.Is(got.Err, tc.want) || tc.want == nil && got.Selection.Value != 42 {
			t.Errorf("registration %q, observed %q, die %q: got %v, %v", tc.registration, tc.observed, tc.die, got.Selection.Value, got.Err)
		}
	}
}
