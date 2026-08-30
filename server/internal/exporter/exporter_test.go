package exporter

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/provider/metax"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeInstantQuerier struct {
	responses        []*pb.InstantResponse
	responsesByQuery map[string]*pb.InstantResponse
	errorsByQuery    map[string]error
	queries          []string
}

func (f *fakeInstantQuerier) QueryInstant(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
	f.queries = append(f.queries, req.GetQuery())
	if err, ok := f.errorsByQuery[req.GetQuery()]; ok {
		return nil, err
	}
	if res, ok := f.responsesByQuery[req.GetQuery()]; ok {
		return res, nil
	}
	if len(f.responses) == 0 {
		return &pb.InstantResponse{}, nil
	}
	res := f.responses[0]
	f.responses = f.responses[1:]
	return res, nil
}

type fakeNodeRepo struct {
	devices []*biz.DeviceInfo
}

func (f *fakeNodeRepo) ListAll(context.Context) ([]*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) GetNode(context.Context, string) (*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return f.devices, nil
}

func (f *fakeNodeRepo) FindDeviceByAliasId(string) (*biz.DeviceInfo, error) {
	return nil, nil
}

func TestNvidiaTaskCoreUsedQueryIncludesIdleSamples(t *testing.T) {
	query := nvidiaTaskCoreUsedQuery("GPU-1", "research", "train", "worker")
	want := `avg(avg_over_time(hami_container_device_utilization_ratio{device_uuid="GPU-1", namespace="research", pod="train", container="worker"}[1m]))`
	if query != want {
		t.Fatalf("query mismatch\nwant: %s\n got: %s", want, query)
	}
	if strings.Contains(query, "!=") || strings.Contains(query, "count_over_time") {
		t.Fatalf("query must include valid zero samples: %s", query)
	}
}

func TestTaskCoreUsedDistinguishesMissingFromIdle(t *testing.T) {
	fake := &fakeInstantQuerier{responses: []*pb.InstantResponse{{}}}
	generator := &MetricsGenerator{monitorService: fake}

	_, err := generator.taskCoreUsed(context.Background(), biz.NvidiaGPUDevice, "research", "train", "worker", "pod-uid", "GPU-1", "node-1", 0)
	if !errors.Is(err, errNoMetricData) {
		t.Fatalf("expected errNoMetricData, got %v", err)
	}
}

func TestTaskCoreUsedKeepsLegacyEmptyBehaviorForOtherProviders(t *testing.T) {
	fake := &fakeInstantQuerier{responses: []*pb.InstantResponse{{}}}
	generator := &MetricsGenerator{monitorService: fake}

	value, err := generator.taskCoreUsed(context.Background(), biz.CambriconGPUDevice, "research", "train", "worker", "pod-uid", "MLU-1", "node-1", 0)
	if err != nil || value != 0 {
		t.Fatalf("Cambricon empty result = (%v, %v), want (0, nil)", value, err)
	}
}

func TestDeviceUsageMetricsDistinguishMissingFromIdle(t *testing.T) {
	tests := []struct {
		name  string
		query func(*MetricsGenerator) (float32, error)
	}{
		{
			name: "memory",
			query: func(generator *MetricsGenerator) (float32, error) {
				return generator.deviceMemUsed(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
			},
		},
		{
			name: "compute",
			query: func(generator *MetricsGenerator) (float32, error) {
				return generator.deviceCoreUtil(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" missing", func(t *testing.T) {
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{}}}}
			_, err := tt.query(generator)
			if !errors.Is(err, errNoMetricData) {
				t.Fatalf("expected errNoMetricData, got %v", err)
			}
		})

		t.Run(tt.name+" idle", func(t *testing.T) {
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: 0}}}}}}
			value, err := tt.query(generator)
			if err != nil || value != 0 {
				t.Fatalf("idle sample = (%v, %v), want (0, nil)", value, err)
			}
		})
	}
}

