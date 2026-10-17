package exporter

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

// AMD telemetry comes from amd-device-plugin's k8s-vgpu-monitor, which reports
// bytes under the NVIDIA monitor's metric names and labels them with the device
// UUID of the registration.
func TestAMDPhysicalTelemetryUsesMonitorNamesAndByteUnits(t *testing.T) {
	device := &biz.DeviceInfo{
		Id: "33ff7590-0000-1000-8021-340866ba8c47", Type: "AMD Radeon Graphics",
		Provider: biz.AMDGPUDevice, NodeName: "amd-node", Devmem: 16304, Devcore: 32, Count: 2,
	}
	uuid := device.Id
	generator := newDeviceMetricsTestGenerator(device, map[string]*pb.InstantResponse{
		`hami_host_gpu_power_usage_watts{device_uuid="` + uuid + `"}`:        {Data: []*pb.Sample{{Metric: map[string]string{"device_index": "1"}, Value: 13}}},
		`avg(hami_host_gpu_memory_total_bytes{device_uuid="` + uuid + `"})`:  instantValue(16304 * 1024 * 1024),
		`avg(hami_host_gpu_memory_used_bytes{device_uuid="` + uuid + `"})`:   instantValue(2038 * 1024 * 1024),
		`avg(hami_host_gpu_utilization_ratio{device_uuid="` + uuid + `"})`:   instantValue(37),
		`avg(hami_host_gpu_temperature_celsius{device_uuid="` + uuid + `"})`: instantValue(42),
		`avg(hami_host_gpu_power_usage_watts{device_uuid="` + uuid + `"})`:   instantValue(13),
	})
	t.Cleanup(func() { deleteTrackedTestCells(generator) })
	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	labels := []string{device.NodeName, "AMD", device.Type, device.Id, "", "amd-1"}
	for gauge, want := range map[*prometheus.GaugeVec]float64{
		HamiVgpuCount: 2, HamiMemorySize: 16304, HamiMemoryUsed: 2038,
		HamiCoreUtil: 37, HamiDeviceTemperature: 42, HamiDevicePower: 13,
	} {
		assertTrackedGaugeValue(t, generator, gauge, labels, want)
	}
}

func TestAMDHasNoPerContainerUtilization(t *testing.T) {
	generator := newDeviceMetricsTestGenerator(&biz.DeviceInfo{Id: "u", Provider: biz.AMDGPUDevice}, nil)
	_, err := generator.taskCoreUsed(context.Background(), biz.AMDGPUDevice, "ns", "pod", "ctr", "", "u", "node", 0)
	if err != errWorkloadTelemetryUnsupported {
		t.Fatalf("taskCoreUsed = %v, want errWorkloadTelemetryUnsupported", err)
	}
	if _, err := generator.memoryTemperature(context.Background(), biz.AMDGPUDevice, "u"); err == nil {
		t.Fatal("AMD memory temperature should be unsupported")
	}
}
