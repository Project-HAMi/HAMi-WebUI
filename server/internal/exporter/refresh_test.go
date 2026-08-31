package exporter

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestDropCurrentCycleDoesNotLeakStagedValues(t *testing.T) {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_refresh_transaction_value",
		Help: "Test-only gauge for refresh transaction semantics.",
	}, []string{"id"})
	generator := &MetricsGenerator{log: log.NewHelper(log.NewStdLogger(io.Discard))}
	t.Cleanup(func() {
		gauge.DeleteLabelValues("existing")
		gauge.DeleteLabelValues("new")
	})

	generator.beginCycle()
	generator.set(gauge, 42, "existing")
	generator.commitCycle()
	if got := testutil.ToFloat64(gauge.WithLabelValues("existing")); got != 42 {
		t.Fatalf("committed value = %v, want 42", got)
	}

	generator.beginCycle()
	generator.set(gauge, 84, "existing")
	generator.set(gauge, 7, "new")
	generator.dropCurrentCycle()

	if got := testutil.ToFloat64(gauge.WithLabelValues("existing")); got != 42 {
		t.Fatalf("value after fatal drop = %v, want previous value 42", got)
	}
	assertGaugeLabelsPresent(t, gauge, map[string]string{"id": "new"}, false)
}

func TestMetricsRefreshTwoCycleTelemetrySemantics(t *testing.T) {
	tests := []struct {
		name               string
		secondResponse     *pb.InstantResponse
		secondErr          error
		wantTemperature    bool
		wantTemperatureVal float64
		wantSuccess        float64
		wantLastSuccess    float64
	}{
		{
			name:            "empty omits telemetry without degrading",
			secondResponse:  &pb.InstantResponse{},
			wantSuccess:     1,
			wantLastSuccess: 200,
		},
		{
			name:               "real zero remains present",
			secondResponse:     instantValue(0),
			wantTemperature:    true,
			wantTemperatureVal: 0,
			wantSuccess:        1,
			wantLastSuccess:    200,
		},
		{
			name:            "upstream request timeout degrades while cycle context is valid",
			secondErr:       context.DeadlineExceeded,
			wantSuccess:     0,
			wantLastSuccess: 100,
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRefreshSelfMetrics(t)
			deviceID := "GPU-refresh-" + string(rune('a'+index))
			nodeName := "node-refresh-" + string(rune('a'+index))
			device := &biz.DeviceInfo{
				Id: deviceID, Devmem: 40960, Devcore: 100, Count: 1,
				Type: "A100", NodeName: nodeName, Provider: biz.NvidiaGPUDevice,
			}
			response := instantValue(42)
			var queryErr error
			querier := &fakeInstantQuerier{query: func(context.Context, *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
				if queryErr != nil {
					return nil, queryErr
				}
				return response, nil
			}}
			now := time.Unix(100, 0)
			generator := newRefreshCycleTestGenerator(&fakeNodeRepo{devices: []*biz.DeviceInfo{device}}, &fakePodRepo{}, querier, func() time.Time { return now })
			t.Cleanup(func() { deleteTrackedTestCells(generator) })

			generator.runOnce(context.Background())
			labels := map[string]string{
				"node": nodeName, "provider": biz.NvidiaGPUDevice, "device_type": "A100",
				"device_uuid": deviceID, "driver_version": "", "device_no": "",
			}
			assertGaugeLabelsPresent(t, HamiDeviceTemperature, labels, true)
			if got := testutil.ToFloat64(HamiDeviceTemperature.WithLabelValues(nodeName, biz.NvidiaGPUDevice, "A100", deviceID, "", "")); got != 42 {
				t.Fatalf("first temperature = %v, want 42", got)
			}

			response = tt.secondResponse
			queryErr = tt.secondErr
			now = time.Unix(200, 0)
			generator.runOnce(context.Background())

			assertGaugeLabelsPresent(t, HamiDeviceTemperature, labels, tt.wantTemperature)
			if tt.wantTemperature {
				if got := testutil.ToFloat64(HamiDeviceTemperature.WithLabelValues(nodeName, biz.NvidiaGPUDevice, "A100", deviceID, "", "")); got != tt.wantTemperatureVal {
					t.Fatalf("second temperature = %v, want %v", got, tt.wantTemperatureVal)
				}
			}
			assertGaugeLabelsPresent(t, HamiVgpuCount, labels, true)
			if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != tt.wantSuccess {
				t.Fatalf("refresh success = %v, want %v", got, tt.wantSuccess)
			}
			if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != tt.wantLastSuccess {
				t.Fatalf("last success timestamp = %v, want %v", got, tt.wantLastSuccess)
			}
		})
	}
}

