package biz

import "testing"

func TestContainersStatisticsInfoKeepsNumberedDevicesApart(t *testing.T) {
	for _, prefix := range []string{"n-cambricon-mlu-", "n-metax-"} {
		containers := []*Container{
			{ContainerDevices: ContainerDevices{{UUID: prefix + "1", Usedcores: 30, Usedmem: 1024}}},
			{ContainerDevices: ContainerDevices{{UUID: prefix + "10", Usedcores: 100, Usedmem: 81920}}},
		}
		for _, tc := range []struct {
			device             string
			vgpu, core, memory int32
		}{
			{prefix + "1", 1, 30, 1024},
			{prefix + "10", 1, 100, 81920},
		} {
			vgpu, core, memory, _ := ContainersStatisticsInfo(containers, tc.device)
			if vgpu != tc.vgpu || core != tc.core || memory != tc.memory {
				t.Fatalf("%s: vgpu=%d core=%d memory=%d, want %d %d %d", tc.device, vgpu, core, memory, tc.vgpu, tc.core, tc.memory)
			}
		}
	}
}

func TestContainersStatisticsInfoMatchesNothingForAnEmptyID(t *testing.T) {
	containers := []*Container{
		{ContainerDevices: ContainerDevices{{UUID: "GPU-A", Usedcores: 30, Usedmem: 1024}}},
		{ContainerDevices: ContainerDevices{{UUID: "", Usedcores: 50, Usedmem: 2048}}},
	}
	if vgpu, core, memory, known := ContainersStatisticsInfo(containers, ""); vgpu != 0 || core != 0 || memory != 0 || known {
		t.Fatalf("empty ID: vgpu=%d core=%d memory=%d known=%v, want nothing", vgpu, core, memory, known)
	}
}
