package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
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
		case strings.Contains(expr, "timestamp(up"):
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

func TestSingleTelemetryFlowFeedsBothLegacyAndSchema2Metrics(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Second)
	sampledSec := float64(at.Add(-10 * time.Second).UnixMilli()) / 1000
	const (
		deviceID   = "GPU-single-flow"
		nodeName   = "node-single-flow"
		nodeUID    = "uid-single-flow"
		deviceType = "A100"
	)
	type promVectorEntry struct {
		Metric map[string]string `json:"metric"`
		Value  [2]any            `json:"value"`
	}
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		expr := r.FormValue("query")
		var rows []promVectorEntry
		add := func(name string, labels map[string]string, val float64) {
			m := make(map[string]string, len(labels)+1)
			for k, v := range labels {
				m[k] = v
			}
			m["__name__"] = name
			rows = append(rows, promVectorEntry{Metric: m, Value: [2]any{float64(at.UnixMilli()) / 1000, strconv.FormatFloat(val, 'f', -1, 64)}})
			ts := make(map[string]string, len(labels)+1)
			for k, v := range labels {
				ts[k] = v
			}
			ts["__name__"] = sampleTimePrefix + name
			rows = append(rows, promVectorEntry{Metric: ts, Value: [2]any{float64(at.UnixMilli()) / 1000, strconv.FormatFloat(sampledSec, 'f', -1, 64)}})
		}
		if strings.Contains(expr, "timestamp(up") {
			add("up", map[string]string{"job": "dcgm", "instance": "node-single-flow:9400", "node": nodeName}, 1)
		} else if strings.Contains(expr, "DCGM_FI_DEV_GPU_UTIL") {
			base := map[string]string{"job": "dcgm", "instance": "node-single-flow:9400", "node": nodeName, "UUID": deviceID}
			add("DCGM_FI_DEV_GPU_UTIL", base, 42)
			add("DCGM_FI_DEV_FB_USED", base, 10240)
			add("DCGM_FI_DEV_FB_FREE", base, 30720)
		} else {
			t.Fatalf("unexpected batch PromQL query: %s", expr)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   map[string]any{"resultType": "vector", "result": rows},
		})
	}))
	defer promServer.Close()

	promClient, err := prom.NewClient(promServer.URL, 2*time.Second, prom.HTTPConfig{}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	facts := resourceview.Snapshot{
		At:           at,
		NodeCoverage: completeCoverage("node", "shared_node_informer", 1),
		Nodes:        []resourceview.Node{{Name: nodeName, UID: nodeUID}},
		Devices: []resourceview.Device{{
			Identity:             resourceview.DeviceIdentity{Provider: biz.NvidiaGPUDevice, Node: nodeName, NodeUID: nodeUID, DeviceID: deviceID},
			Kind:                 "physical",
			Vendor:               "nvidia",
			Model:                deviceType,
			MemoryCapacityBytes:  knownResource("80Gi", "bytes"),
			SlotsCapacity:        knownResource("10", "slots"),
			ComputeCapacityShare: knownResource("1", "share"),
			States:               map[string]*viewv1.FieldState{"kind": {State: resourceview.Known}, "model": {State: resourceview.Known}},
		}},
	}
	forbiddenSubstrings := []string{
		"DCGM_FI_DEV_FB_USED", "DCGM_FI_DEV_FB_FREE", "DCGM_FI_DEV_GPU_UTIL",
		"npu_chip_info_hbm_", "npu_chip_info_utilization",
		"hcu_usedmemory_bytes", "dcu_usedmemory_bytes", "mx_memory_", "mx_gpu_usage",
	}
	monitor := &fakeInstantQuerier{query: func(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		for _, bad := range forbiddenSubstrings {
			if strings.Contains(req.GetQuery(), bad) {
				t.Fatalf("legacy monitorService issued banned duplicate physical query: %s", req.GetQuery())
			}
		}
		return &pb.InstantResponse{}, nil
	}}
	g := &MetricsGenerator{
		promClient:     promClient,
		resourceSource: &sequenceFacts{available: []resourceview.Snapshot{facts}},
		nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
			Id: deviceID, AliasId: deviceID, Devmem: 81920, Devcore: 100, Count: 10,
			Type: deviceType, NodeName: nodeName, NodeUid: nodeUID, Provider: biz.NvidiaGPUDevice,
		}}}, log.NewStdLogger(io.Discard)),
		podUsecase:     biz.NewPodUseCase(&fakePodRepo{}, log.NewStdLogger(io.Discard)),
		monitorService: monitor,
		interval:       30 * time.Second,
		timeout:        5 * time.Second,
		now:            func() time.Time { return at },
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
	resetRefreshSelfMetrics(t)
	t.Cleanup(func() { deleteTrackedTestCells(g) })

	g.runOnce(context.Background())

	legacyLabels := []string{nodeName, biz.NvidiaGPUDevice, deviceType, deviceID, "", ""}
	for gauge, want := range map[*prometheus.GaugeVec]float64{
		HamiMemoryUsed:     10240,
		HamiMemorySize:     40960,
		HamiMemoryUtil:     25,
		HamiVMemoryScaling: 2,
		HamiCoreUsed:       42,
		HamiCoreUtil:       42,
		HamiCoreUsedAvg:    42,
		HamiCoreUtilAvg:    42,
	} {
		if got := testutil.ToFloat64(gauge.WithLabelValues(legacyLabels...)); got != want {
			t.Fatalf("legacy gauge value = %v, want %v", got, want)
		}
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
				if l.GetName() == "source" && l.GetValue() == "dcgm" {
					gathered[f.GetName()] = m.GetGauge().GetValue()
				}
			}
		}
	}
	if gathered["hami_webui_device_utilization_ratio"] != .42 ||
		gathered["hami_webui_device_memory_used_bytes"] != 10240*bytesPerMiB ||
		gathered["hami_webui_device_memory_physical_bytes"] != 40960*bytesPerMiB {
		t.Fatalf("schema 2 metrics did not match single telemetry flow: %+v", gathered)
	}
}