func TestMetricsRefreshCommitsSuccessfulTelemetryWhenAnotherQueryFails(t *testing.T) {
	resetRefreshSelfMetrics(t)
	const (
		deviceID = "GPU-refresh-mixed"
		nodeName = "node-refresh-mixed"
	)
	device := &biz.DeviceInfo{
		Id: deviceID, Devmem: 40960, Devcore: 100, Count: 1,
		Type: "A100", NodeName: nodeName, Provider: biz.NvidiaGPUDevice,
	}
	value := float32(42)
	failTemperature := false
	temperatureQuery := `avg(DCGM_FI_DEV_GPU_TEMP{UUID="GPU-refresh-mixed"})`
	querier := &fakeInstantQuerier{query: func(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		if failTemperature && req.GetQuery() == temperatureQuery {
			return nil, errors.New("temperature query failed")
		}
		return instantValue(value), nil
	}}
	now := time.Unix(250, 0)
	generator := newRefreshCycleTestGenerator(&fakeNodeRepo{devices: []*biz.DeviceInfo{device}}, &fakePodRepo{}, querier, func() time.Time { return now })
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	generator.runOnce(context.Background())
	labels := map[string]string{
		"node": nodeName, "provider": biz.NvidiaGPUDevice, "device_type": "A100",
		"device_uuid": deviceID, "driver_version": "", "device_no": "",
	}
	assertGaugeLabelsPresent(t, HamiDeviceTemperature, labels, true)
	assertGaugeLabelsPresent(t, HamiDevicePower, labels, true)

	failTemperature = true
	value = 84
	now = time.Unix(260, 0)
	generator.runOnce(context.Background())

	assertGaugeLabelsPresent(t, HamiDeviceTemperature, labels, false)
	assertGaugeLabelsPresent(t, HamiDevicePower, labels, true)
	if got := testutil.ToFloat64(HamiDevicePower.WithLabelValues(nodeName, biz.NvidiaGPUDevice, "A100", deviceID, "", "")); got != 84 {
		t.Fatalf("successful telemetry in degraded cycle = %v, want 84", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != 0 {
		t.Fatalf("refresh success after mixed cycle = %v, want 0", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 250 {
		t.Fatalf("last success timestamp after mixed cycle = %v, want 250", got)
	}
}

func TestMetricsRefreshDoesNotDegradeForUnsupportedHygonTelemetry(t *testing.T) {
	resetRefreshSelfMetrics(t)
	device := &biz.DeviceInfo{
		Id: "DCU-refresh-unsupported", Devmem: 16384, Devcore: 100, Count: 1,
		Type: "DCU", NodeName: "node-refresh-hygon", Provider: biz.HygonGPUDevice,
	}
	querier := &fakeInstantQuerier{query: func(context.Context, *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		return instantValue(42), nil
	}}
	now := time.Unix(275, 0)
	generator := newRefreshCycleTestGenerator(&fakeNodeRepo{devices: []*biz.DeviceInfo{device}}, &fakePodRepo{}, querier, func() time.Time { return now })
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	generator.runOnce(context.Background())

	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != 1 {
		t.Fatalf("refresh success with unsupported Hygon telemetry = %v, want 1", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 275 {
		t.Fatalf("last success timestamp = %v, want 275", got)
	}
	labels := map[string]string{
		"node": "node-refresh-hygon", "provider": biz.HygonGPUDevice, "device_type": "DCU",
		"device_uuid": device.Id, "driver_version": "", "device_no": "dcu-",
	}
	assertGaugeLabelsPresent(t, HamiDeviceMemoryTemperature, labels, false)
	assertGaugeLabelsPresent(t, HamiDeviceFanSpeedP, labels, false)
	assertGaugeLabelsPresent(t, HamiDeviceLastXIDErrorCode, labels, false)
}

func TestMetricsRefreshFatalAuthorityFailureDropsEntireStage(t *testing.T) {
	resetRefreshSelfMetrics(t)
	oldDevice := &biz.DeviceInfo{
		Id: "GPU-refresh-fatal-old", Devmem: 40960, Devcore: 100, Count: 1,
		Type: "A100", NodeName: "node-refresh-fatal", Provider: biz.NvidiaGPUDevice,
	}
	newDevice := &biz.DeviceInfo{
		Id: "GPU-refresh-fatal-new", Devmem: 40960, Devcore: 100, Count: 1,
		Type: "A100", NodeName: "node-refresh-fatal", Provider: biz.NvidiaGPUDevice,
	}
	devices := []*biz.DeviceInfo{oldDevice}
	listCalls := 0
	authorityErr := errors.New("device inventory unavailable")
	nodeRepo := &fakeNodeRepo{listAllDevicesFunc: func(context.Context) ([]*biz.DeviceInfo, error) {
		listCalls++
		if listCalls == 4 {
			return nil, authorityErr
		}
		return devices, nil
	}}
	value := float32(42)
	querier := &fakeInstantQuerier{query: func(context.Context, *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		return instantValue(value), nil
	}}
	now := time.Unix(300, 0)
	generator := newRefreshCycleTestGenerator(nodeRepo, &fakePodRepo{}, querier, func() time.Time { return now })
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	generator.runOnce(context.Background())
	oldLabels := map[string]string{
		"node": "node-refresh-fatal", "provider": biz.NvidiaGPUDevice, "device_type": "A100",
		"device_uuid": oldDevice.Id, "driver_version": "", "device_no": "",
	}
	assertGaugeLabelsPresent(t, HamiDeviceTemperature, oldLabels, true)

	devices = []*biz.DeviceInfo{oldDevice, newDevice}
	value = 84
	now = time.Unix(400, 0)
	generator.runOnce(context.Background())

	if got := testutil.ToFloat64(HamiDeviceTemperature.WithLabelValues("node-refresh-fatal", biz.NvidiaGPUDevice, "A100", oldDevice.Id, "", "")); got != 42 {
		t.Fatalf("old value after fatal cycle = %v, want 42", got)
	}
	newLabels := map[string]string{
		"node": "node-refresh-fatal", "provider": biz.NvidiaGPUDevice, "device_type": "A100",
		"device_uuid": newDevice.Id, "driver_version": "", "device_no": "",
	}
	assertGaugeLabelsPresent(t, HamiDeviceTemperature, newLabels, false)
	assertGaugeLabelsPresent(t, HamiVgpuCount, newLabels, false)
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != 0 {
		t.Fatalf("refresh success after fatal cycle = %v, want 0", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 300 {
		t.Fatalf("last success timestamp after fatal cycle = %v, want 300", got)
	}
}

func TestMetricsRefreshUsesCycleContextForNestedQueries(t *testing.T) {
	resetRefreshSelfMetrics(t)
	device := &biz.DeviceInfo{
		Id: "GPU-refresh-context", Devmem: 40960, Devcore: 100, Count: 1,
		Type: "A100", NodeName: "node-refresh-context", Provider: biz.NvidiaGPUDevice,
	}
	queryObservedCancellation := false
	ctx, cancel := context.WithCancel(context.Background())
	querier := &fakeInstantQuerier{query: func(ctx context.Context, _ *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
		cancel()
		<-ctx.Done()
		queryObservedCancellation = true
		return nil, ctx.Err()
	}}
	now := time.Unix(500, 0)
	generator := newRefreshCycleTestGenerator(&fakeNodeRepo{devices: []*biz.DeviceInfo{device}}, &fakePodRepo{}, querier, func() time.Time { return now })
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	generator.runOnce(ctx)

	if !queryObservedCancellation {
		t.Fatal("nested query did not observe cycle cancellation")
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshSuccess); got != 0 {
		t.Fatalf("refresh success after canceled cycle = %v, want 0", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 0 {
		t.Fatalf("last success timestamp after canceled first cycle = %v, want 0", got)
	}
}

func TestQueryDeviceAdditionalTreatsEmptyVectorAsExpectedOmission(t *testing.T) {
	generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{}}}}
	_, err := generator.queryDeviceAdditional(context.Background(), biz.NvidiaGPUDevice, "GPU-empty-additional")
	if !errors.Is(err, errNoMetricData) {
		t.Fatalf("queryDeviceAdditional() error = %v, want errNoMetricData", err)
	}
}

func TestMetricsRefreshDurationAndTimestampAreRecordedAfterCommit(t *testing.T) {
	resetRefreshSelfMetrics(t)
	times := []time.Time{time.Unix(600, 0), time.Unix(602, 0)}
	clockCalls := 0
	commitObserved := false
	var generator *MetricsGenerator
	clock := func() time.Time {
		if clockCalls == 1 {
			generator.cellMu.Lock()
			commitObserved = generator.current == nil && generator.prev != nil
			generator.cellMu.Unlock()
		}
		value := times[clockCalls]
		clockCalls++
		return value
	}
	generator = newRefreshCycleTestGenerator(&fakeNodeRepo{}, &fakePodRepo{}, &fakeInstantQuerier{}, clock)

	generator.runOnce(context.Background())

	if clockCalls != 2 {
		t.Fatalf("clock calls = %d, want 2", clockCalls)
	}
	if !commitObserved {
		t.Fatal("completion time was read before the staged snapshot was committed")
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshDurationSeconds); got != 2 {
		t.Fatalf("refresh duration = %v, want 2", got)
	}
	if got := testutil.ToFloat64(HamiWebUIMetricsRefreshLastSuccessTimestampSeconds); got != 602 {
		t.Fatalf("last success timestamp = %v, want 602", got)
	}
}

func TestRefreshSelfMetricDescriptors(t *testing.T) {
	resetRefreshSelfMetrics(t)
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		HamiWebUIMetricsRefreshSuccess,
		HamiWebUIMetricsRefreshDurationSeconds,
		HamiWebUIMetricsRefreshLastSuccessTimestampSeconds,
	)
	want := `
# HELP hami_webui_metrics_refresh_duration_seconds Duration in seconds of the most recent completed background refresh of the cached HAMi metrics snapshot.
# TYPE hami_webui_metrics_refresh_duration_seconds gauge
hami_webui_metrics_refresh_duration_seconds 0
# HELP hami_webui_metrics_refresh_last_success_timestamp_seconds Unix timestamp in seconds of the most recent fully successful commit of the cached HAMi metrics snapshot.
# TYPE hami_webui_metrics_refresh_last_success_timestamp_seconds gauge
hami_webui_metrics_refresh_last_success_timestamp_seconds 0
# HELP hami_webui_metrics_refresh_success Whether the most recent completed background refresh of the cached HAMi metrics snapshot was fully successful (1) or degraded/failed (0).
# TYPE hami_webui_metrics_refresh_success gauge
hami_webui_metrics_refresh_success 0
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(want)); err != nil {
		t.Fatalf("refresh self metrics differ: %v", err)
	}
}

func newRefreshCycleTestGenerator(
	nodeRepo *fakeNodeRepo,
	podRepo *fakePodRepo,
	querier *fakeInstantQuerier,
	now func() time.Time,
) *MetricsGenerator {
	logger := log.NewStdLogger(io.Discard)
	return &MetricsGenerator{
		nodeUsecase:    biz.NewNodeUsecase(nodeRepo, logger),
		podUsecase:     biz.NewPodUseCase(podRepo, logger),
		monitorService: querier,
		timeout:        time.Minute,
		log:            log.NewHelper(logger),
		now:            now,
	}
}

func resetRefreshSelfMetrics(t *testing.T) {
	t.Helper()
	HamiWebUIMetricsRefreshSuccess.Set(0)
	HamiWebUIMetricsRefreshDurationSeconds.Set(0)
	HamiWebUIMetricsRefreshLastSuccessTimestampSeconds.Set(0)
	t.Cleanup(func() {
		HamiWebUIMetricsRefreshSuccess.Set(0)
		HamiWebUIMetricsRefreshDurationSeconds.Set(0)
		HamiWebUIMetricsRefreshLastSuccessTimestampSeconds.Set(0)
	})
}
