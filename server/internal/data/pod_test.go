package data

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

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

func TestListWholeGPUContainersKeepsPerContainerStatus(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "whole", Namespace: "ns", UID: k8stypes.UID("uid-1")},
		Spec: corev1.PodSpec{
			NodeName:   "node-a",
			Containers: []corev1.Container{{Name: "serving"}, {Name: "sidecar"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "serving", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "sidecar", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			},
		},
	}
	repo := &podRepo{wholeGPUPods: map[k8stypes.UID]*wholeGPUPod{
		pod.UID: {
			pod:      pod,
			nodeName: "node-a",
			nodeUID:  "nuid-1",
			memMiB:   81920,
			ctrs: []wholeGPUContainer{
				{name: "serving", cards: []int64{0}},
				{name: "sidecar", cards: []int64{1}},
			},
		},
	}}

	got := map[string]*biz.Container{}
	for _, c := range repo.listWholeGPUContainers() {
		got[c.Name] = c
	}
	if len(got) != 2 {
		t.Fatalf("containers = %#v", got)
	}
	if got["serving"].Status != biz.ContainerStatusSuccess {
		t.Fatalf("serving status = %q, want %q", got["serving"].Status, biz.ContainerStatusSuccess)
	}
	if got["sidecar"].Status != biz.ContainerStatusError {
		t.Fatalf("sidecar status = %q, want %q", got["sidecar"].Status, biz.ContainerStatusError)
	}
	if got["sidecar"].StatusDetail == nil || got["sidecar"].StatusDetail.Reason != "CrashLoopBackOff" {
		t.Fatalf("sidecar detail = %#v", got["sidecar"].StatusDetail)
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
