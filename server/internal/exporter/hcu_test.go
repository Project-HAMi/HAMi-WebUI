package exporter

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/provider/hygon"
	"vgpu/internal/provider/util"
)

func TestHCUPhysicalTelemetryUsesExporterNamesAndByteUnits(t *testing.T) {
	device := &biz.DeviceInfo{
		Id: "serial-01", AliasId: "HCU-serial-01", Type: "HCU-K100_AI",
		Provider: biz.HygonHCUDevice, NodeName: "hcu-node", Devmem: 65536, Devcore: 100, Count: 4,
	}
	generator := newDeviceMetricsTestGenerator(device, map[string]*pb.InstantResponse{
		`hcu_power_usage{device_id="serial-01"}`:           {Data: []*pb.Sample{{Metric: map[string]string{"minor_number": "3"}, Value: 150}}},
		`avg(hcu_memorycap_bytes{device_id="serial-01"})`:  instantValue(64 * 1024 * 1024 * 1024),
		`avg(hcu_usedmemory_bytes{device_id="serial-01"})`: instantValue(8 * 1024 * 1024 * 1024),
		`avg(hcu_utilizationrate{device_id="serial-01"})`:  instantValue(35),
		`avg(hcu_temp{device_id="serial-01"})`:             instantValue(48),
		`avg(hcu_temp_mem{device_id="serial-01"})`:         instantValue(51),
		`avg(hcu_power_usage{device_id="serial-01"})`:      instantValue(150),
	})
	t.Cleanup(func() { deleteTrackedTestCells(generator) })
	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	labels := []string{device.NodeName, "HCU", device.Type, device.Id, "", "hcu-3"}
	for gauge, want := range map[*prometheus.GaugeVec]float64{
		HamiVgpuCount: 4, HamiVmemorySize: 65536, HamiMemorySize: 65536,
		HamiMemoryUsed: 8192, HamiMemoryUtil: 12.5, HamiCoreUtil: 35,
		HamiDeviceTemperature: 48, HamiDeviceMemoryTemperature: 51, HamiDevicePower: 150,
	} {
		assertTrackedGaugeValue(t, generator, gauge, labels, want)
	}
}

