package exporter

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func TestExporterTelemetryCoreHasZeroVendorImportsOrLiterals(t *testing.T) {
	forbiddenTokens := []string{"nvidia", "dcgm", "hygon", "hcu", "dcu", "metax", "mx_", "ascend", "npu_", "mlu", "cambricon", "mthreads", "amd.com", "huawei"}
	files := []string{"resource_telemetry.go", "observations.go", "resource_metrics.go"}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			impPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if impPath == "vgpu/internal/biz" || (impPath != "vgpu/internal/provider" && strings.HasPrefix(impPath, "vgpu/internal/provider")) {
				t.Errorf("%s imports forbidden package %q", path, impPath)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			lower := strings.ToLower(val)
			for _, tok := range forbiddenTokens {
				if strings.Contains(lower, tok) {
					t.Errorf("%s contains forbidden vendor literal %q (%q)", path, tok, val)
				}
			}
			return true
		})
	}
}

type syntheticAcmeTelemetryDialect struct{}

func (syntheticAcmeTelemetryDialect) Name() string         { return "Acme" }
func (syntheticAcmeTelemetryDialect) ProviderName() string { return "Acme" }
func (syntheticAcmeTelemetryDialect) SourceName() string   { return "acme_exporter" }
func (syntheticAcmeTelemetryDialect) MemoryScale() float64 { return float64(provider.BytesPerKiB) }

func (syntheticAcmeTelemetryDialect) BatchKey(device resourceview.Device) (string, bool) {
	return device.Identity.DeviceID, true
}

func (syntheticAcmeTelemetryDialect) QuerySpec(escapedKeys []string) ([]string, string) {
	return []string{"acme_gpu_util", "acme_mem_used", "acme_mem_total"}, fmt.Sprintf("acme_id=~%q", strings.Join(escapedKeys, "|"))
}

func (syntheticAcmeTelemetryDialect) SampleKey(labels map[string]string) string {
	return labels["acme_id"]
}

func (syntheticAcmeTelemetryDialect) DeviceKey(device resourceview.Device) string {
	return device.Identity.DeviceID
}

var syntheticAcmeSchema = provider.NodeMetricSchema{
	DeviceIDLabel:     "acme_id",
	IndexLabel:        "chip_idx",
	EntityLabel:       "slot",
	ModelLabel:        "model",
	UtilizationMetric: "acme_gpu_util",
	RequireNodeLabel:  true,
}

func (syntheticAcmeTelemetryDialect) ReadDevice(
	device resourceview.Device,
	samples provider.TelemetryRawFamilies,
	at time.Time,
	nodeCreated *time.Time,
	targets map[provider.TelemetryTarget]provider.TelemetryTargetEvidence,
) provider.PhysicalReadings {
	get := func(name string) provider.TelemetryReading {
		return syntheticAcmeSchema.Reading(name, device, samples[name], at, nodeCreated, targets)
	}
	used, total := syntheticAcmeSchema.MemoryPair(get("acme_mem_used"), get("acme_mem_total"))
	return provider.PhysicalReadings{
		Util:  get("acme_gpu_util"),
		Used:  used,
		Total: total,
	}
}

func TestSyntheticTelemetryDialectRequiresZeroCoreChanges(t *testing.T) {
	at := time.UnixMilli(1760000000123).UTC()
	sampled := at.Add(-15 * time.Second)
	id := resourceview.DeviceIdentity{Provider: "Acme", Node: "acme-node", NodeUID: "acme-node-uid", DeviceID: "acme-0"}
	facts := resourceview.Snapshot{
		At:           at,
		NodeCoverage: completeCoverage("node", "shared_node_informer", 1),
		Nodes:        []resourceview.Node{{Name: "acme-node", UID: "acme-node-uid"}},
		Devices: []resourceview.Device{{
			Identity: id, Kind: "physical", Model: "Acme-X1", Vendor: "acme",
			States: map[string]*viewv1.FieldState{"kind": {State: resourceview.Known}},
		}},
	}
	g := &MetricsGenerator{
		resourceSource:    &sequenceFacts{available: []resourceview.Snapshot{facts}},
		now:               func() time.Time { return at },
		telemetryDialects: []provider.TelemetryDialect{syntheticAcmeTelemetryDialect{}},
	}
	query := func(_ context.Context, expr string, evalAt time.Time) (model.Value, error) {
		var rows model.Vector
		add := func(name string, l model.Metric, v float64) {
			l = l.Clone()
			l[model.MetricNameLabel] = model.LabelValue(name)
			rows = append(rows, &model.Sample{Metric: l, Value: model.SampleValue(v), Timestamp: model.Time(evalAt.UnixMilli())})
			ts := l.Clone()
			ts[model.MetricNameLabel] = model.LabelValue(sampleTimePrefix + name)
			rows = append(rows, &model.Sample{Metric: ts, Value: model.SampleValue(float64(sampled.UnixMilli()) / 1000), Timestamp: model.Time(evalAt.UnixMilli())})
		}
		base := model.Metric{"job": "acme-exp", "instance": "acme-node:9100", "node": "acme-node", "acme_id": "acme-0", "chip_idx": "0", "slot": "0", "model": "Acme-X1"}
		switch {
		case strings.Contains(expr, "timestamp(up)"):
			add("up", model.Metric{"job": "acme-exp", "instance": "acme-node:9100", "node": "acme-node"}, 1)
		case strings.Contains(expr, "acme_gpu_util"):
			add("acme_gpu_util", base, 75)
			add("acme_mem_used", base, 2048)
			add("acme_mem_total", base, 8192)
		default:
			t.Fatalf("unexpected query selector: %s", expr)
		}
		return rows, nil
	}
	if err := g.refreshResourceSnapshot(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	obs := g.DeviceObservations()[id]
	if u, ok := obs.UtilizationRatio.KnownValue(); !ok || u != .75 || obs.UtilizationRatio.State.Source != "acme_exporter" {
		t.Fatalf("unexpected utilization observation: %+v", obs.UtilizationRatio)
	}
	if used, ok := obs.MemoryUsedBytes.KnownValue(); !ok || used != 2048*1024 {
		t.Fatalf("unexpected memory used observation: %+v", obs.MemoryUsedBytes)
	}
	if total, ok := obs.MemoryPhysicalBytes.KnownValue(); !ok || total != 8192*1024 {
		t.Fatalf("unexpected memory total observation: %+v", obs.MemoryPhysicalBytes)
	}
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(newResourceMetricCollector(g))
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	gathered := map[string]float64{}
	for _, f := range families {
		for _, m := range f.Metric {
			for _, l := range m.Label {
				if l.GetName() == "source" && l.GetValue() == "acme_exporter" {
					gathered[f.GetName()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	if gathered["hami_webui_device_utilization_ratio"] != .75 ||
		gathered["hami_webui_device_memory_used_bytes"] != 2048*1024 ||
		gathered["hami_webui_device_memory_physical_bytes"] != 8192*1024 {
		t.Fatalf("missing or wrong gathered prometheus metrics for synthetic dialect: %+v", gathered)
	}
}
