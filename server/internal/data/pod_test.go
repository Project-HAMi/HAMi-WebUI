package data

import (
	"testing"

	"vgpu/internal/biz"
	"vgpu/internal/provider/util"
)

func TestMergeContainerDevicesBySlotKeepsInitAlignmentAndDeviceTypes(t *testing.T) {
	podDevices := biz.PodDevices{
		"Ascend910B3": {{}, {{UUID: "B3-0", Type: "Ascend910B3", Usedmem: 28672, Usedcores: 40, CoreAllocationKnown: true}}, {}},
		"NVIDIA":      {{}, {}, {{UUID: "GPU-0", Type: "NVIDIA", Usedmem: 4096, Usedcores: 20}}},
	}
	got := mergeContainerDevicesBySlot(3, podDevices)
	if len(got) != 3 || len(got[0]) != 0 || len(got[1]) != 1 || len(got[2]) != 1 {
		t.Fatalf("merged slots = %#v", got)
	}
	if got[1][0].Type != "Ascend910B3" || got[2][0].Type != "NVIDIA" {
		t.Fatalf("device types moved or overwrote one another: %#v", got)
	}
}

func TestResolveAscendAllocationModeUsesPodThenNodeContract(t *testing.T) {
	tests := []struct {
		name, podMode, nodeHamiCore string
		want                        util.AscendAllocationMode
	}{
		{name: "explicit hami-core", podMode: util.AscendVNPUModeHamiCore, nodeHamiCore: "false", want: util.AscendAllocationModeHamiCore},
		{name: "explicit template", podMode: "template", nodeHamiCore: "true", want: util.AscendAllocationModeTemplate},
		{name: "annotation-less true node is ambiguous across deployed plugin versions", nodeHamiCore: "true", want: util.AscendAllocationModeUnknown},
		{name: "annotation-less hard node", nodeHamiCore: "false", want: util.AscendAllocationModeTemplate},
		{name: "unresolved node mode", want: util.AscendAllocationModeUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveAscendAllocationMode(tt.podMode, tt.nodeHamiCore); got != tt.want {
				t.Fatalf("resolveAscendAllocationMode(%q, %q) = %v, want %v", tt.podMode, tt.nodeHamiCore, got, tt.want)
			}
		})
	}
}
