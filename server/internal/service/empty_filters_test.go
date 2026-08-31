package service

import (
	"context"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

func TestRequestsTreatMissingFiltersAsEmpty(t *testing.T) {
	device := &biz.DeviceInfo{
		Id:       "GPU-1",
		AliasId:  "GPU-1",
		NodeName: "node-a",
		NodeUid:  "node-uid-a",
	}
	nodeRepo := &capacityTestNodeRepo{
		nodes: []*biz.Node{{
			Name:    "node-a",
			Uid:     "node-uid-a",
			Devices: []*biz.DeviceInfo{device},
		}},
		devices: []*biz.DeviceInfo{device},
	}
	podRepo := &emptyFiltersPodRepo{containers: []*biz.Container{{
		Name:      "worker",
		PodName:   "job-a",
		NodeName:  "node-a",
		NodeUID:   "node-uid-a",
		Namespace: "default",
	}}}
	nodeUsecase := biz.NewNodeUsecase(nodeRepo, log.DefaultLogger)
	podUsecase := biz.NewPodUseCase(podRepo, log.DefaultLogger)
	nodes := NewNodeService(
		nodeUsecase,
		podUsecase,
		biz.NewSummaryUseCase(nodeRepo, podRepo, log.DefaultLogger),
	)
	cards := NewCardService(nodeUsecase, podUsecase)
	containers := NewContainerService(nodeUsecase, podUsecase)

	tests := []struct {
		name   string
		invoke func() error
	}{
		{
			name: "summary",
			invoke: func() error {
				_, err := nodes.GetSummary(context.Background(), &pb.GetSummaryReq{})
				return err
			},
		},
		{
			name: "nodes",
			invoke: func() error {
				_, err := nodes.GetAllNodes(context.Background(), &pb.GetAllNodesReq{})
				return err
			},
		},
		{
			name: "cards",
			invoke: func() error {
				_, err := cards.GetAllGPUs(context.Background(), &pb.GetAllGpusReq{})
				return err
			},
		},
		{
			name: "card types",
			invoke: func() error {
				_, err := cards.GetAllGPUTypes(context.Background(), &pb.GetAllGpusReq{})
				return err
			},
		},
		{
			name: "containers",
			invoke: func() error {
				_, err := containers.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.invoke(); err != nil {
				t.Fatalf("request without filters returned an error: %v", err)
			}
		})
	}
}

type emptyFiltersPodRepo struct {
	containers []*biz.Container
}

func (r *emptyFiltersPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return r.containers, nil
}

func (*emptyFiltersPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}
