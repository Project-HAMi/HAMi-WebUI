package exporter

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
)

// testPromHandler returns a handler that acts as a fake Prometheus /api/v1/query
// endpoint. When value is non-empty, it returns a single-sample vector with that
// value. When value is empty, it returns an empty result vector.
func testPromHandler(t *testing.T, value string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/v1/query") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if value == "" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			return
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1719500000,%q]}]}}`, value)
	}
}

// newTestGenerator starts an httptest server with the given handler, creates a
// prom.Client pointed at it, wraps it in a MonitorService, and returns a bare
// MetricsGenerator (other fields left nil). The returned close func shuts down
// the test server.
func newTestGenerator(t *testing.T, handler http.HandlerFunc) (*MetricsGenerator, func()) {
	srv := httptest.NewServer(handler)

	promClient, err := prom.NewClient(srv.URL, time.Second, "")
	if err != nil {
		srv.Close()
		t.Fatalf("prom.NewClient: %v", err)
	}

	monitorSvc := service.NewMonitorService(promClient, nil, nil)

	gen := &MetricsGenerator{
		monitorService: monitorSvc,
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}

	return gen, srv.Close
}

func TestMetricsGenerator_taskCoreUsed_Ascend(t *testing.T) {
	gen, cleanup := newTestGenerator(t, testPromHandler(t, "42.5"))
	defer cleanup()

	result, err := gen.taskCoreUsed(context.Background(), biz.AscendGPUDevice, "ns", "pod", "ctr", "pod-uuid", "dev-uuid", "host", 0)
	if err != nil {
		t.Fatalf("taskCoreUsed failed: %v", err)
	}
	if result != 42.5 {
		t.Errorf("expected 42.5, got %v", result)
	}
}

func TestMetricsGenerator_taskMemoryUsed_Ascend(t *testing.T) {
	// npu_chip_info_hbm_used_memory returns bytes, e.g., 8589934592 for 8 GiB HBM
	gen, cleanup := newTestGenerator(t, testPromHandler(t, "8589934592"))
	defer cleanup()

	result, err := gen.taskMemoryUsed(context.Background(), biz.AscendGPUDevice, "ns", "pod", "ctr", "pod-uuid", "dev-uuid", "host", 0)
	if err != nil {
		t.Fatalf("taskMemoryUsed failed: %v", err)
	}
	if result != 8589934592 {
		t.Errorf("expected 8589934592 (bytes), got %v", result)
	}
}

func TestMetricsGenerator_taskCoreUsed_Ascend_EmptyResult(t *testing.T) {
	gen, cleanup := newTestGenerator(t, testPromHandler(t, ""))
	defer cleanup()

	result, err := gen.taskCoreUsed(context.Background(), biz.AscendGPUDevice, "ns", "pod", "ctr", "pod-uuid", "dev-uuid", "host", 0)
	if err != nil {
		t.Fatalf("taskCoreUsed failed: %v", err)
	}
	if result != 0 {
		t.Errorf("expected 0, got %v", result)
	}
}

// TestMetricsGenerator_deviceMemUsed_Ascend verifies the device-level memory query
// returns raw bytes from npu_chip_info_hbm_used_memory.
func TestMetricsGenerator_deviceMemUsed_Ascend(t *testing.T) {
	gen, cleanup := newTestGenerator(t, testPromHandler(t, "8589934592"))
	defer cleanup()

	result, err := gen.deviceMemUsed(context.Background(), biz.AscendGPUDevice, "dev-uuid")
	if err != nil {
		t.Fatalf("deviceMemUsed failed: %v", err)
	}
	// deviceMemUsed returns the raw Prometheus value (bytes) without any divisor for Ascend.
	if result != 8589934592 {
		t.Errorf("expected 8589934592 (bytes), got %v", result)
	}
}

// TestMetricsGenerator_AscendMemoryConversion verifies the container memory usage
// conversion: taskMemoryUsed returns bytes, downstream /1024/1024 produces MiB.
func TestMetricsGenerator_AscendMemoryConversion(t *testing.T) {
	gen, cleanup := newTestGenerator(t, testPromHandler(t, "8589934592"))
	defer cleanup()

	// Simulate the GenerateContainerMetrics memory conversion for Ascend:
	// taskMemoryUsed returns raw bytes (no *1000*1000 MB conversion).
	rawBytes, err := gen.taskMemoryUsed(context.Background(), biz.AscendGPUDevice, "ns", "pod", "ctr", "pod-uuid", "dev-uuid", "host", 0)
	if err != nil {
		t.Fatalf("taskMemoryUsed failed: %v", err)
	}
	// Downstream: HamiContainerMemoryUsed = taskMemoryUsed / 1024 / 1024 (bytes → MiB)
	containerMemoryUsedMiB := float64(rawBytes) / 1024 / 1024
	// 8 GiB = 8192 MiB
	expected := float64(8192)
	if containerMemoryUsedMiB != expected {
		t.Errorf("expected container memory %.0f MiB, got %.0f MiB", expected, containerMemoryUsedMiB)
	}
}

// TestMetricsGenerator_AscendCoreConversion verifies the container core usage
// conversion: used = taskCoreUsed / 100 * core, util = taskCoreUsed.
func TestMetricsGenerator_AscendCoreConversion(t *testing.T) {
	gen, cleanup := newTestGenerator(t, testPromHandler(t, "42.5"))
	defer cleanup()

	taskCoreUtil, err := gen.taskCoreUsed(context.Background(), biz.AscendGPUDevice, "ns", "pod", "ctr", "pod-uuid", "dev-uuid", "host", 0)
	if err != nil {
		t.Fatalf("taskCoreUsed failed: %v", err)
	}
	if taskCoreUtil != 42.5 {
		t.Errorf("expected 42.5, got %v", taskCoreUtil)
	}

	// Simulate the GenerateContainerMetrics Ascend core conversion:
	// used = float64(taskCoreUsed) / 100 * float64(core)
	// util = float64(taskCoreUsed)
	const allocatedCore int32 = 50
	used := float64(taskCoreUtil) / 100 * float64(allocatedCore)
	util := float64(taskCoreUtil)

	if used != 21.25 {
		t.Errorf("expected used=21.25, got %v", used)
	}
	if util != 42.5 {
		t.Errorf("expected util=42.5, got %v", util)
	}
}
