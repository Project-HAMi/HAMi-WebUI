package data

import (
	"context"
	"fmt"
	"testing"

	"vgpu/internal/biz"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
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

func useNvidiaAllocationKey(t *testing.T) string {
	t.Helper()
	const key = "hami.io/vgpu-devices-allocated"
	previous, existed := util.SupportDevices["NVIDIA"]
	util.SupportDevices["NVIDIA"] = key
	t.Cleanup(func() {
		if existed {
			util.SupportDevices["NVIDIA"] = previous
		} else {
			delete(util.SupportDevices, "NVIDIA")
		}
	})
	return key
}

func TestInitAndSidecarAllocationsFollowHAMiSlotsAndRelease(t *testing.T) {
	key := useNvidiaAllocationKey(t)
	always := corev1.ContainerRestartPolicyAlways
	running := corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}
	succeeded := corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}
	repo := &podRepo{pods: map[k8stypes.UID]*biz.PodInfo{}, log: log.NewHelper(log.DefaultLogger)}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "train", UID: "pod-1", Annotations: map[string]string{
			util.AssignedNodeAnnotations: "node-1",
			key:                          "GPU-1,NVIDIA,1024,10:;GPU-1,NVIDIA,256,5:;;GPU-1,NVIDIA,512,20:;",
		}},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "prepare"}, {Name: "proxy", RestartPolicy: &always}, {Name: "fetch"}},
			Containers:     []corev1.Container{{Name: "main"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending, InitContainerStatuses: []corev1.ContainerStatus{
			{Name: "prepare", State: running}, {Name: "proxy", Ready: true, State: running}, {Name: "fetch"},
		}},
	}
	describe := func() string {
		containers, _ := repo.ListAll(context.Background())
		vGPU, cores, memory, _ := biz.ContainersStatisticsInfo(containers, "")
		result := ""
		for _, c := range containers {
			result += fmt.Sprintf("%s/%s/%d/%d/%s ", c.Name, c.Kind, c.ContainerIdx, c.ContainerDevices[0].Usedmem, c.Status)
		}
		return result + fmt.Sprintf("= %d slots, %d cores, %d MiB", vGPU, cores, memory)
	}

	repo.onAddPod(pod)
	want := "prepare/init/0/1024/success proxy/sidecar/1/256/success main/regular/3/512/waiting = 2 slots, 25 cores, 1024 MiB"
	if got := describe(); got != want {
		t.Fatalf("init phase:\n got %s\nwant %s", got, want)
	}

	released := pod.DeepCopy()
	released.Status.Phase = corev1.PodRunning
	released.Status.InitContainerStatuses = []corev1.ContainerStatus{
		{Name: "prepare", State: succeeded}, {Name: "proxy", Ready: true, State: running}, {Name: "fetch", State: succeeded},
	}
	released.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "main", Ready: true, State: running}}
	repo.onUpdatePod(pod, released)
	want = "proxy/sidecar/1/256/success main/regular/3/512/success = 2 slots, 25 cores, 768 MiB"
	if got := describe(); got != want {
		t.Fatalf("after every regular init succeeded:\n got %s\nwant %s", got, want)
	}
}