func TestDeviceUsageMetricsRejectNonFiniteSamples(t *testing.T) {
	queries := []struct {
		name string
		read func(*MetricsGenerator) (float32, error)
	}{
		{name: "memory used", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.deviceMemUsed(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
		}},
		{name: "memory total", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.deviceMemTotal(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
		}},
		{name: "compute", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.deviceCoreUtil(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
		}},
	}
	values := []struct {
		name  string
		value float32
	}{
		{name: "NaN", value: float32(math.NaN())},
		{name: "positive infinity", value: float32(math.Inf(1))},
		{name: "negative infinity", value: float32(math.Inf(-1))},
	}

	for _, query := range queries {
		for _, value := range values {
			t.Run(query.name+"/"+value.name, func(t *testing.T) {
				generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: value.value}}}}}}
				_, err := query.read(generator)
				if !errors.Is(err, errNoMetricData) {
					t.Fatalf("expected errNoMetricData for %v, got %v", value.value, err)
				}
			})
		}
	}
}

func TestDeviceMemoryQueriesConvertVendorUnitsToMiB(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		raw      float32
		wantMiB  float32
	}{
		{name: "NVIDIA already reports MiB", provider: biz.NvidiaGPUDevice, raw: 1024, wantMiB: 1024},
		{name: "Cambricon reports bytes", provider: biz.CambriconGPUDevice, raw: 1024 * 1024 * 1024, wantMiB: 1024},
		{name: "Ascend already reports MiB", provider: biz.AscendGPUDevice, raw: 1024, wantMiB: 1024},
		{name: "Hygon reports bytes", provider: biz.HygonGPUDevice, raw: 1024 * 1024 * 1024, wantMiB: 1024},
		{name: "MetaX keeps existing KiB assumption", provider: biz.MetaxGPUDevice, raw: 1024 * 1024, wantMiB: 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: tt.raw}}}}}}
			used, err := generator.deviceMemUsed(context.Background(), tt.provider, "device-1")
			if err != nil || used != tt.wantMiB {
				t.Fatalf("deviceMemUsed() = (%v, %v), want (%v, nil)", used, err, tt.wantMiB)
			}

			generator.monitorService = &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: tt.raw}}}}}
			total, err := generator.deviceMemTotal(context.Background(), tt.provider, "device-1")
			if err != nil || total != tt.wantMiB {
				t.Fatalf("deviceMemTotal() = (%v, %v), want (%v, nil)", total, err, tt.wantMiB)
			}
		})
	}
}

func TestGenerateDeviceMetricsSeparatesPhysicalAndSchedulableMemory(t *testing.T) {
	const (
		deviceID      = "GPU-physical-contract"
		nodeName      = "node-physical-contract"
		deviceType    = "A100"
		schedulableMB = 81920
		physicalMB    = 40960
		usedMB        = 10240
	)
	usedQuery := `avg(DCGM_FI_DEV_FB_USED{UUID="GPU-physical-contract"})`
	totalQuery := `avg(DCGM_FI_DEV_FB_FREE{UUID="GPU-physical-contract"})+avg(DCGM_FI_DEV_FB_USED{UUID="GPU-physical-contract"})`
	generator := newDeviceMetricsTestGenerator(
		&biz.DeviceInfo{
			Id:       deviceID,
			Devmem:   schedulableMB,
			Devcore:  100,
			Count:    1,
			Type:     deviceType,
			NodeName: nodeName,
			Provider: biz.NvidiaGPUDevice,
		},
		map[string]*pb.InstantResponse{
			usedQuery:  instantValue(usedMB),
			totalQuery: instantValue(physicalMB),
		},
	)

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{nodeName, biz.NvidiaGPUDevice, deviceType, deviceID, "", ""}
	assertTrackedGaugeValue(t, generator, HamiVmemorySize, labels, schedulableMB)
	assertTrackedGaugeValue(t, generator, HamiMemorySize, labels, physicalMB)
	assertTrackedGaugeValue(t, generator, HamiMemoryUsed, labels, usedMB)
	assertTrackedGaugeValue(t, generator, HamiMemoryUtil, labels, 25)
	assertTrackedGaugeValue(t, generator, HamiVMemoryScaling, labels, 2)
}

