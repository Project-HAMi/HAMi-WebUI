package biz

import "testing"

func allocatedContainer(pod, name, kind string, idx int, devices ...ContainerDevice) *Container {
	return &Container{PodUID: pod, Name: name, Kind: kind, ContainerIdx: idx, ContainerDevices: devices}
}

func gpuShare(uuid string, memory, cores int32) ContainerDevice {
	return ContainerDevice{UUID: uuid, Type: "NVIDIA", Usedmem: memory, Usedcores: cores}
}

func TestContainersStatisticsInfoFollowsHAMiPodAccounting(t *testing.T) {
	tests := []struct {
		name               string
		deviceID           string
		containers         []*Container
		vGPU, core, memory int32
	}{
		{name: "app containers add up", containers: []*Container{
			allocatedContainer("p", "a", ContainerKindRegular, 0, gpuShare("GPU-1", 1024, 10)),
			allocatedContainer("p", "b", ContainerKindRegular, 1, gpuShare("GPU-1", 512, 20)),
		}, vGPU: 2, core: 30, memory: 1536},
		{name: "init counts at its peak per dimension", containers: []*Container{
			allocatedContainer("p", "prepare", ContainerKindInit, 0, gpuShare("GPU-1", 4096, 10)),
			allocatedContainer("p", "main", ContainerKindRegular, 1, gpuShare("GPU-1", 1024, 30)),
		}, vGPU: 1, core: 30, memory: 4096},
		{name: "sidecar runs alongside app containers", containers: []*Container{
			allocatedContainer("p", "proxy", ContainerKindSidecar, 0, gpuShare("GPU-1", 128, 2)),
			allocatedContainer("p", "main", ContainerKindRegular, 1, gpuShare("GPU-1", 384, 5)),
		}, vGPU: 2, core: 7, memory: 512},
		// The two orderings from HAMi's Test_calcScore_SidecarInitOrdering.
		{name: "init before a sidecar does not overlap it", containers: []*Container{
			allocatedContainer("p", "prepare", ContainerKindInit, 0, gpuShare("GPU-1", 5000, 0)),
			allocatedContainer("p", "proxy", ContainerKindSidecar, 1, gpuShare("GPU-1", 8000, 0)),
		}, vGPU: 1, memory: 8000},
		{name: "init after a sidecar overlaps it, whatever the input order", containers: []*Container{
			allocatedContainer("p", "prepare", ContainerKindInit, 1, gpuShare("GPU-1", 5000, 0)),
			allocatedContainer("p", "proxy", ContainerKindSidecar, 0, gpuShare("GPU-1", 8000, 0)),
		}, vGPU: 2, memory: 13000},
		{name: "pods and devices are accounted separately", containers: []*Container{
			allocatedContainer("p1", "prepare", ContainerKindInit, 0, gpuShare("GPU-2", 4096, 10)),
			allocatedContainer("p1", "main", ContainerKindRegular, 1, gpuShare("GPU-1", 1024, 20)),
			allocatedContainer("p2", "prepare", ContainerKindInit, 0, gpuShare("GPU-1", 2048, 10)),
			allocatedContainer("p2", "main", ContainerKindRegular, 1, gpuShare("GPU-1", 1024, 20)),
		}, vGPU: 3, core: 50, memory: 7168},
		{name: "device filter", deviceID: "GPU-2", containers: []*Container{
			allocatedContainer("p1", "prepare", ContainerKindInit, 0, gpuShare("GPU-2", 4096, 10)),
			allocatedContainer("p1", "main", ContainerKindRegular, 1, gpuShare("GPU-1", 1024, 20)),
		}, vGPU: 1, core: 10, memory: 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vGPU, core, memory, known := ContainersStatisticsInfo(tt.containers, tt.deviceID)
			if vGPU != tt.vGPU || core != tt.core || memory != tt.memory || !known {
				t.Fatalf("got %d slots, %d cores, %d MiB (known %v); want %d, %d, %d", vGPU, core, memory, known, tt.vGPU, tt.core, tt.memory)
			}
		})
	}
}
