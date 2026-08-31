package service

import (
	"context"
	"testing"

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
	nodeService := NewNodeService(nodeUsecase, podUsecase, summaryUsecase)
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
	assertCapacity(t, "node list", nodes.List[0].CoreTotal, nodes.List[0].MemoryTotal, 200, 98304)

	node, err := nodeService.GetNode(context.Background(), &pb.GetNodeReq{Uid: "node-uid-a"})
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	assertCapacity(t, "node detail", node.CoreTotal, node.MemoryTotal, 200, 98304)

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

type capacityTestPodRepo struct{}

func (*capacityTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return nil, nil
}

func (*capacityTestPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}
