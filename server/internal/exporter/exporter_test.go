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
	queryErr         error
	queries          []string
}

func (f *fakeInstantQuerier) QueryInstant(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
	f.queries = append(f.queries, req.GetQuery())
	if f.queryErr != nil {
		return nil, f.queryErr
	}
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

type fakePodRepo struct {
	containers []*biz.Container
}

func (f *fakePodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return f.containers, nil
}

func (f *fakePodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
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

func TestTaskUsageQueriesDistinguishMissingFailureAndIdle(t *testing.T) {
	readers := []struct {
		name string
		read func(*MetricsGenerator) (float32, error)
	}{
		{name: "compute", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.taskCoreUsed(context.Background(), biz.CambriconGPUDevice, "research", "train", "worker", "pod-uid", "MLU-1", "node-1", 0)
		}},
		{name: "memory", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.taskMemoryUsed(context.Background(), biz.CambriconGPUDevice, "research", "train", "worker", "pod-uid", "MLU-1", "node-1", 0)
		}},
	}

	for _, reader := range readers {
		t.Run(reader.name+"/missing", func(t *testing.T) {
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{}}}}
			_, err := reader.read(generator)
			if !errors.Is(err, errNoMetricData) {
				t.Fatalf("expected errNoMetricData, got %v", err)
			}
		})

		t.Run(reader.name+"/query failure", func(t *testing.T) {
			queryErr := errors.New("prometheus unavailable")
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{queryErr: queryErr}}
			_, err := reader.read(generator)
			if !errors.Is(err, queryErr) {
				t.Fatalf("expected query error, got %v", err)
			}
		})

		t.Run(reader.name+"/idle", func(t *testing.T) {
			generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{{Data: []*pb.Sample{{Value: 0}}}}}}
			value, err := reader.read(generator)
			if err != nil || value != 0 {
				t.Fatalf("idle sample = (%v, %v), want (0, nil)", value, err)
			}
		})
	}
}

func TestAscendTaskUsageIsUnsupported(t *testing.T) {
	readers := []struct {
		name string
		read func(*MetricsGenerator) (float32, error)
	}{
		{name: "compute", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.taskCoreUsed(context.Background(), biz.AscendGPUDevice, "research", "train", "worker", "pod-uid", "ascend-1", "node-1", 0)
		}},
		{name: "memory", read: func(generator *MetricsGenerator) (float32, error) {
			return generator.taskMemoryUsed(context.Background(), biz.AscendGPUDevice, "research", "train", "worker", "pod-uid", "ascend-1", "node-1", 0)
		}},
	}

	for _, reader := range readers {
		t.Run(reader.name, func(t *testing.T) {
			querier := &fakeInstantQuerier{}
			generator := &MetricsGenerator{monitorService: querier}
			_, err := reader.read(generator)
			if !errors.Is(err, errWorkloadTelemetryUnsupported) {
				t.Fatalf("expected errWorkloadTelemetryUnsupported, got %v", err)
			}
			if len(querier.queries) != 0 {
				t.Fatalf("unsupported telemetry made %d queries, want 0", len(querier.queries))
			}
		})
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

func TestOptionalDeviceTelemetryDistinguishesMissingNonFiniteAndZero(t *testing.T) {
	readers := []struct {
		name string
		read func(*MetricsGenerator, string) (float32, error)
	}{
		{name: "temperature", read: func(generator *MetricsGenerator, provider string) (float32, error) {
			return generator.gpuTemperature(context.Background(), provider, "GPU-1")
		}},
		{name: "memory temperature", read: func(generator *MetricsGenerator, provider string) (float32, error) {
			return generator.memoryTemperature(context.Background(), provider, "GPU-1")
		}},
		{name: "power", read: func(generator *MetricsGenerator, provider string) (float32, error) {
			return generator.gpuPower(context.Background(), provider, "GPU-1")
		}},
		{name: "fan speed", read: func(generator *MetricsGenerator, provider string) (float32, error) {
			return generator.fanSpeed(context.Background(), provider, "GPU-1")
		}},
		{name: "hardware health", read: func(generator *MetricsGenerator, provider string) (float32, error) {
			return generator.gpuHardwareHealth(context.Background(), provider, "GPU-1")
		}},
	}
	tests := []struct {
		name        string
		response    *pb.InstantResponse
		wantValue   float32
		wantPresent bool
	}{
		{name: "missing", response: &pb.InstantResponse{}},
		{name: "zero", response: instantValue(0), wantPresent: true},
		{name: "ordinary value", response: instantValue(42.5), wantValue: 42.5, wantPresent: true},
		{name: "NaN", response: instantValue(float32(math.NaN()))},
		{name: "positive infinity", response: instantValue(float32(math.Inf(1)))},
		{name: "negative infinity", response: instantValue(float32(math.Inf(-1)))},
	}

	for _, reader := range readers {
		for _, tt := range tests {
			t.Run(reader.name+"/"+tt.name, func(t *testing.T) {
				generator := &MetricsGenerator{monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{tt.response}}}
				value, err := reader.read(generator, biz.NvidiaGPUDevice)
				if tt.wantPresent {
					if err != nil || value != tt.wantValue {
						t.Fatalf("optional telemetry = (%v, %v), want (%v, nil)", value, err, tt.wantValue)
					}
					return
				}
				if !errors.Is(err, errNoMetricData) {
					t.Fatalf("expected errNoMetricData, got (%v, %v)", value, err)
				}
			})
		}

		t.Run(reader.name+"/unsupported provider", func(t *testing.T) {
			querier := &fakeInstantQuerier{}
			generator := &MetricsGenerator{monitorService: querier}
			_, err := reader.read(generator, "unsupported")
			if err == nil {
				t.Fatal("unsupported provider returned a nil error")
			}
			if len(querier.queries) != 0 {
				t.Fatalf("unsupported provider made %d queries, want 0", len(querier.queries))
			}
		})
	}
}