func TestGenerateDeviceMetricsOmitsPhysicalUtilizationWithoutMatchingCoverage(t *testing.T) {
	tests := []struct {
		name              string
		deviceID          string
		responses         map[string]*pb.InstantResponse
		wantPhysicalSize  bool
		wantPhysicalUsed  bool
		wantPhysicalUtil  bool
		wantMemoryScaling bool
	}{
		{
			name:     "used without total",
			deviceID: "GPU-used-only",
			responses: map[string]*pb.InstantResponse{
				`avg(DCGM_FI_DEV_FB_USED{UUID="GPU-used-only"})`: instantValue(512),
			},
			wantPhysicalUsed: true,
		},
		{
			name:     "total without used",
			deviceID: "GPU-total-only",
			responses: map[string]*pb.InstantResponse{
				`avg(DCGM_FI_DEV_FB_FREE{UUID="GPU-total-only"})+avg(DCGM_FI_DEV_FB_USED{UUID="GPU-total-only"})`: instantValue(40960),
			},
			wantPhysicalSize:  true,
			wantMemoryScaling: true,
		},
		{
			name:     "zero total is not capacity",
			deviceID: "GPU-zero-total",
			responses: map[string]*pb.InstantResponse{
				`avg(DCGM_FI_DEV_FB_FREE{UUID="GPU-zero-total"})+avg(DCGM_FI_DEV_FB_USED{UUID="GPU-zero-total"})`: instantValue(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := newDeviceMetricsTestGenerator(
				&biz.DeviceInfo{
					Id:       tt.deviceID,
					Devmem:   81920,
					Devcore:  100,
					Count:    1,
					Type:     "A100",
					NodeName: "node-coverage-" + tt.deviceID,
					Provider: biz.NvidiaGPUDevice,
				},
				tt.responses,
			)

			if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
				t.Fatalf("GenerateDeviceMetrics() error = %v", err)
			}
			labels := []string{"node-coverage-" + tt.deviceID, biz.NvidiaGPUDevice, "A100", tt.deviceID, "", ""}
			assertGaugeTracked(t, generator, HamiMemorySize, labels, tt.wantPhysicalSize)
			assertGaugeTracked(t, generator, HamiMemoryUsed, labels, tt.wantPhysicalUsed)
			assertGaugeTracked(t, generator, HamiMemoryUtil, labels, tt.wantPhysicalUtil)
			assertGaugeTracked(t, generator, HamiVMemoryScaling, labels, tt.wantMemoryScaling)
		})
	}
}

func TestCommitCyclePrunesPhysicalMemorySeriesWhenTelemetryDisappears(t *testing.T) {
	const (
		deviceID   = "GPU-prune-physical-contract"
		nodeName   = "node-prune-physical-contract"
		deviceType = "A100"
	)
	usedQuery := `avg(DCGM_FI_DEV_FB_USED{UUID="GPU-prune-physical-contract"})`
	totalQuery := `avg(DCGM_FI_DEV_FB_FREE{UUID="GPU-prune-physical-contract"})+avg(DCGM_FI_DEV_FB_USED{UUID="GPU-prune-physical-contract"})`
	generator := newDeviceMetricsTestGenerator(
		&biz.DeviceInfo{
			Id:       deviceID,
			Devmem:   81920,
			Devcore:  100,
			Count:    1,
			Type:     deviceType,
			NodeName: nodeName,
			Provider: biz.NvidiaGPUDevice,
		},
		map[string]*pb.InstantResponse{
			usedQuery:  instantValue(10240),
			totalQuery: instantValue(40960),
		},
	)
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("first GenerateDeviceMetrics() error = %v", err)
	}
	generator.commitCycle()
	labels := map[string]string{
		"node":           nodeName,
		"provider":       biz.NvidiaGPUDevice,
		"device_type":    deviceType,
		"device_uuid":    deviceID,
		"driver_version": "",
		"device_no":      "",
	}
	assertGaugeLabelsPresent(t, HamiMemorySize, labels, true)
	assertGaugeLabelsPresent(t, HamiMemoryUsed, labels, true)
	assertGaugeLabelsPresent(t, HamiMemoryUtil, labels, true)
	assertGaugeLabelsPresent(t, HamiVMemoryScaling, labels, true)

	// A successful cycle with no vendor memory telemetry must retain the
	// schedulable inventory while pruning physical series from the registry.
	generator.monitorService = &fakeInstantQuerier{responsesByQuery: map[string]*pb.InstantResponse{}}
	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("second GenerateDeviceMetrics() error = %v", err)
	}
	generator.commitCycle()

	assertGaugeLabelsPresent(t, HamiVmemorySize, labels, true)
	assertGaugeLabelsPresent(t, HamiMemorySize, labels, false)
	assertGaugeLabelsPresent(t, HamiMemoryUsed, labels, false)
	assertGaugeLabelsPresent(t, HamiMemoryUtil, labels, false)
	assertGaugeLabelsPresent(t, HamiVMemoryScaling, labels, false)
}

