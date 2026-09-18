package service

import (
	"context"
	"testing"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type splitTestNodeRepo struct {
	containerTestNodeRepo
	devices []*biz.DeviceInfo
}

func (r *splitTestNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return r.devices, nil
}

func TestSplitRepliesCarryEveryMIGField(t *testing.T) {
	profile := biz.MigProfile{Name: "3g.40gb", MemoryMB: 40192, SliceCount: 3, Core: 43, Placements: []biz.MigPlacement{{Start: 0, Size: 4}, {Start: 4, Size: 4}}}
	nodes := &splitTestNodeRepo{devices: []*biz.DeviceInfo{{
		Id: "GPU-M", AliasId: "GPU-M", Type: "NVIDIA A100", Mode: "mig", Provider: biz.NvidiaGPUDevice, MigProfiles: []biz.MigProfile{profile},
	}}}
	containers := []*biz.Container{{Name: "worker", PodUID: "pod-1", Status: biz.ContainerStatusSuccess, ContainerDevices: biz.ContainerDevices{
		{UUID: "GPU-M", Type: "NVIDIA", Usedmem: 40192, Usedcores: 43, Vendor: biz.NvidiaGPUDevice, Shape: biz.SplitShapeMig, Template: "3g.40gb", MigPlacement: biz.MigPlacement{Start: 4, Size: 4}},
		{UUID: "GPU-C", Type: "NVIDIA", Usedmem: 4096, Usedcores: 30, Vendor: biz.NvidiaGPUDevice, Shape: biz.SplitShapeSoft},
	}}}
	pods := biz.NewPodUseCase(&containerTestPodRepo{containers: containers}, log.DefaultLogger)
	nodeUsecase := biz.NewNodeUsecase(nodes, log.DefaultLogger)

	cards := NewCardService(nodeUsecase, pods)
	gpu, err := cards.GetGPU(context.Background(), &pb.GetGpuReq{Uid: "GPU-M"})
	if err != nil {
		t.Fatal(err)
	}
	if gpu.Mode != "mig" || len(gpu.MigProfiles) != 1 {
		t.Fatalf("GetGPU = %+v", gpu)
	}
	if got := gpu.MigProfiles[0]; got.Name != "3g.40gb" || got.MemoryMb != 40192 || got.SliceCount != 3 || got.Core != 43 ||
		len(got.Placements) != 2 || got.Placements[1].Start != 4 || got.Placements[1].Size != 4 {
		t.Fatalf("profile = %+v", got)
	}

	service := NewContainerService(nodeUsecase, pods)
	reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil || len(reply.Items) != 1 || len(reply.Items[0].Devices) != 2 {
		t.Fatalf("GetAllContainers = %v, %v", reply, err)
	}
	mig, soft := reply.Items[0].Devices[0], reply.Items[0].Devices[1]
	if mig.AllocationShape != biz.SplitShapeMig || mig.Template != "3g.40gb" || mig.GetMigStart() != 4 || mig.GetMigSize() != 4 || mig.MigStart == nil {
		t.Fatalf("MIG device = %+v", mig)
	}
	if soft.AllocationShape != biz.SplitShapeSoft || soft.MigStart != nil || soft.MigSize != nil {
		t.Fatalf("HAMi-core device = %+v", soft)
	}
}

func TestUnknownShapesSayWhy(t *testing.T) {
	containers := []*biz.Container{
		{Name: "broken", PodUID: "pod-1", Status: biz.ContainerStatusSuccess, ContainerDevices: biz.ContainerDevices{
			{UUID: "GPU-M", Type: "NVIDIA", Usedmem: 5120, Usedcores: 14, Vendor: biz.NvidiaGPUDevice, Shape: biz.SplitShapeUnknown, ShapeReason: "mig_reservation_invalid"},
		}},
		{Name: "mixed", PodUID: "pod-2", Status: biz.ContainerStatusSuccess, ContainerDevices: biz.ContainerDevices{
			{UUID: "GPU-M", Type: "NVIDIA", Vendor: biz.NvidiaGPUDevice, Shape: biz.SplitShapeUnknown, ShapeReason: "mig_reservation_missing"},
			{UUID: "GPU-C", Type: "NVIDIA", Vendor: biz.NvidiaGPUDevice, Shape: biz.SplitShapeSoft},
		}},
	}
	service := NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&containerTestPodRepo{containers: containers}, log.DefaultLogger),
	)
	reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil || len(reply.Items) != 2 {
		t.Fatalf("GetAllContainers = %v, %v", reply, err)
	}
	byName := map[string]*pb.ContainerReply{}
	for _, item := range reply.Items {
		byName[item.Name] = item
	}
	if item := byName["broken"]; item.AllocationShape != biz.SplitShapeUnknown || item.AllocationShapeReason != "mig_reservation_invalid" ||
		item.Devices[0].AllocationShapeReason != "mig_reservation_invalid" || !item.GetAllocatedCoresKnown() {
		t.Fatalf("broken = %+v", item)
	}
	// Devices that disagree leave the container's shape, and so its reason, empty.
	if item := byName["mixed"]; item.AllocationShape != "" || item.AllocationShapeReason != "" || item.Devices[0].AllocationShapeReason != "mig_reservation_missing" {
		t.Fatalf("mixed = %+v", item)
	}
}
