package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
)

// ---- Fake repos ----

type fakePodRepo struct {
	containers []*biz.Container
}

func (f *fakePodRepo) ListAll(_ context.Context) ([]*biz.Container, error) {
	return f.containers, nil
}

func (f *fakePodRepo) FindOne(_ context.Context, _, _ string) (*biz.Container, error) {
	if len(f.containers) > 0 {
		return f.containers[0], nil
	}
	return nil, fmt.Errorf("not found")
}

type fakeNodeRepo struct {
	devices []*biz.DeviceInfo
}

func (f *fakeNodeRepo) ListAll(_ context.Context) ([]*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) GetNode(_ context.Context, _ string) (*biz.Node, error) {
	return nil, nil
}

func (f *fakeNodeRepo) ListAllDevices(_ context.Context) ([]*biz.DeviceInfo, error) {
	return f.devices, nil
}

func (f *fakeNodeRepo) FindDeviceByAliasId(_ string) (*biz.DeviceInfo, error) {
	return nil, nil
}

// ---- Mock Prometheus server ----

type mockPromHandler struct {
	responses map[string]string
}

func (h *mockPromHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/query" {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query().Get("query")
	if r.Method == "POST" {
		query = r.PostFormValue("query")
	}
	if body, ok := h.responses[query]; ok {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	} else {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}
}

func buildPromVectorResponse(samples []model.Sample) string {
	var result []map[string]interface{}
	for _, s := range samples {
		metric := make(map[string]string)
		for k, v := range s.Metric {
			metric[string(k)] = string(v)
		}
		result = append(result, map[string]interface{}{
			"metric": metric,
			"value":  []interface{}{float64(s.Timestamp.Unix()), s.Value.String()},
		})
	}
	wrapper := map[string]interface{}{
		"status": "success",
		"data": map[string]interface{}{
			"resultType": "vector",
			"result":     result,
		},
	}
	body, _ := json.Marshal(wrapper)
	return string(body)
}

// ---- Test helpers ----

func newTestMetricsGenerator(t *testing.T, promURL string, containers []*biz.Container, devices []*biz.DeviceInfo) *MetricsGenerator {
	t.Helper()
	promClient, err := prom.NewClient(promURL, time.Second*5, "")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	podUC := biz.NewPodUseCase(&fakePodRepo{containers: containers}, log.DefaultLogger)
	nodeUC := biz.NewNodeUsecase(&fakeNodeRepo{devices: devices}, log.DefaultLogger)
	monitorSvc := service.NewMonitorService(promClient, nodeUC, podUC)
	gen := NewMetricsGenerator(
		&conf.Bootstrap{},
		promClient,
		nodeUC,
		podUC,
		monitorSvc,
		log.DefaultLogger,
	)
	return gen
}

func readMetricAnyLabels(metricName string, partialLabels map[string]string) float64 {
	registry := prometheus.DefaultRegisterer.(*prometheus.Registry)
	families, err := registry.Gather()
	if err != nil {
		return -1
	}
	for _, f := range families {
		if f.GetName() != metricName {
			continue
		}
		for _, m := range f.GetMetric() {
			ml := m.GetLabel()
			match := true
			for _, l := range ml {
				if v, ok := partialLabels[l.GetName()]; ok && v != l.GetValue() {
					match = false
					break
				}
			}
			if match {
				return m.GetGauge().GetValue()
			}
		}
	}
	return -1
}

func resetTestMetrics() {
	// Reset all gauge vectors between tests
	for _, m := range []*prometheus.GaugeVec{
		HamiContainerVgpuAllocated, HamiContainerVmemoryAllocated, HamiContainerVcoreAllocated,
		HamiContainerMemoryUsed, HamiContainerMemoryUtil,
		HamiContainerCoreUsed, HamiContainerCoreUtil,
	} {
		m.Reset()
	}
}

