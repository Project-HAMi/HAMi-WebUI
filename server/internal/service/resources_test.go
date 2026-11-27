package service

import (
	"context"
	"os"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/resourceview"
)

type resourcesTestNodeRepo struct {
	nodes   []*biz.Node
	devices []*biz.DeviceInfo
}

func (r *resourcesTestNodeRepo) ListAll(context.Context) ([]*biz.Node, error) { return r.nodes, nil }
func (r *resourcesTestNodeRepo) GetNode(_ context.Context, uid string) (*biz.Node, error) {
	for _, n := range r.nodes {
		if n.Uid == uid {
			return n, nil
		}
	}
	return nil, os.ErrNotExist
}
func (r *resourcesTestNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return r.devices, nil
}
func (r *resourcesTestNodeRepo) FindDeviceByAliasId(aliasID string) (*biz.DeviceInfo, error) {
	for _, d := range r.devices {
		if d.AliasId == aliasID {
			return d, nil
		}
	}
	return nil, os.ErrNotExist
}

type resourcesTestPodRepo struct{ containers []*biz.Container }

func (r *resourcesTestPodRepo) NodeHolders(_ context.Context, node, uid string) (biz.NodeHolders, error) {
	var matched []*biz.Container
	for _, c := range r.containers {
		if c.NodeName == node && c.NodeUID == uid {
			matched = append(matched, c)
		}
	}
	return biz.NodeHolders{Containers: matched}, nil
}
func (r *resourcesTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return r.containers, nil
}
func (r *resourcesTestPodRepo) FindOne(_ context.Context, podUID, name string) (*biz.Container, error) {
	for _, c := range r.containers {
		if c.PodUID == podUID && c.Name == name {
			return c, nil
		}
	}
	return nil, os.ErrNotExist
}

type resourcesTestSchedulingRepo struct{}

func (resourcesTestSchedulingRepo) ListSchedulingPods(context.Context) ([]*biz.SchedulingPod, error) {
	return nil, nil
}
func (resourcesTestSchedulingRepo) GetSchedulingPod(context.Context, string, string, string) (*biz.SchedulingDetail, error) {
	return nil, kratoserrors.NotFound("POD_NOT_FOUND", "missing")
}

type staticSnapshotSource struct{ snap resourceview.Snapshot }

func (s staticSnapshotSource) Snapshot(context.Context) (resourceview.Snapshot, error) {
	return s.snap, nil
}