func TestHCUContainerTelemetryJoinsRegisteredSerialAndKeepsMissingUsageAbsent(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)
	for _, tt := range []struct {
		name            string
		present         bool
		busy            float32
		used            float32
		annotationCores int
		wantCores       float64
		memoryMiB       int
		wantMemoryUtil  float64
	}{
		{name: "busy virtual allocation", present: true, busy: 80, used: 2 * 1024 * 1024 * 1024, annotationCores: 25, wantCores: 25, memoryMiB: 8192, wantMemoryUtil: 25},
		{name: "idle virtual allocation", present: true, annotationCores: 25, wantCores: 25, memoryMiB: 8192},
		{name: "missing workload telemetry", annotationCores: 25, wantCores: 25, memoryMiB: 8192},
		{name: "exclusive whole-card allocation", present: true, busy: 80, used: 2 * 1024 * 1024 * 1024, annotationCores: 100, wantCores: 100, memoryMiB: 65536, wantMemoryUtil: 3.1},
		// The plugin neither creates a vHCU nor marks the card for this shape,
		// so the exporter never labels it; only the allocation is reported.
		{name: "zero-core full-memory allocation without attribution", annotationCores: 0, wantCores: 100, memoryMiB: 65536},
	} {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "hcu-node", Annotations: map[string]string{
				"hami.io/node-hcu-register": "HCU-serial-01,4,65536,100,HCU-K100_AI,0,true,3,hami:",
			}}}
			registered, err := hygon.NewHCU(log.NewHelper(logger), "").FetchDevices(node)
			if err != nil || len(registered) != 1 {
				t.Fatalf("HCU registration = %v, %v", registered, err)
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "train", Namespace: "research", UID: "pod-uid", Annotations: map[string]string{
					"hami.io/hcu-devices-allocated": fmt.Sprintf("HCU-serial-01,HCU,%d,%d:;", tt.memoryMiB, tt.annotationCores),
				}},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
			}
			allocated, err := util.DecodePodDevices(pod, log.NewHelper(logger), util.AscendAllocationModeUnknown)
			if err != nil || len(allocated["HCU"]) != 1 || len(allocated["HCU"][0]) != 1 {
				t.Fatalf("HCU allocation = %v, %v", allocated, err)
			}
			allocation := allocated["HCU"][0][0]
			device := registered[0]
			responses := map[string]*pb.InstantResponse{}
			const workload = `device_id="serial-01", node="hcu-node", hcu_pod_namespace="research", hcu_pod_name="train"`
			computeQuery := fmt.Sprintf(`avg(avg_over_time(vhcu_utilizationrate{%[1]s, container="worker"}[1m]))`+
				` or avg(avg_over_time(vhcu_utilizationrate{%[1]s, exported_container="worker"}[1m]))`+
				` or avg(avg_over_time(hcu_utilizationrate{%[1]s, container="worker"}[1m]))`+
				` or avg(avg_over_time(hcu_utilizationrate{%[1]s, exported_container="worker"}[1m]))`, workload)
			memoryQuery := fmt.Sprintf(`avg(vhcu_usedmemory_bytes{%[1]s, container="worker"})`+
				` or avg(vhcu_usedmemory_bytes{%[1]s, exported_container="worker"})`+
				` or avg(hcu_usedmemory_bytes{%[1]s, container="worker"})`+
				` or avg(hcu_usedmemory_bytes{%[1]s, exported_container="worker"})`, workload)
			if tt.present {
				responses[computeQuery] = instantValue(tt.busy)
				responses[memoryQuery] = instantValue(tt.used)
			}
			querier := &fakeInstantQuerier{responsesByQuery: responses}
			generator := &MetricsGenerator{
				nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{{
					Id: device.ID, AliasId: device.AliasId, Type: device.Type, Provider: "HCU", NodeName: node.Name,
				}}}, logger),
				podUsecase: biz.NewPodUseCase(&fakePodRepo{containers: []*biz.Container{{
					Name: "worker", PodName: pod.Name, PodUID: string(pod.UID), Namespace: pod.Namespace,
					ContainerDevices: biz.ContainerDevices{{UUID: allocation.UUID, Type: allocation.Type, Usedmem: allocation.Usedmem, Usedcores: allocation.Usedcores}},
				}}}, logger),
				monitorService: querier,
				log:            log.NewHelper(logger),
			}
			t.Cleanup(func() { deleteTrackedTestCells(generator) })
			if err := generator.GenerateContainerMetrics(context.Background()); err != nil {
				t.Fatal(err)
			}
			labels := []string{node.Name, "HCU", device.Type, device.ID, pod.Name, "worker", pod.Namespace}
			allocationLabels := append(append([]string{}, labels...), "worker:pod-uid")
			assertTrackedGaugeValue(t, generator, HamiContainerVgpuAllocated, allocationLabels, 1)
			assertTrackedGaugeValue(t, generator, HamiContainerVcoreAllocated, allocationLabels, tt.wantCores)
			assertTrackedGaugeValue(t, generator, HamiContainerVmemoryAllocated, allocationLabels, float64(tt.memoryMiB))
			for gauge, want := range map[*prometheus.GaugeVec]float64{
				HamiContainerCoreUsed:   float64(tt.busy) * tt.wantCores / 100,
				HamiContainerCoreUtil:   float64(tt.busy),
				HamiContainerMemoryUsed: float64(tt.used) / 1024 / 1024,
				HamiContainerMemoryUtil: tt.wantMemoryUtil,
			} {
				assertGaugeTracked(t, generator, gauge, labels, tt.present)
				if tt.present {
					assertTrackedGaugeValue(t, generator, gauge, labels, want)
				}
			}
			if len(querier.queries) != 2 || querier.queries[0] != computeQuery || querier.queries[1] != memoryQuery {
				t.Fatalf("queries must use only exporter workload identities, without physical-card fallback queries: %v", querier.queries)
			}
		})
	}
}
