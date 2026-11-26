package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

func TestAllocationCapacityUsesDeterministicPhysicalCoreBaseline(t *testing.T) {
	device1 := &biz.DeviceInfo{
		Id:       "GPU-1",
		AliasId:  "GPU-1",
		Count:    1,
		Devmem:   65536,
		Devcore:  200,
		NodeName: "node-a",
		NodeUid:  "node-uid-a",
	}
	device2 := &biz.DeviceInfo{
		Id:       "GPU-2",
		AliasId:  "GPU-2",
		Count:    2,
		Devmem:   32768,
		Devcore:  300,
		NodeName: "node-a",
		NodeUid:  "node-uid-a",
	}
	nodeRepo := &capacityTestNodeRepo{
		nodes:   []*biz.Node{{Name: "node-a", Uid: "node-uid-a", Devices: []*biz.DeviceInfo{device1, device2}}},
		devices: []*biz.DeviceInfo{device1, device2},
	}
	podRepo := &capacityTestPodRepo{}
	nodeUsecase := biz.NewNodeUsecase(nodeRepo, log.DefaultLogger)
	podUsecase := biz.NewPodUseCase(podRepo, log.DefaultLogger)
	summaryUsecase := biz.NewSummaryUseCase(nodeRepo, podRepo, log.DefaultLogger)
	nodeService := NewNodeService(nodeUsecase, podUsecase, summaryUsecase, nil)
	cardService := NewCardService(nodeUsecase, podUsecase)

	nodes, err := nodeService.GetAllNodes(
		context.Background(),
		&pb.GetAllNodesReq{Filters: &pb.GetAllNodesReq_Filters{}},
	)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes.List) != 1 {
		t.Fatalf("GetAllNodes returned %d nodes, want 1", len(nodes.List))
	}
	assertCapacity(t, "node list", nodes.List[0].GetCoreTotal(), nodes.List[0].GetMemoryTotal(), 200, 98304)

	node, err := nodeService.GetNode(context.Background(), &pb.GetNodeReq{Uid: "node-uid-a"})
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	assertCapacity(t, "node detail", node.GetCoreTotal(), node.GetMemoryTotal(), 200, 98304)

	cards, err := cardService.GetAllGPUs(
		context.Background(),
		&pb.GetAllGpusReq{Filters: &pb.GetAllGpusReq_Filters{}},
	)
	if err != nil {
		t.Fatalf("GetAllGPUs: %v", err)
	}
	if len(cards.List) != 2 {
		t.Fatalf("GetAllGPUs returned %d cards, want 2", len(cards.List))
	}
	assertCapacity(t, "first card list", cards.List[0].CoreTotal, cards.List[0].MemoryTotal, 100, 65536)
	assertCapacity(t, "second card list", cards.List[1].CoreTotal, cards.List[1].MemoryTotal, 100, 32768)

	card, err := cardService.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "GPU-2"})
	if err != nil {
		t.Fatalf("GetGPU: %v", err)
	}
	assertCapacity(t, "card detail", card.CoreTotal, card.MemoryTotal, 100, 32768)

	summary, err := nodeService.GetSummary(context.Background(), &pb.GetSummaryReq{})
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	assertCapacity(t, "summary", summary.CoreTotal, summary.MemoryTotal, 200, 98304)
	if summary.GpuCount != 2 || summary.NodeCount != 1 || summary.VgpuTotal != 3 {
		t.Fatalf("summary counts = gpu:%d node:%d vgpu:%d, want 2/1/3", summary.GpuCount, summary.NodeCount, summary.VgpuTotal)
	}

	filteredSummary, err := nodeService.GetSummary(context.Background(), &pb.GetSummaryReq{
		Filters: &pb.GetSummaryReq_Filters{DeviceId: "GPU-1"},
	})
	if err != nil {
		t.Fatalf("GetSummary filtered by device: %v", err)
	}
	assertCapacity(t, "filtered summary", filteredSummary.CoreTotal, filteredSummary.MemoryTotal, 100, 65536)
}

func assertCapacity(t *testing.T, scope string, core, memory, wantCore, wantMemory int32) {
	t.Helper()
	if core != wantCore || memory != wantMemory {
		t.Fatalf("%s capacity = core:%d memory:%d, want core:%d memory:%d", scope, core, memory, wantCore, wantMemory)
	}
}

type capacityTestNodeRepo struct {
	nodes   []*biz.Node
	devices []*biz.DeviceInfo
}

func (r *capacityTestNodeRepo) ListAll(context.Context) ([]*biz.Node, error) {
	return r.nodes, nil
}

func (r *capacityTestNodeRepo) GetNode(context.Context, string) (*biz.Node, error) {
	return r.nodes[0], nil
}

