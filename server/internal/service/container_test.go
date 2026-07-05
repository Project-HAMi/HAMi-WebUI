package service

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

// mockPodRepo implements biz.PodRepo.
type mockPodRepo struct {
	containers []*biz.Container
	err        error
}

func (m *mockPodRepo) ListAll(_ context.Context) ([]*biz.Container, error) {
	return m.containers, m.err
}

func (m *mockPodRepo) FindOne(_ context.Context, podUID string, name string) (*biz.Container, error) {
	for _, c := range m.containers {
		if c.PodUID == podUID && c.Name == name {
			return c, nil
		}
	}
	return nil, m.err
}

// mockNodeRepo implements biz.NodeRepo.
type mockNodeRepo struct{}

func (m *mockNodeRepo) ListAll(_ context.Context) ([]*biz.Node, error) {
	return nil, nil
}

func (m *mockNodeRepo) GetNode(_ context.Context, _ string) (*biz.Node, error) {
	return nil, nil
}

func (m *mockNodeRepo) ListAllDevices(_ context.Context) ([]*biz.DeviceInfo, error) {
	return nil, nil
}

func (m *mockNodeRepo) FindDeviceByAliasId(_ string) (*biz.DeviceInfo, error) {
	return nil, io.ErrUnexpectedEOF // return error so the service falls through to raw UUID
}

func TestContainerService_GetAllContainers_Labels(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	mockPod := &mockPodRepo{
		containers: []*biz.Container{
			{
				Name:      "ctr-1",
				PodUID:    "pod-1",
				PodName:   "my-app",
				Status:    biz.ContainerStatusSuccess,
				Namespace: "default",
				NodeName:  "node-1",
				NodeUID:   "node-uid-1",
				Priority:  "1",
				Image:     "nginx:latest",
				ContainerDevices: biz.ContainerDevices{
					{UUID: "gpu-uuid-1", Type: "NVIDIA", Usedmem: 1000, Usedcores: 10},
				},
				Labels: map[string]string{
					"app":     "test-app",
					"version": "v1",
				},
			},
		},
	}

	mockNode := &mockNodeRepo{}

	svc := NewContainerService(biz.NewNodeUsecase(mockNode, logger), biz.NewPodUseCase(mockPod, logger))

	res, err := svc.GetAllContainers(context.Background(), &pb.GetAllContainersReq{
		Filters: &pb.GetAllContainersReq_Filters{},
	})
	if err != nil {
		t.Fatalf("GetAllContainers failed: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	ctr := res.Items[0]
	labels := ctr.Labels
	if labels == nil {
		t.Fatal("expected Labels to be non-nil")
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if v, ok := labels["app"]; !ok || v != "test-app" {
		t.Fatalf(`expected labels["app"] == "test-app", got %q`, v)
	}
	if v, ok := labels["version"]; !ok || v != "v1" {
		t.Fatalf(`expected labels["version"] == "v1", got %q`, v)
	}

	// Verify the rest of the mapping still works
	if len(ctr.DeviceIds) != 1 || ctr.DeviceIds[0] != "gpu-uuid-1" {
		t.Fatalf("expected DeviceIds [gpu-uuid-1], got %v", ctr.DeviceIds)
	}
}

func TestContainerService_GetContainer_Labels(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	mockPod := &mockPodRepo{
		containers: []*biz.Container{
			{
				Name:      "ctr-1",
				PodUID:    "pod-1",
				PodName:   "my-app",
				Status:    biz.ContainerStatusSuccess,
				Namespace: "default",
				NodeName:  "node-1",
				NodeUID:   "node-uid-1",
				Priority:  "1",
				Image:     "nginx:latest",
				CreateTime: mustParseTime("2025-01-01T00:00:00Z"),
				ContainerDevices: biz.ContainerDevices{
					{UUID: "gpu-uuid-1", Type: "NVIDIA", Usedmem: 1000, Usedcores: 10},
				},
				Labels: map[string]string{
					"app":     "test-app",
					"version": "v1",
				},
			},
		},
	}

	mockNode := &mockNodeRepo{}

	svc := NewContainerService(biz.NewNodeUsecase(mockNode, logger), biz.NewPodUseCase(mockPod, logger))

	ctrReply, err := svc.GetContainer(context.Background(), &pb.GetContainerReq{
		PodUid: "pod-1",
		Name:   "ctr-1",
	})
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}

	labels := ctrReply.Labels
	if labels == nil {
		t.Fatal("expected Labels to be non-nil")
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if v, ok := labels["app"]; !ok || v != "test-app" {
		t.Fatalf(`expected labels["app"] == "test-app", got %q`, v)
	}
	if v, ok := labels["version"]; !ok || v != "v1" {
		t.Fatalf(`expected labels["version"] == "v1", got %q`, v)
	}

	if len(ctrReply.DeviceIds) != 1 || ctrReply.DeviceIds[0] != "gpu-uuid-1" {
		t.Fatalf("expected DeviceIds [gpu-uuid-1], got %v", ctrReply.DeviceIds)
	}
}

func TestContainerService_GetAllContainers_NilLabels(t *testing.T) {
	logger := log.NewStdLogger(io.Discard)

	mockPod := &mockPodRepo{
		containers: []*biz.Container{
			{
				Name:      "ctr-nil-labels",
				PodUID:    "pod-2",
				PodName:   "no-labels-app",
				Status:    biz.ContainerStatusSuccess,
				Namespace: "default",
				NodeName:  "node-1",
				NodeUID:   "node-uid-1",
				Priority:  "0",
				Image:     "alpine:latest",
				ContainerDevices: biz.ContainerDevices{
					{UUID: "gpu-uuid-2", Type: "NVIDIA", Usedmem: 500, Usedcores: 5},
				},
				Labels: nil,
			},
		},
	}

	mockNode := &mockNodeRepo{}

	svc := NewContainerService(biz.NewNodeUsecase(mockNode, logger), biz.NewPodUseCase(mockPod, logger))

	res, err := svc.GetAllContainers(context.Background(), &pb.GetAllContainersReq{
		Filters: &pb.GetAllContainersReq_Filters{},
	})
	if err != nil {
		t.Fatalf("GetAllContainers failed: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	ctr := res.Items[0]
	if ctr.Labels != nil {
		t.Fatal("expected Labels to be nil for a container with nil labels")
	}
}

// mustParseTime is a test helper for parsing RFC3339 timestamps.
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
