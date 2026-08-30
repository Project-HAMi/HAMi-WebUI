package exporter

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/provider/metax"
)

type fakeInstantQuerier struct {
	responses []*pb.InstantResponse
	queries   []string
}

func (f *fakeInstantQuerier) QueryInstant(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
	f.queries = append(f.queries, req.GetQuery())
	if len(f.responses) == 0 {
		return &pb.InstantResponse{}, nil
	}
	res := f.responses[0]
	f.responses = f.responses[1:]
	return res, nil
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
	for _, value := range []float32{float32(math.NaN()), float32(math.Inf(1)), float32(math.Inf(-1))} {
		generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: value}}}}}}
		_, err := generator.deviceCoreUtil(context.Background(), biz.NvidiaGPUDevice, "GPU-1")
		if !errors.Is(err, errNoMetricData) {
			t.Fatalf("expected errNoMetricData for %v, got %v", value, err)
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