func (r *capacityTestNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return r.devices, nil
}

func (r *capacityTestNodeRepo) FindDeviceByAliasId(aliasID string) (*biz.DeviceInfo, error) {
	for _, device := range r.devices {
		if device.AliasId == aliasID {
			return device, nil
		}
	}
	return nil, nil
}

type capacityTestPodRepo struct {
	biz.NodeHolderReader
	containers []*biz.Container
	err        error
}

func (r *capacityTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return r.containers, r.err
}

func (*capacityTestPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}

func TestGetGPUValidationNotFoundAndLargeScaleAggregation(t *testing.T) {
	dev := &biz.DeviceInfo{
		Id:       "GPU-LARGE",
		AliasId:  "GPU-LARGE",
		Count:    200,
		Devmem:   81920,
		Devcore:  100,
		NodeName: "node-a",
		NodeUid:  "node-uid-a",
		Provider: biz.NvidiaGPUDevice,
	}
	containers := make([]*biz.Container, 0, 150)
	for i := 0; i < 150; i++ {
		containers = append(containers, &biz.Container{
			Name:     fmt.Sprintf("worker-%d", i),
			PodUID:   fmt.Sprintf("pod-%d", i),
			NodeName: "node-a",
			NodeUID:  "node-uid-a",
			ContainerDevices: biz.ContainerDevices{{
				UUID:      "GPU-LARGE",
				Usedcores: 1,
				Usedmem:   256,
			}},
		})
	}
	nodeRepo := &capacityTestNodeRepo{
		nodes:   []*biz.Node{{Name: "node-a", Uid: "node-uid-a", Devices: []*biz.DeviceInfo{dev}}},
		devices: []*biz.DeviceInfo{dev},
	}
	podRepo := &capacityTestPodRepo{containers: containers}
	nodeUsecase := biz.NewNodeUsecase(nodeRepo, log.DefaultLogger)
	podUsecase := biz.NewPodUseCase(podRepo, log.DefaultLogger)
	summaryUsecase := biz.NewSummaryUseCase(nodeRepo, podRepo, log.DefaultLogger)
	nodeService := NewNodeService(nodeUsecase, podUsecase, summaryUsecase, nil)
	cardService := NewCardService(nodeUsecase, podUsecase)

	if _, err := cardService.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "   "}); kratoserrors.FromError(err).GetCode() != 400 {
		t.Fatalf("expected 400 INVALID_ARGUMENT, got %v", err)
	}
	if _, err := cardService.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "GPU-MISSING"}); kratoserrors.FromError(err).GetCode() != 404 || kratoserrors.FromError(err).GetReason() != "RESOURCE_GPU_NOT_FOUND" {
		t.Fatalf("expected 404 RESOURCE_GPU_NOT_FOUND, got %v", err)
	}

	// Verify server-side aggregation beyond 100 containers across GetGPU, GetAllGPUs, GetGPUSummary, and GetAllNodes.
	gpu, err := cardService.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "GPU-LARGE"})
	if err != nil || gpu.VgpuUsed != 150 || gpu.CoreUsed != 150 || gpu.MemoryUsed != 150*256 {
		t.Fatalf("GetGPU large aggregation mismatch: err=%v gpu=%+v", err, gpu)
	}
	gpus, err := cardService.GetAllGPUs(context.Background(), &pb.GetAllGpusReq{})
	if err != nil || len(gpus.List) != 1 || gpus.List[0].VgpuUsed != 150 || gpus.List[0].CoreUsed != 150 || gpus.List[0].MemoryUsed != 150*256 {
		t.Fatalf("GetAllGPUs large aggregation mismatch: err=%v list=%+v", err, gpus)
	}
	summary, err := nodeService.GetSummary(context.Background(), &pb.GetSummaryReq{})
	if err != nil || summary.VgpuUsed != 150 || summary.CoreUsed != 150 || summary.MemoryUsed != 150*256 {
		t.Fatalf("GetSummary large aggregation mismatch: err=%v summary=%+v", err, summary)
	}
	nodes, err := nodeService.GetAllNodes(context.Background(), &pb.GetAllNodesReq{})
	if err != nil || len(nodes.List) != 1 || nodes.List[0].GetVgpuUsed() != 150 || nodes.List[0].GetCoreUsed() != 150 || nodes.List[0].GetMemoryUsed() != 150*256 {
		t.Fatalf("GetAllNodes large aggregation mismatch: err=%v nodes=%+v", err, nodes)
	}

	podRepo.err = errors.New("container store failed")
	if _, err := cardService.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "GPU-LARGE"}); err == nil {
		t.Fatal("expected GetGPU to propagate ListAll error")
	}
}