func TestProviderDeviceTelemetryUsesPhysicalMetricSeries(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		wantQuery string
		read      func(*MetricsGenerator, string, string) (float32, error)
	}{
		{
			name:      "Ascend HBM temperature",
			provider:  biz.AscendGPUDevice,
			wantQuery: `avg(npu_chip_info_hbm_temperature{vdie_id="device-1"})`,
			read: func(generator *MetricsGenerator, provider, deviceUUID string) (float32, error) {
				return generator.memoryTemperature(context.Background(), provider, deviceUUID)
			},
		},
		{
			name:      "MetaX HBM temperature",
			provider:  biz.MetaxGPUDevice,
			wantQuery: `avg(mx_chip_hbm_temp{uuid="device-1"})`,
			read: func(generator *MetricsGenerator, provider, deviceUUID string) (float32, error) {
				return generator.memoryTemperature(context.Background(), provider, deviceUUID)
			},
		},
		{
			name:      "MLU physical memory temperature",
			provider:  biz.CambriconGPUDevice,
			wantQuery: `avg(mlu_memory_temperature{uuid="device-1",memory_die=""})`,
			read: func(generator *MetricsGenerator, provider, deviceUUID string) (float32, error) {
				return generator.memoryTemperature(context.Background(), provider, deviceUUID)
			},
		},
		{
			name:      "MLU physical power",
			provider:  biz.CambriconGPUDevice,
			wantQuery: `avg(mlu_power_usage{uuid="device-1",vf=""})`,
			read: func(generator *MetricsGenerator, provider, deviceUUID string) (float32, error) {
				return generator.gpuPower(context.Background(), provider, deviceUUID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			querier := &fakeInstantQuerier{responsesByQuery: map[string]*pb.InstantResponse{
				tt.wantQuery: instantValue(42.5),
			}}
			generator := &MetricsGenerator{monitorService: querier}

			value, err := tt.read(generator, tt.provider, "device-1")
			if err != nil || value != 42.5 {
				t.Fatalf("telemetry = (%v, %v), want (42.5, nil)", value, err)
			}
			if len(querier.queries) != 1 || querier.queries[0] != tt.wantQuery {
				t.Fatalf("queries = %q, want [%q]", querier.queries, tt.wantQuery)
			}
		})
	}
}