func newDeviceMetricsTestGenerator(device *biz.DeviceInfo, responses map[string]*pb.InstantResponse) *MetricsGenerator {
	return &MetricsGenerator{
		nodeUsecase: biz.NewNodeUsecase(
			&fakeNodeRepo{devices: []*biz.DeviceInfo{device}},
			log.NewStdLogger(io.Discard),
		),
		monitorService: &fakeInstantQuerier{responsesByQuery: responses},
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
}

func instantValue(value float32) *pb.InstantResponse {
	return &pb.InstantResponse{Data: []*pb.Sample{{Value: value}}}
}

func assertTrackedGaugeValue(
	t *testing.T,
	generator *MetricsGenerator,
	gauge *prometheus.GaugeVec,
	labels []string,
	want float64,
) {
	t.Helper()
	assertGaugeTracked(t, generator, gauge, labels, true)
	if got := testutil.ToFloat64(gauge.WithLabelValues(labels...)); got != want {
		t.Fatalf("gauge value = %v, want %v", got, want)
	}
}

func assertGaugeTracked(
	t *testing.T,
	generator *MetricsGenerator,
	gauge *prometheus.GaugeVec,
	labels []string,
	want bool,
) {
	t.Helper()
	key := cellKey{gauge: gauge, joined: strings.Join(labels, labelSep)}
	_, got := generator.current[key]
	if got != want {
		t.Fatalf("gauge tracked = %t, want %t", got, want)
	}
}

func assertGaugeLabelsPresent(
	t *testing.T,
	gauge *prometheus.GaugeVec,
	wantLabels map[string]string,
	want bool,
) {
	t.Helper()
	registry := prometheus.NewRegistry()
	registry.MustRegister(gauge)
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	found := false
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			matches := true
			for _, pair := range metric.GetLabel() {
				value, ok := wantLabels[pair.GetName()]
				if !ok || value != pair.GetValue() {
					matches = false
					break
				}
			}
			if matches && len(metric.GetLabel()) == len(wantLabels) {
				found = true
				break
			}
		}
	}
	if found != want {
		t.Fatalf("gauge label set present = %t, want %t; labels = %v", found, want, wantLabels)
	}
}

func deleteTrackedTestCells(generator *MetricsGenerator) {
	for _, cells := range []map[cellKey]cell{generator.current, generator.prev} {
		for _, tracked := range cells {
			tracked.gauge.DeleteLabelValues(tracked.labels...)
		}
	}
}

func TestNvidiaContainerCoreMetrics(t *testing.T) {
	tests := []struct {
		name      string
		raw       float32
		allocated int32
		wantUsed  float64
		wantUtil  float64
	}{
		{name: "idle", raw: 0, allocated: 50, wantUsed: 0, wantUtil: 0},
		{name: "intermittent activity", raw: 70.93, allocated: 50, wantUsed: 35.47, wantUtil: 70.9},
		{name: "force policy full activity", raw: 100, allocated: 20, wantUsed: 20, wantUtil: 100},
		{name: "source activity is bounded", raw: 125, allocated: 50, wantUsed: 50, wantUtil: 100},
		{name: "single-card estimate is bounded", raw: 100, allocated: 200, wantUsed: 100, wantUtil: 100},
		{name: "negative source is bounded", raw: -5, allocated: 50, wantUsed: 0, wantUtil: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: tt.raw}}}}}
			generator := &MetricsGenerator{monitorService: fake}

			used, util, err := generator.containerCoreMetrics(context.Background(), biz.NvidiaGPUDevice, "research", "train", "worker", "pod-uid", "GPU-1", "node-1", 0, tt.allocated)
			if err != nil {
				t.Fatalf("containerCoreMetrics() error = %v", err)
			}
			if math.Abs(used-tt.wantUsed) > 0.001 || math.Abs(util-tt.wantUtil) > 0.001 {
				t.Fatalf("containerCoreMetrics() = (%v, %v), want (%v, %v)", used, util, tt.wantUsed, tt.wantUtil)
			}
			if len(fake.queries) != 1 {
				t.Fatalf("NVIDIA metrics made %d queries, want 1 (no device-level fallback)", len(fake.queries))
			}
			if strings.Contains(fake.queries[0], "DCGM_FI_DEV_GPU_UTIL") {
				t.Fatalf("NVIDIA task metrics must not use card-level DCGM data: %s", fake.queries[0])
			}
		})
	}
}

