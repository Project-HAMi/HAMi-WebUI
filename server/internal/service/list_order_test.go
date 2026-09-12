package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

func TestGetAllContainersOrderIsIndependentOfRepositoryOrder(t *testing.T) {
	containers := []*biz.Container{
		{Name: "worker", PodName: "duplicate", Namespace: "team-a", PodUID: "duplicate-z", Status: biz.ContainerStatusSuccess},
		{Name: "main", PodName: "alpha", Namespace: "team-b", PodUID: "other-team", Status: biz.ContainerStatusSuccess},
		{Name: "worker", PodName: "z-failed", Namespace: "team-z", PodUID: "failed", Status: biz.ContainerStatusFailed},
		{Name: "sidecar", PodName: "alpha", Namespace: "team-a", PodUID: "alpha", Status: biz.ContainerStatusSuccess},
		{Name: "worker", PodName: "a-completed", Namespace: "team-a", PodUID: "completed", Status: biz.ContainerStatusClosed},
		{Name: "main", PodName: "zeta", Namespace: "team-a", PodUID: "zeta", Status: biz.ContainerStatusSuccess},
		{Name: "worker", PodName: "z-unknown", Namespace: "team-z", PodUID: "unknown", Status: biz.ContainerStatusUnknown},
		{Name: "worker", PodName: "duplicate", Namespace: "team-a", PodUID: "duplicate-a", Status: biz.ContainerStatusSuccess},
		{Name: "main", PodName: "alpha", Namespace: "team-a", PodUID: "alpha", Status: biz.ContainerStatusSuccess},
		{Name: "main", PodName: "duplicate", Namespace: "team-a", PodUID: "duplicate-z", Status: biz.ContainerStatusSuccess},
	}
	for _, container := range containers {
		container.CreateTime = time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
		container.ContainerDevices = biz.ContainerDevices{{UUID: "GPU-1"}}
	}
	repo := &containerTestPodRepo{}
	service := NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(repo, log.DefaultLogger),
	)
	tests := []struct {
		name    string
		filters *pb.GetAllContainersReq_Filters
		want    []string
	}{
		{
			name: "all statuses retain their priority before workload names",
			want: []string{
				"failed/worker", "unknown/worker", "alpha/main", "alpha/sidecar",
				"duplicate-z/main", "duplicate-a/worker", "duplicate-z/worker", "zeta/main", "other-team/main", "completed/worker",
			},
		},
		{
			name:    "same status and timestamp use namespace pod container and UID",
			filters: &pb.GetAllContainersReq_Filters{Status: biz.ContainerStatusSuccess},
			want:    []string{"alpha/main", "alpha/sidecar", "duplicate-z/main", "duplicate-a/worker", "duplicate-z/worker", "zeta/main", "other-team/main"},
		},
		{
			name:    "pod name filter retains the matching subset order",
			filters: &pb.GetAllContainersReq_Filters{Name: "alpha"},
			want:    []string{"alpha/main", "alpha/sidecar", "other-team/main"},
		},
		{
			name:    "container name filter retains status and namespace priority",
			filters: &pb.GetAllContainersReq_Filters{Name: "worker"},
			want:    []string{"failed/worker", "unknown/worker", "duplicate-a/worker", "duplicate-z/worker", "completed/worker"},
		},
	}
	for inputName, input := range listOrderInputs(containers) {
		t.Run(inputName, func(t *testing.T) {
			repo.containers = input
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{Filters: tt.filters})
					if err != nil {
						t.Fatalf("GetAllContainers: %v", err)
					}
					got := make([]string, 0, len(reply.Items))
					for _, item := range reply.Items {
						got = append(got, item.PodUid+"/"+item.Name)
					}
					if !slices.Equal(got, tt.want) {
						t.Fatalf("workload order = %v, want %v", got, tt.want)
					}
				})
			}
		})
	}
}

func TestGetAllGPUTypesOrderIsIndependentOfRepositoryOrder(t *testing.T) {
	devices := []*biz.DeviceInfo{
		{Type: "NVIDIA-H100", Provider: biz.NvidiaGPUDevice},
		{Type: "NVIDIA-A10", Provider: biz.NvidiaGPUDevice},
		{Type: "Ascend-910B", Provider: biz.AscendGPUDevice},
		{Type: "NVIDIA-A10", Provider: biz.NvidiaGPUDevice},
	}
	repo := &capacityTestNodeRepo{}
	service := NewCardService(biz.NewNodeUsecase(repo, log.DefaultLogger), nil)
	tests := []struct {
		name    string
		filters *pb.GetAllGpusReq_Filters
		want    []string
	}{
		{name: "all unique types", want: []string{"Ascend-910B", "NVIDIA-A10", "NVIDIA-H100"}},
		{
			name:    "provider filter retains the matching subset order",
			filters: &pb.GetAllGpusReq_Filters{Provider: biz.NvidiaGPUDevice},
			want:    []string{"NVIDIA-A10", "NVIDIA-H100"},
		},
		{
			name:    "no matching provider",
			filters: &pb.GetAllGpusReq_Filters{Provider: "missing-provider"},
			want:    []string{},
		},
	}
	for inputName, input := range listOrderInputs(devices) {
		t.Run(inputName, func(t *testing.T) {
			repo.devices = input
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					reply, err := service.GetAllGPUTypes(context.Background(), &pb.GetAllGpusReq{Filters: tt.filters})
					if err != nil {
						t.Fatalf("GetAllGPUTypes: %v", err)
					}
					got := make([]string, 0, len(reply.List))
					for _, item := range reply.List {
						got = append(got, item.Type)
					}
					if !slices.Equal(got, tt.want) {
						t.Fatalf("GPU type order = %v, want %v", got, tt.want)
					}
				})
			}
		})
	}
}

func listOrderInputs[T any](items []T) map[string][]T {
	reversed := slices.Clone(items)
	slices.Reverse(reversed)
	return map[string][]T{
		"original": items,
		"reversed": reversed,
		"rotated":  append(slices.Clone(items[1:]), items[0]),
	}
}