func TestObservationLookupRequiresMatchingNodeUID(t *testing.T) {
	at := time.Unix(1760000000, 0).UTC()
	state := &viewv1.FieldState{State: resourceview.Known, Source: "npu_exporter", Scope: "device", ObservedAt: timestamppb.New(at)}
	ratio := resource.MustParse("0.55")
	observations := resourceview.DeviceObservations{
		{Provider: biz.AscendGPUDevice, Node: "recreated-node", NodeUID: types.UID("uid-new"), DeviceID: "vdie-0"}: {
			UtilizationRatio: resourceview.Quantity{Value: &ratio, Unit: "ratio", State: state},
		},
	}
	if _, ok := lookupDeviceObservation(observations, biz.AscendGPUDevice, "recreated-node", "uid-old", "vdie-0", "vdie-0"); ok {
		t.Fatal("stale NodeUID incorrectly matched replacement node observation")
	}
	obs, ok := lookupDeviceObservation(observations, biz.AscendGPUDevice, "recreated-node", "uid-new", "vdie-0", "vdie-0")
	if !ok {
		t.Fatal("matching NodeUID did not match observation")
	}
	if got, known := obs.UtilizationRatio.KnownValue(); !known || got != 0.55 {
		t.Fatalf("matched utilization = (%v, %t), want (0.55, true)", got, known)
	}
}