func TestMLUAdditionalInfoUsesPhysicalPowerSeries(t *testing.T) {
	const wantQuery = `mlu_power_usage{uuid="device-1",vf=""}`
	querier := &fakeInstantQuerier{responsesByQuery: map[string]*pb.InstantResponse{
		wantQuery: {
			Data: []*pb.Sample{{Metric: map[string]string{
				"driver": "1.2.3",
				"sn":     "MLU-serial",
			}}},
		},
	}}
	generator := &MetricsGenerator{monitorService: querier}

	info, err := generator.queryDeviceAdditional(context.Background(), biz.CambriconGPUDevice, "device-1")
	if err != nil {
		t.Fatalf("queryDeviceAdditional() error = %v", err)
	}
	if len(querier.queries) != 1 || querier.queries[0] != wantQuery {
		t.Fatalf("queries = %q, want [%q]", querier.queries, wantQuery)
	}
	if info.DriverVersion != "1.2.3" || info.DeviceNo != "MLU-serial" {
		t.Fatalf("additional info = %+v, want driver 1.2.3 and device MLU-serial", info)
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

func TestDeviceAdditionalInfoKeepsUnknownDriverVersionEmpty(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		metric       map[string]string
		wantQuery    string
		wantDeviceNo string
	}{
		{
			name:         "Ascend",
			provider:     biz.AscendGPUDevice,
			metric:       map[string]string{"id": "7"},
			wantQuery:    `npu_chip_info_power{vdie_id="device-1"}`,
			wantDeviceNo: "ascend-7",
		},
		{
			name:         "Hygon",
			provider:     biz.HygonGPUDevice,
			metric:       map[string]string{"minor_number": "3"},
			wantQuery:    `dcu_power_usage{device_id="device-1"}`,
			wantDeviceNo: "dcu-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			querier := &fakeInstantQuerier{responses: []*pb.InstantResponse{{
				Data: []*pb.Sample{{Metric: tt.metric}},
			}}}
			generator := &MetricsGenerator{monitorService: querier}

			info, err := generator.queryDeviceAdditional(context.Background(), tt.provider, "device-1")
			if err != nil {
				t.Fatalf("queryDeviceAdditional() error = %v", err)
			}
			if len(querier.queries) != 1 || querier.queries[0] != tt.wantQuery {
				t.Fatalf("queries = %q, want [%q]", querier.queries, tt.wantQuery)
			}
			if info.DriverVersion != "" {
				t.Fatalf("driver version = %q, want empty", info.DriverVersion)
			}
			if info.DeviceNo != tt.wantDeviceNo {
				t.Fatalf("device number = %q, want %q", info.DeviceNo, tt.wantDeviceNo)
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

func TestGenerateDeviceMetricsExportsOnlyPresentFiniteOptionalTelemetry(t *testing.T) {
	tests := []struct {
		name        string
		deviceID    string
		response    *pb.InstantResponse
		wantTracked bool
		wantValue   float64
	}{
		{name: "missing", deviceID: "GPU-optional-missing"},
		{name: "zero", deviceID: "GPU-optional-zero", response: instantValue(0), wantTracked: true},
		{name: "ordinary value", deviceID: "GPU-optional-value", response: instantValue(42.5), wantTracked: true, wantValue: 42.5},
		{name: "NaN", deviceID: "GPU-optional-nan", response: instantValue(float32(math.NaN()))},
		{name: "positive infinity", deviceID: "GPU-optional-pos-inf", response: instantValue(float32(math.Inf(1)))},
		{name: "negative infinity", deviceID: "GPU-optional-neg-inf", response: instantValue(float32(math.Inf(-1)))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := map[string]*pb.InstantResponse{}
			if tt.response != nil {
				responses[`avg(DCGM_FI_DEV_GPU_TEMP{UUID="`+tt.deviceID+`"})`] = tt.response
				responses[`avg(DCGM_FI_DEV_MEMORY_TEMP{UUID="`+tt.deviceID+`"})`] = tt.response
				responses[`avg(DCGM_FI_DEV_POWER_USAGE{UUID="`+tt.deviceID+`"})`] = tt.response
				responses[`avg(DCGM_FI_DEV_FAN_SPEED{UUID="`+tt.deviceID+`"})`] = tt.response
				responses[`avg(DCGM_FI_DEV_XID_ERRORS{UUID="`+tt.deviceID+`"})`] = tt.response
			}
			generator := newDeviceMetricsTestGenerator(
				&biz.DeviceInfo{
					Id: tt.deviceID, Devmem: 40960, Devcore: 100, Count: 1,
					Type: "A100", NodeName: "node-optional", Provider: biz.NvidiaGPUDevice,
				},
				responses,
			)
			t.Cleanup(func() { deleteTrackedTestCells(generator) })

			if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
				t.Fatalf("GenerateDeviceMetrics() error = %v", err)
			}
			labels := []string{"node-optional", biz.NvidiaGPUDevice, "A100", tt.deviceID, "", ""}
			for _, gauge := range []*prometheus.GaugeVec{
				HamiDeviceTemperature,
				HamiDeviceMemoryTemperature,
				HamiDevicePower,
				HamiDeviceFanSpeedP,
				HamiDeviceHardwareHealth,
			} {
				assertGaugeTracked(t, generator, gauge, labels, tt.wantTracked)
				if tt.wantTracked && testutil.ToFloat64(gauge.WithLabelValues(labels...)) != tt.wantValue {
					t.Fatalf("optional gauge value = %v, want %v", testutil.ToFloat64(gauge.WithLabelValues(labels...)), tt.wantValue)
				}
			}
		})
	}
}