func TestResourcesServiceValidationAndDelegation(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	devA := &biz.DeviceInfo{
		Id:       "GPU-1",
		AliasId:  "GPU-1",
		Count:    4,
		Devmem:   81920,
		Devcore:  100,
		Type:     "NVIDIA A100",
		Health:   true,
		NodeName: "node-1",
		NodeUid:  "node-uid-1",
		Provider: biz.NvidiaGPUDevice,
	}
	devB := &biz.DeviceInfo{
		Id:       "Ascend-1",
		AliasId:  "Ascend-1",
		Count:    2,
		Devmem:   32768,
		Devcore:  100,
		Type:     "Ascend910",
		Health:   true,
		NodeName: "node-1",
		NodeUid:  "node-uid-1",
		Provider: biz.AscendGPUDevice,
	}
	pod := &biz.Container{
		Name:       "trainer",
		PodUID:     "pod-uid-1",
		PodName:    "train-job-0",
		Namespace:  "default",
		NodeName:   "node-1",
		NodeUID:    "node-uid-1",
		Status:     biz.ContainerStatusSuccess,
		CreateTime: now,
		ContainerDevices: biz.ContainerDevices{{
			UUID:      "GPU-1",
			Type:      "NVIDIA A100",
			Vendor:    biz.NvidiaGPUDevice,
			Usedmem:   20480,
			Usedcores: 50,
			Shape:     biz.SplitShapeSoft,
		}},
	}

	nodeRepo := &resourcesTestNodeRepo{
		nodes:   []*biz.Node{{Name: "node-1", Uid: "node-uid-1", IsReady: true, IsSchedulable: true, Devices: []*biz.DeviceInfo{devA, devB}}},
		devices: []*biz.DeviceInfo{devA, devB},
	}
	podRepo := &resourcesTestPodRepo{containers: []*biz.Container{pod}}
	nodeUC := biz.NewNodeUsecase(nodeRepo, log.DefaultLogger)
	podUC := biz.NewPodUseCase(podRepo, log.DefaultLogger)
	summaryUC := biz.NewSummaryUseCase(nodeRepo, podRepo, log.DefaultLogger)

	nodeSvc := NewNodeService(nodeUC, podUC, summaryUC, staticSnapshotSource{snap: resourceview.Snapshot{At: now}})
	cardSvc := NewCardService(nodeUC, podUC)
	ctrSvc := NewContainerService(nodeUC, podUC)
	workloadSvc := NewWorkloadService(ctrSvc, biz.NewSchedulingUsecase(resourcesTestSchedulingRepo{}))
	schedSvc := NewSchedulingService(biz.NewSchedulingUsecase(resourcesTestSchedulingRepo{}))
	cfgSvc := NewDeviceConfigService(devicecatalog.Static{Current: &devicecatalog.Snapshot{State: devicecatalog.StateLoaded}})

	res := NewResourcesService(nodeSvc, cardSvc, ctrSvc, workloadSvc, schedSvc, cfgSvc, nil)

	if _, err := res.ListNodes(context.Background(), &pb.ListNodesRequest{IsSchedulable: "not-a-bool"}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("invalid is_schedulable err = %v, want 400", err)
	}
	if _, err := res.GetNode(context.Background(), &pb.GetResourceNodeRequest{Uid: " "}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("empty node uid err = %v, want 400", err)
	}
	if _, err := res.ListNodeContainers(context.Background(), &pb.ListNodeContainersRequest{Uid: "node-uid-1"}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("missing node_name err = %v, want 400", err)
	}
	if _, err := res.ListDevices(context.Background(), &pb.ListDevicesRequest{NodeUid: "node-uid-1"}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("node_uid without node_name err = %v, want 400", err)
	}
	if _, err := res.GetDevice(context.Background(), &pb.GetDeviceRequest{}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("empty device id err = %v, want 400", err)
	}
	if _, err := res.ListContainers(context.Background(), &pb.ListContainersRequest{Page: -1}); !kratoserrors.IsBadRequest(err) {
		t.Fatalf("negative page err = %v, want 400", err)
	}
	if _, err := res.GetContainer(context.Background(), &pb.GetResourceContainerRequest{PodUid: "pod-uid-1", Container: "missing"}); !kratoserrors.IsNotFound(err) {
		t.Fatalf("missing container err = %v, want 404", err)
	}
	if _, err := res.GetContainer(context.Background(), &pb.GetResourceContainerRequest{PodUid: "pod-uid-1", Container: "trainer", DeviceId: "OTHER"}); !kratoserrors.IsNotFound(err) {
		t.Fatalf("mismatched device_id container err = %v, want 404", err)
	}
	if _, err := res.GetAbout(context.Background(), &pb.GetAboutReq{}); !kratoserrors.IsServiceUnavailable(err) {
		t.Fatalf("nil about service err = %v, want 503", err)
	}

	nvidiaOnly, err := res.ListDevices(context.Background(), &pb.ListDevicesRequest{Provider: biz.NvidiaGPUDevice})
	if err != nil {
		t.Fatalf("ListDevices(provider) failed: %v", err)
	}
	if len(nvidiaOnly.List) != 1 || nvidiaOnly.List[0].Uuid != "GPU-1" {
		t.Fatalf("ListDevices(provider) = %+v, want only GPU-1", nvidiaOnly.List)
	}

	detail, err := res.GetContainer(context.Background(), &pb.GetResourceContainerRequest{PodUid: "pod-uid-1", Container: "trainer", DeviceId: "GPU-1"})
	if err != nil {
		t.Fatalf("GetContainer failed: %v", err)
	}
	if detail.AllocatedDevices != 1 || detail.PodUid != "pod-uid-1" {
		t.Fatalf("GetContainer = %+v", detail)
	}
}
