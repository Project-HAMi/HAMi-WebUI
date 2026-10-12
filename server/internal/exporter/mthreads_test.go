package exporter

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/provider/mthreads"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
)

// mthreadsDevice builds a Moore Threads device whose DCGM selector is
// gpu=<index>, which is what the whole-card ordinal maps onto.
func mthreadsDevice(id, typ string, index int, count, devmem int32) *biz.DeviceInfo {
	return &biz.DeviceInfo{
		Id:       id,
		AliasId:  id,
		Provider: biz.MthreadsGPUDevice,
		Type:     typ,
		NodeName: "node-a",
		Index:    index,
		Count:    count,
		Devmem:   devmem,
		Devcore:  100,
	}
}

// dcgmQueries returns a response for every series the generator reads, so a
// test can override just the ones it cares about.
func dcgmQueries(node string, index int, overrides map[string]float32) map[string]*pb.InstantResponse {
	out := map[string]*pb.InstantResponse{}
	for _, metric := range []string{"GPU_TEMP", "MEMORY_TEMP", "POWER_USAGE", "GPU_UTIL", "FB_USED", "FB_FREE"} {
		value := float32(1)
		if v, ok := overrides[metric]; ok {
			value = v
		}
		q := fmt.Sprintf(`avg(DCGM_FI_DEV_%s{Hostname=%q, gpu="%d"})`, metric, node, index)
		out[q] = instantValue(value)
	}
	return out
}

func mthreadsGeneratorFor(t *testing.T, device *biz.DeviceInfo, responses map[string]*pb.InstantResponse) (*MetricsGenerator, *fakeInstantQuerier) {
	t.Helper()
	querier := &fakeInstantQuerier{responsesByQuery: responses}
	generator := &MetricsGenerator{
		nodeUsecase: biz.NewNodeUsecase(&fakeNodeRepo{devices: []*biz.DeviceInfo{device}},
			log.NewStdLogger(io.Discard)),
		monitorService: querier,
		log:            log.NewHelper(log.NewStdLogger(io.Discard)),
	}
	t.Cleanup(func() { deleteTrackedTestCells(generator) })
	return generator, querier
}

// Whole cards keep their physical ordinal in DeviceInfo.Index, so the DCGM
// query must select gpu=<index> rather than restarting at zero and colliding
// with the sliced-card series.
func TestMthreadsDeviceMetricsUsePhysicalCardIndex(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-full-3", mthreads.MthreadsWholeGPUType, 3, 1, 81920)
	generator, querier := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 3, nil))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	if len(querier.queries) == 0 {
		t.Fatal("no DCGM queries issued for a Moore Threads device")
	}
	for _, q := range querier.queries {
		if !strings.Contains(q, `Hostname="node-a", gpu="3"`) {
			t.Fatalf("query %q does not select gpu=3 for the whole card", q)
		}
	}
}

// A fully allocated card reports FB_FREE=0; the memory series must still be
// emitted instead of being dropped and pruned on the next cycle.
func TestMthreadsDeviceMetricsEmitsMemoryWhenFramebufferIsFull(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, map[string]float32{
		"FB_USED": 81920,
		"FB_FREE": 0,
	}))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}

	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	for _, gauge := range []*prometheus.GaugeVec{HamiMemoryUsed, HamiMemorySize, HamiMemoryUtil} {
		assertGaugeTracked(t, generator, gauge, labels, true)
	}
	assertTrackedGaugeValue(t, generator, HamiMemoryUsed, labels, 81920)
	assertTrackedGaugeValue(t, generator, HamiMemorySize, labels, 81920)
	assertTrackedGaugeValue(t, generator, HamiMemoryUtil, labels, 100)
}

// An empty framebuffer must not divide by zero when computing utilization.
func TestMthreadsDeviceMetricsSkipsUtilWhenFramebufferEmpty(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, map[string]float32{
		"FB_USED": 0,
		"FB_FREE": 0,
	}))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	assertGaugeTracked(t, generator, HamiMemoryUtil, labels, false)
}

// Capacity gauges are provider-agnostic and must be emitted for Moore Threads
// too, otherwise the card-detail charts lose their totals.
func TestMthreadsDeviceMetricsEmitCapacityGauges(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, nil))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	assertTrackedGaugeValue(t, generator, HamiVgpuCount, labels, 100)
	assertTrackedGaugeValue(t, generator, HamiVmemorySize, labels, 81920)
	assertTrackedGaugeValue(t, generator, HamiVcoreSize, labels, 100)
}

// The provider returns before the shared block that derives
// hami_vmemory_scaling, so it must emit schedulable/physical itself.
func TestMthreadsDeviceMetricsEmitMemoryScaling(t *testing.T) {
	// 40960 MiB schedulable out of an 81920 MiB framebuffer => 0.5.
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 40960)
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, map[string]float32{
		"FB_USED": 40960,
		"FB_FREE": 40960,
	}))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	assertTrackedGaugeValue(t, generator, HamiVMemoryScaling, labels, 0.5)
}

// An empty framebuffer yields no physical size, so no scaling ratio exists.
func TestMthreadsDeviceMetricsSkipsMemoryScalingWithoutFramebuffer(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, map[string]float32{
		"FB_USED": 0,
		"FB_FREE": 0,
	}))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	assertGaugeTracked(t, generator, HamiVMemoryScaling, labels, false)
}

// HAMi does not schedule unconfigured devices, so they contribute no
// schedulable capacity and must not report a scaling ratio.
func TestMthreadsDeviceMetricsSkipMemoryScalingForUnconfigured(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	device.Unconfigured = true
	generator, _ := mthreadsGeneratorFor(t, device, dcgmQueries("node-a", 0, map[string]float32{
		"FB_USED": 40960,
		"FB_FREE": 40960,
	}))

	if err := generator.GenerateDeviceMetrics(context.Background()); err != nil {
		t.Fatalf("GenerateDeviceMetrics() error = %v", err)
	}
	labels := []string{"node-a", biz.MthreadsGPUDevice, biz.MthreadsGPUDevice, device.Id, "", ""}
	assertGaugeTracked(t, generator, HamiVMemoryScaling, labels, false)
	assertGaugeTracked(t, generator, HamiVgpuCount, labels, false)
}

// Missing vendor samples must mark the cycle degraded; otherwise the commit
// would prune the series while reporting success.
func TestMthreadsDeviceMetricsReportMissingSamples(t *testing.T) {
	device := mthreadsDevice("node-a-mthreads-0", biz.MthreadsGPUDevice, 0, 100, 81920)
	generator, _ := mthreadsGeneratorFor(t, device, map[string]*pb.InstantResponse{})

	err := generator.GenerateDeviceMetrics(context.Background())
	if err == nil {
		t.Fatal("GenerateDeviceMetrics() = nil, want a degraded refresh error")
	}
	if !strings.Contains(err.Error(), "no data") {
		t.Fatalf("error = %v, want a missing-sample error", err)
	}
}