func TestNvidiaContainerCoreMetricsRejectsNonFiniteValues(t *testing.T) {
	tests := []struct {
		name string
		raw  float32
	}{
		{name: "NaN", raw: float32(math.NaN())},
		{name: "positive infinity", raw: float32(math.Inf(1))},
		{name: "negative infinity", raw: float32(math.Inf(-1))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: tt.raw}}}}}
			generator := &MetricsGenerator{monitorService: fake}

			_, _, err := generator.containerCoreMetrics(context.Background(), biz.NvidiaGPUDevice, "research", "train", "worker", "pod-uid", "GPU-1", "node-1", 0, 50)
			if !errors.Is(err, errNoMetricData) {
				t.Fatalf("expected errNoMetricData, got %v", err)
			}
		})
	}
}

func TestContainerCoreMetricsRejectsZeroAllocation(t *testing.T) {
	fake := &fakeInstantQuerier{}
	generator := &MetricsGenerator{monitorService: fake}

	_, _, err := generator.containerCoreMetrics(context.Background(), biz.NvidiaGPUDevice, "research", "train", "worker", "pod-uid", "GPU-1", "node-1", 0, 0)
	if !errors.Is(err, errInvalidCoreCapacity) {
		t.Fatalf("expected errInvalidCoreCapacity, got %v", err)
	}
	if len(fake.queries) != 0 {
		t.Fatalf("invalid allocation should not query Prometheus, got %d queries", len(fake.queries))
	}
}

func TestContainerCoreMetricsKeepsLegacyProviderConversions(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		taskValue float32
		cardValue float32
		allocated int32
		wantUsed  float64
		wantUtil  float64
	}{
		{name: "Cambricon task metric", provider: biz.CambriconGPUDevice, taskValue: 50, cardValue: 50, allocated: 40, wantUsed: 20, wantUtil: 50},
		{name: "Cambricon legacy card fallback", provider: biz.CambriconGPUDevice, taskValue: 50, cardValue: 100, allocated: 40, wantUsed: 40, wantUtil: 100},
		{name: "Hygon task metric", provider: biz.HygonGPUDevice, taskValue: 25, cardValue: 50, allocated: 50, wantUsed: 25, wantUtil: 50},
		{name: "Metax sGPU task metric", provider: metax.MetaxSGPUDevice, taskValue: 25, cardValue: 50, allocated: 50, wantUsed: 25, wantUtil: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeInstantQuerier{responses: []*pb.InstantResponse{
				{Data: []*pb.Sample{{Value: tt.taskValue}}},
				{Data: []*pb.Sample{{Value: tt.cardValue}}},
			}}
			generator := &MetricsGenerator{monitorService: fake}

			used, util, err := generator.containerCoreMetrics(context.Background(), tt.provider, "research", "train", "worker", "pod-uid", "device-1", "node-1", 0, tt.allocated)
			if err != nil {
				t.Fatalf("containerCoreMetrics() error = %v", err)
			}
			if used != tt.wantUsed || util != tt.wantUtil {
				t.Fatalf("containerCoreMetrics() = (%v, %v), want (%v, %v)", used, util, tt.wantUsed, tt.wantUtil)
			}
			if len(fake.queries) != 2 {
				t.Fatalf("legacy provider made %d queries, want 2", len(fake.queries))
			}
		})
	}
}