func TestLegacyPhysicalFallbackScopedToUnobservedNativeMetaxAndNumericDCU(t *testing.T) {
	const (
		nodeName = "mixed-legacy-node"
		nodeUID  = "mixed-legacy-uid"
	)
	devices := []*biz.DeviceInfo{
		{Id: "mx-native-0", AliasId: nodeName + "-metax-0", Devmem: 65536, Devcore: 100, Count: 1, Type: "C500", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.MetaxGPUDevice},
		{Id: "legacy-serial", AliasId: nodeName + "-dcu-3", Devmem: 32768, Devcore: 100, Count: 4, Type: "DCU", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.HygonGPUDevice},
		{Id: "DCU-board-3", AliasId: nodeName + "-dcu-4", Devmem: 32768, Devcore: 100, Count: 4, Type: "DCU", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.HygonGPUDevice},
		{Id: "", AliasId: nodeName + "-dcu-5", Devmem: 32768, Devcore: 100, Count: 4, Type: "DCU", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.HygonGPUDevice},
		{Id: "mx-sgpu-0", AliasId: "mx-sgpu-0", Devmem: 65536, Devcore: 100, Count: 4, Type: "C500", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.MetaxGPUDevice},
		{Id: "DCU-board-1", AliasId: "DCU-board-1", Devmem: 32768, Devcore: 100, Count: 4, Type: "DCU", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.HygonGPUDevice},
		{Id: "910C-module-0", AliasId: "910C-module-0", Devmem: 65536, Devcore: 100, Count: 1, Type: "Ascend910C", NodeName: nodeName, NodeUid: nodeUID, Provider: biz.AscendGPUDevice},
	}
	querier := &fakeInstantQuerier{query: func(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		q := req.GetQuery()
		if strings.Contains(q, `device_id=""`) {
			t.Fatalf("unexpected blank DCU device_id query: %s", q)
		}
		switch q {
		case `avg(mx_memory_used{uuid="mx-native-0", type="vram"})`:
			return instantValue(2048 * bytesPerKiB), nil
		case `avg(mx_memory_total{uuid="mx-native-0", type="vram"})`:
			return instantValue(65536 * bytesPerKiB), nil
		case `avg(mx_gpu_usage{uuid="mx-native-0"})`:
			return instantValue(40), nil
		case `avg(dcu_usedmemory_bytes{device_id="legacy-serial"})`:
			return instantValue(4096 * bytesPerMiB), nil
		case `avg(dcu_memorycap_bytes{device_id="legacy-serial"})`:
			return instantValue(32768 * bytesPerMiB), nil
		case `avg(dcu_utilizationrate{device_id="legacy-serial"})`:
			return instantValue(30), nil
		case `avg(dcu_usedmemory_bytes{device_id="DCU-board-3"})`:
			return instantValue(8192 * bytesPerMiB), nil
		case `avg(dcu_memorycap_bytes{device_id="DCU-board-3"})`:
			return instantValue(32768 * bytesPerMiB), nil
		case `avg(dcu_utilizationrate{device_id="DCU-board-3"})`:
			return instantValue(45), nil
		}
		for _, id := range []string{"mx-sgpu-0", "DCU-board-1", "910C-module-0"} {
			if strings.Contains(q, id) && (strings.Contains(q, "memory") || strings.Contains(q, "utilization") || strings.Contains(q, "usage") || strings.Contains(q, "mem_")) {
				if !strings.Contains(q, "power") && !strings.Contains(q, "temp") {
					t.Fatalf("unexpected legacy physical query for single-flow device %s: %s", id, q)
				}
			}
		}
		return &pb.InstantResponse{}, nil
	}}
	g := &MetricsGenerator{
		nodeUsecase:    biz.NewNodeUsecase(&fakeNodeRepo{devices: devices}, log.NewStdLogger(io.Discard)),
		monitorService: querier,
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
	t.Cleanup(func() { deleteTrackedTestCells(g) })

	if err := g.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	metaxLabels := []string{nodeName, biz.MetaxGPUDevice, "C500", "mx-native-0", "", ""}
	assertTrackedGaugeValue(t, g, HamiMemoryUsed, metaxLabels, 2048)
	assertTrackedGaugeValue(t, g, HamiMemorySize, metaxLabels, 65536)
	assertTrackedGaugeValue(t, g, HamiCoreUtil, metaxLabels, 40)

	dcuLabels := []string{nodeName, biz.HygonGPUDevice, "DCU", "legacy-serial", "", ""}
	assertTrackedGaugeValue(t, g, HamiMemoryUsed, dcuLabels, 4096)
	assertTrackedGaugeValue(t, g, HamiMemorySize, dcuLabels, 32768)
	assertTrackedGaugeValue(t, g, HamiCoreUtil, dcuLabels, 30)

	prefixedDCULabels := []string{nodeName, biz.HygonGPUDevice, "DCU", "DCU-board-3", "", ""}
	assertTrackedGaugeValue(t, g, HamiMemoryUsed, prefixedDCULabels, 8192)
	assertTrackedGaugeValue(t, g, HamiMemorySize, prefixedDCULabels, 32768)
	assertTrackedGaugeValue(t, g, HamiCoreUtil, prefixedDCULabels, 45)

	blankDCULabels := []string{nodeName, biz.HygonGPUDevice, "DCU", "", "", ""}
	assertGaugeTracked(t, g, HamiMemoryUsed, blankDCULabels, false)
	assertGaugeTracked(t, g, HamiCoreUtil, blankDCULabels, false)

	for _, id := range []struct{ id, provider, model string }{
		{"mx-sgpu-0", biz.MetaxGPUDevice, "C500"},
		{"DCU-board-1", biz.HygonGPUDevice, "DCU"},
		{"910C-module-0", biz.AscendGPUDevice, "Ascend910C"},
	} {
		labels := []string{nodeName, id.provider, id.model, id.id, "", ""}
		assertGaugeTracked(t, g, HamiMemoryUsed, labels, false)
		assertGaugeTracked(t, g, HamiCoreUtil, labels, false)
	}
}

func TestRunOnceIsolatesSnapshotTimeoutAndDegradesSelfMetric(t *testing.T) {
	resetRefreshSelfMetrics(t)
	const (
		deviceID   = "GPU-timeout-isolation"
		nodeName   = "node-timeout-isolation"
		nodeUID    = "uid-timeout-isolation"
		deviceType = "A100"
	)
	release := make(chan struct{})
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
		http.Error(w, "timed out", http.StatusRequestTimeout)
	}))
	defer func() {
		close(release)
		promServer.Close()
	}()

	promClient, err := prom.NewClient(promServer.URL, 2*time.Second, prom.HTTPConfig{}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	facts := resourceview.Snapshot{
		At:           time.Unix(1760000000, 0).UTC(),
		NodeCoverage: completeCoverage("node", "shared_node_informer", 1),
		Nodes:        []resourceview.Node{{Name: nodeName, UID: nodeUID}},
		Devices: []resourceview.Device{{
			Identity: resourceview.DeviceIdentity{Provider: biz.NvidiaGPUDevice, Node: nodeName, NodeUID: nodeUID, DeviceID: deviceID},
			Kind:     "physical",
			Model:    deviceType,
			States:   map[string]*viewv1.FieldState{"kind": {State: resourceview.Known}},
		}},
	}
	tempQuery := `avg(DCGM_FI_DEV_GPU_TEMP{UUID="` + deviceID + `"})`
	querier := &fakeInstantQuerier{query: func(ctx context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("expected fresh GenerateMetrics context, got %w", err)
		}
		if req.GetQuery() == tempQuery {
			return instantValue(57), nil
		}
		return &pb.InstantResponse{}, nil
	}}
	now := time.Unix(700, 0)
	g := &MetricsGenerator{
		promClient:     promClient,
		resourceSource: &sequenceFacts{available: []resourceview.Snapshot{facts}},
		nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
			Id: deviceID, AliasId: deviceID, Devmem: 81920, Devcore: 100, Count: 1,
			Type: deviceType, NodeName: nodeName, NodeUid: nodeUID, Provider: biz.NvidiaGPUDevice,
		}}}, log.NewStdLogger(io.Discard)),
		podUsecase:     biz.NewPodUseCase(&fakePodRepo{}, log.NewStdLogger(io.Discard)),
		monitorService: querier,
		interval:       time.Second,
		timeout:        40 * time.Millisecond,
		now:            func() time.Time { return now },
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
	t.Cleanup(func() { deleteTrackedTestCells(g) })

	g.runOnce(context.Background())

	labels := []string{nodeName, biz.NvidiaGPUDevice, deviceType, deviceID, "", ""}
	if got := testutil.ToFloat64(HamiVgpuCount.WithLabelValues(labels...)); got != 1 {
		t.Fatalf("allocation gauge = %v, want 1", got)
	}
	if got := testutil.ToFloat64(HamiDeviceTemperature.WithLabelValues(labels...)); got != 57 {
		t.Fatalf("temperature gauge = %v, want 57", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != 0 {
		t.Fatalf("refresh success after snapshot timeout = %v, want 0", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 0 {
		t.Fatalf("last success timestamp after snapshot timeout = %v, want 0", got)
	}
}