func TestGenerateDeviceMetricsOmitsUnsupportedOptionalTelemetry(t *testing.T) {
	const deviceID = "DCU-optional-support"
	generator := newDeviceMetricsTestGenerator(
		&biz.DeviceInfo{
			Id: deviceID, Devmem: 16384, Devcore: 100, Count: 1,
			Type: "DCU", NodeName: "node-dcu", Provider: biz.HygonGPUDevice,
		},
		map[string]*pb.InstantResponse{
			`avg(dcu_temp{device_id="DCU-optional-support"})`:        instantValue(0),
			`avg(dcu_power_usage{device_id="DCU-optional-support"})`: instantValue(0),
		},
	)
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-dcu", biz.HygonGPUDevice, "DCU", deviceID, "", ""}
	assertTrackedGaugeValue(t, generator, HamiDeviceTemperature, labels, 0)
	assertTrackedGaugeValue(t, generator, HamiDevicePower, labels, 0)
	for _, gauge := range []*prometheus.GaugeVec{
		HamiDeviceMemoryTemperature,
		HamiDeviceFanSpeedP,
		HamiDeviceFanSpeedR,
		HamiDeviceHardwareHealth,
	} {
		assertGaugeTracked(t, generator, gauge, labels, false)
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

func TestGenerateContainerMetricsPreservesMissingAndIdleUsage(t *testing.T) {
	const (
		deviceID   = "GPU-workload-presence"
		nodeName   = "node-workload-presence"
		deviceType = "A100"
		podName    = "train"
		container  = "worker"
		namespace  = "research"
		podUID     = "pod-uid"
	)

	tests := []struct {
		name         string
		response     *pb.InstantResponse
		wantUsageSet bool
	}{
		{name: "empty vectors omit usage", response: &pb.InstantResponse{}, wantUsageSet: false},
		{name: "real zero exports idle usage", response: instantValue(0), wantUsageSet: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &MetricsGenerator{
				nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
					Id: deviceID, Type: deviceType, Provider: biz.NvidiaGPUDevice, NodeName: nodeName,
				}}}, log.NewStdLogger(io.Discard)),
				podUsecase: biz.NewPodUseCase(&fakePodRepo{containers: []*biz.Container{{
					Name: container, PodName: podName, PodUID: podUID, Namespace: namespace,
					ContainerDevices: biz.ContainerDevices{{UUID: deviceID, Type: biz.NvidiaGPUDevice, Usedmem: 1024, Usedcores: 50}},
				}}}, log.NewStdLogger(io.Discard)),
				monitorService: &fakeInstantQuerier{responses: []*pb.InstantResponse{tt.response, tt.response}},
				log:            log.NewHelper(log.NewStdLogger(io.Discard)),
			}
			t.Cleanup(func() { deleteTrackedTestCells(generator) })

			if err := generator.GenerateContainerMetrics(context.Background()); err != nil {
				t.Fatalf("GenerateContainerMetrics() error = %v", err)
			}
			labels := []string{nodeName, biz.NvidiaGPUDevice, deviceType, deviceID, podName, container, namespace}
			for _, gauge := range []*prometheus.GaugeVec{HamiContainerCoreUsed, HamiContainerCoreUtil, HamiContainerMemoryUsed, HamiContainerMemoryUtil} {
				assertGaugeTracked(t, generator, gauge, labels, tt.wantUsageSet)
				if tt.wantUsageSet && testutil.ToFloat64(gauge.WithLabelValues(labels...)) != 0 {
					t.Fatalf("idle gauge value must be zero")
				}
			}
		})
	}
}

