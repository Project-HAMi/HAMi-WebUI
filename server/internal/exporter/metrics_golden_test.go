package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/resourceview"
)

type metricDescriptorGolden struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Labels []string `json:"labels"`
}

func TestMetricsContractGoldens(t *testing.T) {
	resetRefreshSelfMetrics(t)

	at := time.Unix(1760000000, 0).UTC()
	nvidiaID := resourceview.DeviceIdentity{
		Provider: biz.NvidiaGPUDevice,
		Node:     "node-1",
		NodeUID:  types.UID("node-uid-1"),
		DeviceID: "GPU-1",
	}
	cambriconID := resourceview.DeviceIdentity{
		Provider: biz.CambriconGPUDevice,
		Node:     "node-1",
		NodeUID:  types.UID("node-uid-1"),
		DeviceID: "MLU-1",
	}
	snapshot := resourceview.Snapshot{
		At:           at,
		NodeCoverage: completeCoverage("node", "shared_node_informer", 1),
		Nodes:        []resourceview.Node{{Name: "node-1", UID: types.UID("node-uid-1")}},
		Devices: []resourceview.Device{
			{
				Identity:             nvidiaID,
				Kind:                 "physical",
				Vendor:               "nvidia",
				Model:                "NVIDIA A100",
				UUID:                 proto.String("GPU-1"),
				Resource:             "nvidia.com/gpu",
				Mode:                 "hami-core",
				Configured:           proto.Bool(true),
				SlotsCapacity:        knownResource("4", "slots"),
				ComputeCapacityShare: knownResource("1", "share"),
				MemoryCapacityBytes:  knownResource("85899345920", "bytes"),
				AllocationCoverage:   completeCoverage("occupancy", "hami", 1),
				EffectiveOccupancies: []resourceview.EffectiveOccupancy{{
					Device:                nvidiaID,
					PodUID:                types.UID("pod-uid-1"),
					Source:                "hami",
					SlotsAllocated:        knownResource("1", "slots"),
					ComputeAllocatedShare: knownResource("0.5", "share"),
					MemoryAllocatedBytes:  knownResource("21474836480", "bytes"),
				}},
				States: map[string]*viewv1.FieldState{
					"kind":     {State: resourceview.Known},
					"model":    {State: resourceview.Known},
					"uuid":     {State: resourceview.Known},
					"resource": {State: resourceview.Known},
					"mode":     {State: resourceview.Known},
				},
			},
		},
	}

	knownQty := func(val, unit, source string) resourceview.Quantity {
		q := resource.MustParse(val)
		return resourceview.Quantity{
			Value: &q,
			Unit:  unit,
			State: &viewv1.FieldState{
				State:      resourceview.Known,
				Source:     source,
				Scope:      "device",
				ObservedAt: timestamppb.New(at),
			},
		}
	}
	utilQty := knownQty("0.42", "ratio", "dcgm")
	usedQty := knownQty("21474836480", "bytes", "dcgm")
	totalQty := knownQty("85899345920", "bytes", "dcgm")

	mluUtil := knownQty("0.3", "ratio", "cambricon")
	mluUsed := knownQty("1073741824", "bytes", "cambricon")
	mluTotal := knownQty("34359738368", "bytes", "cambricon")

	nvidiaDev := &biz.DeviceInfo{
		Id:       "GPU-1",
		AliasId:  "GPU-1",
		Count:    4,
		Devmem:   81920,
		Devcore:  100,
		Type:     "NVIDIA A100",
		Mode:     "hami-core",
		Health:   true,
		NodeName: "node-1",
		NodeUid:  "node-uid-1",
		Provider: biz.NvidiaGPUDevice,
	}
	cambriconDev := &biz.DeviceInfo{
		Id:       "MLU-1",
		AliasId:  "MLU-1",
		Count:    1,
		Devmem:   32768,
		Devcore:  100,
		Type:     "MLU370",
		Mode:     "smlu",
		Health:   true,
		NodeName: "node-1",
		NodeUid:  "node-uid-1",
		Provider: biz.CambriconGPUDevice,
	}

	container := &biz.Container{
		Name:       "trainer",
		PodUID:     "pod-uid-1",
		PodName:    "train-0",
		Namespace:  "default",
		NodeName:   "node-1",
		NodeUID:    "node-uid-1",
		CreateTime: at,
		ContainerDevices: biz.ContainerDevices{{
			UUID:      "GPU-1",
			Type:      "NVIDIA A100",
			Vendor:    biz.NvidiaGPUDevice,
			Usedmem:   20480,
			Usedcores: 50,
		}},
	}

	querier := &fakeInstantQuerier{
		query: func(_ context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
			q := req.GetQuery()
			switch {
			case strings.HasPrefix(q, "DCGM_FI_DEV_POWER_USAGE{"):
				return &pb.InstantResponse{Data: []*pb.Sample{{Metric: map[string]string{
					"DCGM_FI_DRIVER_VERSION": "550.54",
					"device":                 "nvidia0",
				}, Value: 180}}}, nil
			case strings.HasPrefix(q, "mlu_power_usage{"):
				return &pb.InstantResponse{Data: []*pb.Sample{{Metric: map[string]string{
					"driver": "5.10",
					"sn":     "mlu-sn-0",
				}, Value: 120}}}, nil
			case strings.Contains(q, "DCGM_FI_DEV_GPU_TEMP"),
				strings.Contains(q, "DCGM_FI_DEV_MEMORY_TEMP"),
				strings.Contains(q, "DCGM_FI_DEV_POWER_USAGE"),
				strings.Contains(q, "DCGM_FI_DEV_FAN_SPEED"),
				strings.Contains(q, "DCGM_FI_DEV_XID_ERRORS"),
				strings.Contains(q, "hami_container_device_utilization_ratio"),
				strings.Contains(q, "hami_vgpu_memory_used_bytes"),
				strings.Contains(q, "mlu_fan_speed"):
				return instantValue(42), nil
			default:
				return &pb.InstantResponse{}, nil
			}
		},
	}

	logger := log.NewStdLogger(io.Discard)
	g := &MetricsGenerator{
		nodeUsecase:      biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{nvidiaDev, cambriconDev}}, logger),
		podUsecase:       biz.NewPodUseCase(&fakePodRepo{containers: []*biz.Container{container}}, logger),
		monitorService:   querier,
		timeout:          time.Minute,
		log:              log.NewHelper(logger),
		now:              func() time.Time { return at },
		resourceSnapshot: &snapshot,
		physicalGroups:   resourceview.GroupPhysicalDevices(snapshot).Groups,
		observations: resourceview.DeviceObservations{
			nvidiaID:    {UtilizationRatio: utilQty, MemoryUsedBytes: usedQty, MemoryPhysicalBytes: totalQty},
			cambriconID: {UtilizationRatio: mluUtil, MemoryUsedBytes: mluUsed, MemoryPhysicalBytes: mluTotal},
		},
	}
	t.Cleanup(func() {
		g.beginCycle()
		g.commitCycle()
	})

	if err := g.GenerateMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateMetrics error = %v", err)
	}
	g.commitCycle()

	families, err := NewMetricsGatherer(g).Gather()
	if err != nil {
		t.Fatalf("Gather error = %v", err)
	}

	got := make([]metricDescriptorGolden, 0, len(families))
	byName := make(map[string]metricDescriptorGolden, len(families))
	for _, fam := range families {
		name := fam.GetName()
		if !strings.HasPrefix(name, "hami_") && !strings.HasPrefix(name, "hami_webui_") {
			continue
		}
		var labels []string
		for idx, m := range fam.GetMetric() {
			current := make([]string, 0, len(m.GetLabel()))
			for _, l := range m.GetLabel() {
				current = append(current, l.GetName())
			}
			sort.Strings(current)
			if idx == 0 {
				labels = current
			} else if !slices.Equal(labels, current) {
				t.Fatalf("metric %s has inconsistent label sets across series: %v vs %v", name, labels, current)
			}
		}
		entry := metricDescriptorGolden{
			Name:   name,
			Type:   strings.ToLower(fam.GetType().String()),
			Labels: labels,
		}
		got = append(got, entry)
		byName[name] = entry
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	required := []string{
		"hami_vcore_scaling", "hami_vmemory_scaling", "hami_vgpu_count",
		"hami_vmemory_size", "hami_vcore_size", "hami_memory_size",
		"hami_memory_used", "hami_memory_util", "hami_core_size",
		"hami_core_used", "hami_core_util", "hami_core_used_avg",
		"hami_core_util_avg", "hami_device_temperature", "hami_device_memory_temperature",
		"hami_device_power", "hami_device_fan_speed_p", "hami_device_fan_speed_r",
		"hami_device_last_xid_error_code",
		"hami_container_vgpu_allocated", "hami_container_vmemory_allocated",
		"hami_container_vcore_allocated", "hami_container_vcore_allocation_known",
		"hami_container_memory_used", "hami_container_memory_util",
		"hami_container_core_used", "hami_container_core_util",
		"hami_webui_device_info", "hami_webui_metrics_schema_info",
		"hami_webui_device_slots_capacity", "hami_webui_device_compute_capacity_ratio",
		"hami_webui_device_memory_capacity_bytes", "hami_webui_device_slots_allocated",
		"hami_webui_device_compute_allocated_ratio", "hami_webui_device_memory_allocated_bytes",
		"hami_webui_device_compute_unknown_occupancies", "hami_webui_device_utilization_ratio",
		"hami_webui_device_memory_used_bytes", "hami_webui_device_memory_physical_bytes",
		"hami_webui_metrics_refresh_success", "hami_webui_metrics_refresh_duration_seconds",
		"hami_webui_metrics_refresh_last_success_timestamp_seconds",
		"hami_webui_prom_queries_total", "hami_webui_prom_circuit_breaker_opens_total",
		"hami_webui_prom_circuit_breaker_open", "hami_webui_telemetry_batch_splits_total",
		"hami_webui_telemetry_observations",
	}
	for _, name := range required {
		if _, ok := byName[name]; !ok {
			t.Fatalf("required exported metric %q missing from gatherer output", name)
		}
	}

	goldenPath := filepath.Join("..", "..", "..", "test", "contracts", "metrics-goldens", "descriptors.json")
	raw, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if os.Getenv("UPDATE_METRICS_GOLDENS") == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with UPDATE_METRICS_GOLDENS=1 to generate)", goldenPath, err)
	}
	var want []metricDescriptorGolden
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatalf("parse golden %s: %v", goldenPath, err)
	}
	canonicalGot, _ := json.Marshal(got)
	canonicalWant, _ := json.Marshal(want)
	if !bytes.Equal(canonicalGot, canonicalWant) {
		t.Fatalf("metrics contract golden drift:\ngot:  %s\nwant: %s", string(canonicalGot), string(canonicalWant))
	}
}
