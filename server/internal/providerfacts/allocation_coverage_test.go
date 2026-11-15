package providerfacts

import (
	"testing"

	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

func TestEmptySlotIsCheckedAgainstItsOwnRequest(t *testing.T) {
	node := testNode()
	const nvidia = "hami_annotation:NVIDIA"
	const ascend = "hami_annotation:Ascend910B3"
	sources := nodeAllocationSources{nvidia: {"nvidia.com/gpu": true}, ascend: {"huawei.com/npu": true}}
	device := resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: "NVIDIA", Node: node.Name, NodeUID: node.UID}}

	for _, tc := range []struct {
		name, resource string
		container      int
		released, gap  bool
	}{
		{"request with sibling allocation", "nvidia.com/gpu", 2, false, true},
		{"CPU sibling", "", 2, false, false},
		{"other allocator sibling", "huawei.com/npu", 2, false, false},
		{"init before release", "nvidia.com/gpu", 1, false, true},
		{"init after release", "nvidia.com/gpu", 1, true, false},
		{"restartable init after release", "nvidia.com/gpu", 0, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := allocationInventory(t, node).Entries[0].Facts
			facts.InitUsageReleased = tc.released
			slot := &facts.Containers[tc.container]
			slot.Allocations[0].Records = nil
			if tc.resource != "" {
				slot.Requests = []podfacts.Request{{Resource: tc.resource, Positive: true}}
			}
			issue := allocationSourceIssue(device, map[string]bool{"GPU-a": true}, sources, []*podfacts.Facts{facts})
			if (issue != "") != tc.gap {
				t.Fatalf("coverage issue %q, want gap %v", issue, tc.gap)
			}
		})
	}
}