func approxEqual(a, b, epsilon float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// ---- Ascend proportional split tests ----

func TestAscend910B_Proportional_SingleContainer_Success(t *testing.T) {
	devices := []*biz.DeviceInfo{
		{
			Id: "npu-uuid-1", AliasId: "npu-uuid-1",
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "npu-uuid-1", Type: "Ascend910B", Usedmem: 11264, Usedcores: 25},
			},
		},
	}

	now := model.Now()
	totalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", "npu-uuid-1")
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", "npu-uuid-1")
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", "npu-uuid-1")

	mockResponses := map[string]string{
		totalQ: buildPromVectorResponse([]model.Sample{
			{Value: 65536, Timestamp: now},
		}),
		coreUtilQ: buildPromVectorResponse([]model.Sample{
			{Value: 30, Timestamp: now},
		}),
		memUsedQ: buildPromVectorResponse([]model.Sample{
			{Value: 14305.1, Timestamp: now},
		}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()
	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	// ratio=11264/11264=1.0, cardUtil=30
	// core_used = 30 × 1.0 = 30, core_util = core_used = 30
	wantUsed := float64(30)
	wantUtil := float64(30)
	if got := readMetricAnyLabels("hami_container_core_used", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); !approxEqual(wantUsed, got, 0.1) {
		t.Errorf("hami_container_core_used: want %v, got %v", wantUsed, got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", map[string]string{
		"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1",
	}); !approxEqual(wantUtil, got, 0.1) {
		t.Errorf("hami_container_core_util: want ~%v (=core_used), got %v", wantUtil, got)
	}
}

func TestAscend910B_Proportional_MultiContainer_CoreMemory(t *testing.T) {
	deviceUUID := "npu-multi-1"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr-a", PodName: "pod-a", Namespace: "ns-1", PodUID: "pod-uid-a", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
		{
			Name: "ctr-b", PodName: "pod-b", Namespace: "ns-1", PodUID: "pod-uid-b", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 32768, Usedcores: 50},
			},
		},
	}

	now := model.Now()
	coreUtilQ := fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	memUsedQ := fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	memTotalQ := fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)

	mockResponses := map[string]string{
		coreUtilQ: buildPromVectorResponse([]model.Sample{{Value: 90, Timestamp: now}}),
		memUsedQ:  buildPromVectorResponse([]model.Sample{{Value: 28610.2, Timestamp: now}}),
		memTotalQ: buildPromVectorResponse([]model.Sample{{Value: 65536, Timestamp: now}}),
	}

	mux := http.NewServeMux()
	mux.Handle("/api/v1/query", &mockPromHandler{responses: mockResponses})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labelsA := map[string]string{"pod_name": "pod-a", "container_name": "ctr-a", "namespace_name": "ns-1"}
	labelsB := map[string]string{"pod_name": "pod-b", "container_name": "ctr-b", "namespace_name": "ns-1"}

	// ctr-a: ratio=16384/49152=0.333, core_used=90*0.333=30, core_util=30
	// ctr-b: ratio=32768/49152=0.667, core_used=90*0.667=60, core_util=60
	if got := readMetricAnyLabels("hami_container_core_used", labelsA); !approxEqual(30, got, 1.0) {
		t.Errorf("ctr-a core_used: want ~30, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", labelsA); !approxEqual(30, got, 1.0) {
		t.Errorf("ctr-a core_util: want ~30 (=core_used), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_core_used", labelsB); !approxEqual(60, got, 1.0) {
		t.Errorf("ctr-b core_used: want ~60, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_core_util", labelsB); !approxEqual(60, got, 1.0) {
		t.Errorf("ctr-b core_util: want ~60 (=core_used), got %v", got)
	}

	// ctr-a: memory_used = 28610.2 * (16384/49152) ≈ 9536.7 MB
	wantMemA := roundToOneDecimal(28610.2 * float64(16384) / float64(49152))
	if got := readMetricAnyLabels("hami_container_memory_used", labelsA); !approxEqual(wantMemA, got, 1.0) {
		t.Errorf("ctr-a memory_used: want ~%v, got %v", wantMemA, got)
	}
	// ctr-b: memory_used = 28610.2 * (32768/49152) ≈ 19073.5 MB
	wantMemB := roundToOneDecimal(28610.2 * float64(32768) / float64(49152))
	if got := readMetricAnyLabels("hami_container_memory_used", labelsB); !approxEqual(wantMemB, got, 1.0) {
		t.Errorf("ctr-b memory_used: want ~%v, got %v", wantMemB, got)
	}

	// Sum of memory should equal full card memory used
	if got := readMetricAnyLabels("hami_container_memory_used", labelsA) + readMetricAnyLabels("hami_container_memory_used", labelsB); !approxEqual(28610.2, got, 2.0) {
		t.Errorf("memory_used sum: want ~28610.2, got %v", got)
	}
}

func TestAscend910B_CardQueryFails_FallbackToContainerMetric(t *testing.T) {
	// Card-level query failure → ascendCardQueriesOK = false → fallback to taskCoreUsed/taskMemoryUsed
	// container_npu_* queries return empty (not mocked) → queryInstantVal returns (0, nil)
	// → metrics set to 0 (this PR branch does not have ascendSkipMetrics)
	deviceUUID := "npu-uuid-skip"
	devices := []*biz.DeviceInfo{
		{
			Id: deviceUUID, AliasId: deviceUUID,
			Count: 1, Devmem: 65536, Devcore: 100,
			Type: "Ascend910B", NodeName: "node-1", Provider: "Ascend", Health: true,
		},
	}
	containers := []*biz.Container{
		{
			Name: "ctr", PodName: "pod-1", Namespace: "ns-1", PodUID: "pod-uid-1", NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: deviceUUID, Type: "Ascend910B", Usedmem: 16384, Usedcores: 25},
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/query", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		if r.Method == "POST" {
			query = r.PostFormValue("query")
		}
		if strings.Contains(query, "npu_chip_info_utilization") || strings.Contains(query, "npu_chip_info_hbm_used_memory") {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	gen := newTestMetricsGenerator(t, server.URL, containers, devices)
	resetTestMetrics()

	err := gen.GenerateContainerMetrics(context.Background())
	if err != nil {
		t.Fatalf("GenerateContainerMetrics failed: %v", err)
	}

	labels := map[string]string{"pod_name": "pod-1", "container_name": "ctr", "namespace_name": "ns-1"}

	// Allocation metrics should be set
	if got := readMetricAnyLabels("hami_container_vgpu_allocated", labels); got != 1 {
		t.Errorf("hami_container_vgpu_allocated: want 1, got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_vmemory_allocated", labels); got != 16384 {
		t.Errorf("hami_container_vmemory_allocated: want 16384, got %v", got)
	}

	// Fallback path: container_npu_* returns empty → queryInstantVal returns (0, nil)
	// → taskCoreUsed=0, taskMemoryUsed=0 → metrics set to 0
	if got := readMetricAnyLabels("hami_container_core_used", labels); got != 0 {
		t.Errorf("hami_container_core_used: want 0 (fallback empty), got %v", got)
	}
	if got := readMetricAnyLabels("hami_container_memory_used", labels); got != 0 {
		t.Errorf("hami_container_memory_used: want 0 (fallback empty), got %v", got)
	}
}