func TestAscendContainerAllocationOmitsUnknownCoreAndUnsupportedUsage(t *testing.T) {
	tests := []struct {
		name          string
		core          int32
		coreKnown     bool
		wantCoreGauge bool
		wantKnown     float64
	}{
		{name: "known template", core: 25, coreKnown: true, wantCoreGauge: true, wantKnown: 1},
		{name: "known zero", core: 0, coreKnown: true, wantCoreGauge: true, wantKnown: 1},
		{name: "unknown template", core: 0, coreKnown: false, wantCoreGauge: false, wantKnown: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				aliasID   = "B4-alias"
				deviceID  = "B4-telemetry"
				nodeName  = "ascend-node"
				podName   = "ascend-pod"
				container = "worker"
				namespace = "research"
				podUID    = "pod-uid"
			)
			generator := &MetricsGenerator{
				nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
					Id: deviceID, AliasId: aliasID, Type: "Ascend910B4", Provider: biz.AscendGPUDevice, NodeName: nodeName,
				}}}, log.NewStdLogger(io.Discard)),
				podUsecase: biz.NewPodUseCase(&fakePodRepo{containers: []*biz.Container{{
					Name: container, PodName: podName, PodUID: podUID, Namespace: namespace,
					ContainerDevices: biz.ContainerDevices{{UUID: aliasID, Type: "Ascend910B4", Usedmem: 8192, Usedcores: tt.core, CoreAllocationKnown: tt.coreKnown}},
				}}}, log.NewStdLogger(io.Discard)),
				monitorService: &fakeInstantQuerier{},
				log:            log.NewHelper(log.NewStdLogger(io.Discard)),
			}
			t.Cleanup(func() { deleteTrackedTestCells(generator) })

			if err := generator.GenerateContainerMetrics(context.Background()); err != nil {
				t.Fatalf("GenerateContainerMetrics() error = %v", err)
			}
			labels := []string{nodeName, biz.AscendGPUDevice, "Ascend910B4", deviceID, podName, container, namespace, container + ":" + podUID}
			assertGaugeTracked(t, generator, HamiContainerVgpuAllocated, labels, true)
			assertGaugeTracked(t, generator, HamiContainerVmemoryAllocated, labels, true)
			assertGaugeTracked(t, generator, HamiContainerVcoreAllocated, labels, tt.wantCoreGauge)
			assertTrackedGaugeValue(t, generator, HamiContainerVcoreAllocationKnown, labels, tt.wantKnown)
			usageLabels := labels[:len(labels)-1]
			for _, gauge := range []*prometheus.GaugeVec{HamiContainerCoreUsed, HamiContainerCoreUtil, HamiContainerMemoryUsed, HamiContainerMemoryUtil} {
				assertGaugeTracked(t, generator, gauge, usageLabels, false)
			}
			if querier := generator.monitorService.(*fakeInstantQuerier); len(querier.queries) != 0 {
				t.Fatalf("unsupported Ascend workload telemetry made %d queries, want 0", len(querier.queries))
			}
		})
	}
}

func TestNonAscendContainerAllocationIsKnown(t *testing.T) {
	const (
		deviceID  = "GPU-allocation-known"
		nodeName  = "nvidia-node"
		podName   = "nvidia-pod"
		container = "worker"
		namespace = "research"
		podUID    = "pod-uid"
	)
	generator := &MetricsGenerator{
		nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
			Id: deviceID, AliasId: deviceID, Type: "A100", Provider: biz.NvidiaGPUDevice, NodeName: nodeName,
		}}}, log.NewStdLogger(io.Discard)),
		podUsecase: biz.NewPodUseCase(&fakePodRepo{containers: []*biz.Container{{
			Name: container, PodName: podName, PodUID: podUID, Namespace: namespace,
			ContainerDevices: biz.ContainerDevices{{UUID: deviceID, Type: biz.NvidiaGPUDevice, Usedmem: 8192, Usedcores: 0}},
		}}}, log.NewStdLogger(io.Discard)),
		monitorService: &fakeInstantQuerier{},
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
	t.Cleanup(func() { deleteTrackedTestCells(generator) })

	if err := generator.GenerateContainerMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateContainerMetrics() error = %v", err)
	}
	labels := []string{nodeName, biz.NvidiaGPUDevice, "A100", deviceID, podName, container, namespace, container + ":" + podUID}
	assertTrackedGaugeValue(t, generator, HamiContainerVcoreAllocated, labels, 0)
	assertTrackedGaugeValue(t, generator, HamiContainerVcoreAllocationKnown, labels, 1)
}
