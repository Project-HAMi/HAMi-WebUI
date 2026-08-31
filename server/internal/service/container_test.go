package service

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/encoding/protojson"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

func TestMatchesWorkloadName(t *testing.T) {
	tests := []struct {
		name    string
		podName string
		filter  string
		want    bool
	}{
		{
			name:    "empty filter matches all",
			podName: "demo-workload-abc",
			want:    true,
		},
		{
			name:    "partial pod name matches",
			podName: "demo-workload-abc",
			filter:  "demo-workload",
			want:    true,
		},
		{
			name:    "unrelated name does not match",
			podName: "demo-workload-abc",
			filter:  "worker",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesWorkloadName(tt.podName, "inference-container", tt.filter); got != tt.want {
				t.Fatalf("matchesWorkloadName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAllContainersMatchesPodOrContainerName(t *testing.T) {
	containers := []*biz.Container{
		{
			Name:     "inference-container",
			PodName:  "training-job-abc",
			Status:   biz.ContainerStatusSuccess,
			PodUID:   "pod-1",
			NodeName: "node-1",
			ContainerDevices: biz.ContainerDevices{
				{UUID: "device-1"},
			},
		},
	}
	service := NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&containerTestPodRepo{containers: containers}, log.DefaultLogger),
	)

	for _, filter := range []string{"training-job", "inference-container"} {
		reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{
			Filters: &pb.GetAllContainersReq_Filters{Name: filter},
		})
		if err != nil {
			t.Fatalf("GetAllContainers(%q): %v", filter, err)
		}
		if len(reply.Items) != 1 {
			t.Fatalf("GetAllContainers(%q) returned %d items, want 1", filter, len(reply.Items))
		}
	}
}

func TestGetAllContainersMarksUnknownAscendCoreAllocation(t *testing.T) {
	containers := []*biz.Container{{
		Name: "worker", PodName: "ascend-job", Status: biz.ContainerStatusSuccess, PodUID: "pod-1", NodeName: "node-1",
		ContainerDevices: biz.ContainerDevices{{UUID: "ascend-0", Type: "Ascend910B4", Usedmem: 4096, CoreAllocationKnown: false}},
	}}
	service := NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&containerTestPodRepo{containers: containers}, log.DefaultLogger),
	)

	reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil {
		t.Fatalf("GetAllContainers() error = %v", err)
	}
	if len(reply.Items) != 1 || reply.Items[0].AllocatedCoresKnown == nil || reply.Items[0].GetAllocatedCoresKnown() {
		t.Fatalf("allocated core presence = %#v, want explicit unknown", reply.Items)
	}
	if reply.Items[0].AllocatedMem != 4096 || len(reply.Items[0].DeviceIds) != 1 {
		t.Fatalf("unknown core must preserve device and memory allocation: %#v", reply.Items[0])
	}
	encoded, err := protojson.Marshal(reply.Items[0])
	if err != nil {
		t.Fatalf("marshal reply: %v", err)
	}
	if !strings.Contains(string(encoded), `"allocatedCoresKnown":false`) {
		t.Fatalf("optional false presence was lost in JSON: %s", encoded)
	}
}

type containerTestPodRepo struct {
	containers []*biz.Container
}

func (r *containerTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return r.containers, nil
}

func (r *containerTestPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}

type containerTestNodeRepo struct{}

func (r *containerTestNodeRepo) ListAll(context.Context) ([]*biz.Node, error) {
	return nil, nil
}

func (r *containerTestNodeRepo) GetNode(context.Context, string) (*biz.Node, error) {
	return nil, nil
}

func (r *containerTestNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return nil, nil
}

func (r *containerTestNodeRepo) FindDeviceByAliasId(aliasID string) (*biz.DeviceInfo, error) {
	return &biz.DeviceInfo{Id: aliasID, AliasId: aliasID}, nil
}
