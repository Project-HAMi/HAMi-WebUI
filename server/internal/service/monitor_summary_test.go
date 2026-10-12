package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

// summaryTestPodRepo returns an error so every StatisticsByDeviceId lookup
// fails, which is the condition that used to leave capacity totals behind.
type summaryTestPodRepo struct{ err error }

func (r *summaryTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	if r.err != nil {
		return nil, r.err
	}
	return nil, nil
}

func (*summaryTestPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}

// A failing statistics lookup must not leave the device's capacity in the
// totals: keeping it would understate every reported distribution rate.
func TestMonitorSummaryExcludesDevicesWithFailedStatistics(t *testing.T) {
	devices := []*biz.DeviceInfo{
		{Id: "GPU-1", AliasId: "GPU-1", Count: 1, Devmem: 65536, Devcore: 100, NodeName: "node-a"},
		{Id: "GPU-2", AliasId: "GPU-2", Count: 1, Devmem: 65536, Devcore: 100, NodeName: "node-a"},
	}
	nodeRepo := &capacityTestNodeRepo{nodes: []*biz.Node{{Name: "node-a"}}, devices: devices}
	podRepo := &summaryTestPodRepo{err: errors.New("statistics unavailable")}

	svc := NewMonitorService(nil,
		biz.NewNodeUsecase(nodeRepo, log.DefaultLogger),
		biz.NewPodUseCase(podRepo, log.DefaultLogger),
	)

	got, err := svc.Summary(context.Background(), &pb.SummaryFilter{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.GetDistributionRate().GetVgpu() != 0 || got.GetDistributionRate().GetMemory() != 0 || got.GetDistributionRate().GetCore() != 0 {
		t.Fatalf("distribution rate = %+v, want 0 when no statistics resolve", got.GetDistributionRate())
	}
	if got.GetScaling().GetVgpu() != 0 || got.GetScaling().GetMemory() != 0 || got.GetScaling().GetCore() != 0 {
		t.Fatalf("scaling = %+v, want 0 when no statistics resolve", got.GetScaling())
	}
}

// Node and device filters must restrict the aggregation scope.
func TestMonitorSummaryAppliesNodeAndDeviceFilters(t *testing.T) {
	devices := []*biz.DeviceInfo{
		{Id: "GPU-1", AliasId: "GPU-1", Count: 1, Devmem: 1024, Devcore: 100, NodeName: "node-a"},
		{Id: "GPU-2", AliasId: "GPU-2", Count: 1, Devmem: 1024, Devcore: 100, NodeName: "node-b"},
	}
	nodeRepo := &capacityTestNodeRepo{nodes: []*biz.Node{{Name: "node-a"}, {Name: "node-b"}}, devices: devices}
	podRepo := &summaryTestPodRepo{}

	svc := NewMonitorService(nil,
		biz.NewNodeUsecase(nodeRepo, log.DefaultLogger),
		biz.NewPodUseCase(podRepo, log.DefaultLogger),
	)

	filtered, err := svc.Summary(context.Background(), &pb.SummaryFilter{Node: "node-a"})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if filtered.GetDistributionRate().GetVgpu() != 0 {
		t.Fatalf("node filter rate = %v, want 0 with no allocations", filtered.GetDistributionRate().GetVgpu())
	}

	// No device matches the filter, so nothing is aggregated and no divide
	// by zero can occur.
	missing, err := svc.Summary(context.Background(), &pb.SummaryFilter{Device: "no-such-device"})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if missing.GetDistributionRate().GetVgpu() != 0 {
		t.Fatalf("unmatched device filter rate = %v, want 0", missing.GetDistributionRate().GetVgpu())
	}
}
