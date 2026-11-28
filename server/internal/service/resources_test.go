package service

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	obsv1 "vgpu/api/observability/v1"
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
	reservations := make(map[string]biz.DeviceReservation)
	for _, c := range r.containers {
		if c.NodeName == node && c.NodeUID == uid {
			matched = append(matched, c)
			for _, cd := range c.ContainerDevices {
				if cd.UUID == "" {
					continue
				}
				entry := reservations[cd.UUID]
				entry.Slots++
				entry.Cores += cd.Usedcores
				entry.MemoryMiB += cd.Usedmem
				if cd.CoreAllocationUnknown {
					entry.CoreUnknown = true
					entry.CoreUnknownCount++
				}
				reservations[cd.UUID] = entry
			}
		}
	}
	return biz.NodeHolders{Reservations: reservations, Containers: matched}, nil
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

	snapDevices := []resourceview.Device{
		{
			Identity: resourceview.DeviceIdentity{Provider: "NVIDIA", Node: "node-1", NodeUID: types.UID("node-uid-1"), DeviceID: "GPU-1"},
			Kind:     "physical",
			States:   map[string]*obsv1.FieldState{"kind": {State: resourceview.Known}},
		},
		{
			Identity: resourceview.DeviceIdentity{Provider: "HCU", Node: "node-1", NodeUID: types.UID("node-uid-1"), DeviceID: "HCU-serial-with-hyphens"},
			Kind:     "physical",
			States:   map[string]*obsv1.FieldState{"kind": {State: resourceview.Known}},
		},
		{
			Identity: resourceview.DeviceIdentity{Provider: "DCU", Node: "node-1", NodeUID: types.UID("node-uid-1"), DeviceID: "DCU-3"},
			Kind:     "physical",
			States:   map[string]*obsv1.FieldState{"kind": {State: resourceview.Known}},
		},
		{
			Identity: resourceview.DeviceIdentity{Provider: "DCU", Node: "node-2", NodeUID: types.UID("node-uid-2"), DeviceID: "DCU-3"},
			Kind:     "physical",
			States:   map[string]*obsv1.FieldState{"kind": {State: resourceview.Known}},
		},
	}
	snapSource := resourceview.WithObservations(staticSnapshotSource{snap: resourceview.Snapshot{
		At: now,
		Nodes: []resourceview.Node{
			{Name: "node-1", UID: types.UID("node-uid-1")},
			{Name: "node-2", UID: types.UID("node-uid-2")},
		},
		Devices: snapDevices,
	}}, nil)
	nodeSvc := NewNodeService(nodeUC, podUC, summaryUC, snapSource)
	cardSvc := NewCardService(nodeUC, podUC, snapSource)
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

	// Public HCU ID differs from internal alias HCU-<serial>; filtering by public ID must match.
	hcuDev := &biz.DeviceInfo{
		Id: "serial-with-hyphens", AliasId: "HCU-serial-with-hyphens",
		Count: 1, Devmem: 16384, Devcore: 100, Type: "HCU", Health: true,
		NodeName: "node-1", NodeUid: "node-uid-1", Provider: biz.HygonHCUDevice,
	}
	dcuNode1 := &biz.DeviceInfo{
		Id: "DCU-3", AliasId: "DCU-3",
		Count: 1, Devmem: 16384, Devcore: 100, Type: "DCU", Health: true,
		NodeName: "node-1", NodeUid: "node-uid-1", Provider: biz.HygonGPUDevice,
	}
	dcuNode2 := &biz.DeviceInfo{
		Id: "DCU-3", AliasId: "DCU-3",
		Count: 1, Devmem: 16384, Devcore: 100, Type: "DCU", Health: true,
		NodeName: "node-2", NodeUid: "node-uid-2", Provider: biz.HygonGPUDevice,
	}
	slashDev := &biz.DeviceInfo{
		Id: "GPU/with space", AliasId: "GPU/with space",
		Count: 1, Devmem: 16384, Devcore: 100, Type: "NVIDIA", Health: true,
		NodeName: "node-1", NodeUid: "node-uid-1", Provider: biz.NvidiaGPUDevice,
	}
	nodeRepo.devices = append(nodeRepo.devices, hcuDev, dcuNode1, dcuNode2, slashDev)
	podRepo.containers = append(podRepo.containers,
		&biz.Container{
			Name: "hcu-worker", PodUID: "pod-hcu-1", PodName: "hcu-pod", Namespace: "default",
			NodeName: "node-1", NodeUID: "node-uid-1", Status: biz.ContainerStatusSuccess, CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "HCU-serial-with-hyphens", Type: "HCU", Vendor: biz.HygonHCUDevice, Usedmem: 4096, Usedcores: 50}},
		},
		&biz.Container{
			Name: "dcu-n1", PodUID: "pod-dcu-1", PodName: "dcu-pod-1", Namespace: "default",
			NodeName: "node-1", NodeUID: "node-uid-1", Status: biz.ContainerStatusSuccess, CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "DCU-3", Type: "DCU", Vendor: biz.HygonGPUDevice, Usedmem: 2048, Usedcores: 25}},
		},
		&biz.Container{
			Name: "dcu-n2", PodUID: "pod-dcu-2", PodName: "dcu-pod-2", Namespace: "default",
			NodeName: "node-2", NodeUID: "node-uid-2", Status: biz.ContainerStatusSuccess, CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "DCU-3", Type: "DCU", Vendor: biz.HygonGPUDevice, Usedmem: 8192, Usedcores: 75}},
		},
	)

	hcuDetail, err := res.GetDevice(context.Background(), &pb.GetDeviceRequest{Id: "serial-with-hyphens", NodeUid: "node-uid-1"})
	wantHCUTelemetryID, _ := resourceview.DeviceID("HCU", "node-1", "HCU-serial-with-hyphens")
	if err != nil || hcuDetail.TelemetryDeviceId != wantHCUTelemetryID {
		t.Fatalf("HCU device telemetryDeviceId = (%+v, %v), want %q", hcuDetail, err, wantHCUTelemetryID)
	}

	hcuContainer, err := res.GetContainer(context.Background(), &pb.GetResourceContainerRequest{
		PodUid: "pod-hcu-1", Container: "hcu-worker", DeviceId: "serial-with-hyphens",
	})
	if err != nil || hcuContainer.AllocatedDevices != 1 || len(hcuContainer.DeviceIds) != 1 || hcuContainer.DeviceIds[0] != "serial-with-hyphens" {
		t.Fatalf("HCU public device_id lookup got (%+v, %v)", hcuContainer, err)
	}

	if _, err := res.GetDevice(context.Background(), &pb.GetDeviceRequest{Id: "DCU-3"}); !kratoserrors.IsConflict(err) {
		t.Fatalf("ambiguous device without node_uid err = %v, want 409 Conflict", err)
	}
	dcu2, err := res.GetDevice(context.Background(), &pb.GetDeviceRequest{Id: "DCU-3", NodeUid: "node-uid-2"})
	wantDCU2TelemetryID, _ := resourceview.DeviceID("DCU", "node-2", "DCU-3")
	if err != nil || dcu2.NodeUid != "node-uid-2" || dcu2.MemoryUsed != 8192 || dcu2.CoreUsed != 75 || dcu2.TelemetryDeviceId != wantDCU2TelemetryID {
		t.Fatalf("disambiguated device detail = (%+v, %v), want node-uid-2 with 8192MiB/75 cores and telemetryDeviceId %q", dcu2, err, wantDCU2TelemetryID)
	}
	slashDetail, err := res.GetDevice(context.Background(), &pb.GetDeviceRequest{Id: "GPU/with space", NodeUid: "node-uid-1"})
	if err != nil || slashDetail.Uuid != "GPU/with space" {
		t.Fatalf("slash device detail = (%+v, %v)", slashDetail, err)
	}
}

type cardTestSnapshotSource struct {
	snap resourceview.Snapshot
}

func (s cardTestSnapshotSource) Snapshot(context.Context) (resourceview.Snapshot, error) {
	return s.snap, nil
}

func TestGetDeviceExposesReservationsTelemetryAndMigMemory(t *testing.T) {
	now := time.Now()
	dev1 := &biz.DeviceInfo{
		Id: "GPU-1", AliasId: "GPU-1", Count: 4, Devmem: 81920, Devcore: 100,
		Type: "NVIDIA A100", Mode: "mig", Health: true,
		NodeName: "node-1", NodeUid: "node-uid-1", Provider: biz.NvidiaGPUDevice,
	}
	nodeUC := biz.NewNodeUsecase(&resourcesTestNodeRepo{devices: []*biz.DeviceInfo{dev1}}, log.NewStdLogger(io.Discard))
	podUC := biz.NewPodUseCase(&resourcesTestPodRepo{containers: []*biz.Container{
		{
			Name: "known", PodUID: "p1", NodeName: "node-1", NodeUID: "node-uid-1", CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "GPU-1", Usedcores: 40, Usedmem: 10240}},
		},
		{
			Name: "unknown", PodUID: "p2", NodeName: "node-1", NodeUID: "node-uid-1", CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "GPU-1", Usedmem: 5120, CoreAllocationUnknown: true}},
		},
	}}, log.NewStdLogger(io.Discard))

	knownFlag := true
	devID := resourceview.DeviceIdentity{Provider: biz.NvidiaGPUDevice, Node: "node-1", NodeUID: "node-uid-1", DeviceID: "GPU-1"}
	q := func(v string, unit string) resourceview.Quantity {
		qty := resource.MustParse(v)
		return resourceview.Quantity{
			Value: &qty, Unit: unit,
			State: &obsv1.FieldState{State: resourceview.Known, Source: "exporter", Scope: "device", ObservedAt: timestamppb.New(now)},
		}
	}
	snap := resourceview.Snapshot{
		At: now,
		Nodes: []resourceview.Node{{
			UID: "node-uid-1", Name: "node-1",
		}},
		Devices: []resourceview.Device{{
			Identity: devID, Kind: "physical", Vendor: "NVIDIA", Model: "NVIDIA A100",
			States: map[string]*obsv1.FieldState{"kind": {State: resourceview.Known}},
		}},
	}
	obs := map[resourceview.DeviceIdentity]resourceview.DeviceObservation{
		devID: {
			UtilizationRatio:    q("0.55", "ratio"),
			TemperatureCelsius:  q("61", "celsius"),
			PowerWatts:          q("180", "watts"),
			MigMemoryUsedBytes:  q("2147483648", "bytes"),
			MigMemoryTotalBytes: q("10737418240", "bytes"),
		},
	}
	_ = knownFlag
	cardSvc := &CardService{
		node: nodeUC,
		pod:  podUC,
		source: resourceview.WithObservations(
			cardTestSnapshotSource{snap: snap},
			cardObsReader(obs),
		),
	}
	reply, err := cardSvc.getDevice(context.Background(), "GPU-1", "node-uid-1", true)
	if err != nil {
		t.Fatalf("getDevice() error = %v", err)
	}
	if reply.GetCoreUnknownCount() != 1 || reply.GetCoreUsedKnown() {
		t.Fatalf("core unknown = (%d, %v), want (1, false)", reply.GetCoreUnknownCount(), reply.GetCoreUsedKnown())
	}
	if reply.GetUtilizationRatio() != 0.55 || reply.GetTemperatureCelsius() != 61 || reply.GetPowerWatts() != 180 {
		t.Fatalf("unexpected device telemetry: %+v", reply)
	}
	if reply.MigInstanceMemory == nil || reply.MigInstanceMemory.UsedBytes != 2147483648 || reply.MigInstanceMemory.GetTotalBytes() != 10737418240 {
		t.Fatalf("MigInstanceMemory = %+v, want (2GiB, 10GiB)", reply.MigInstanceMemory)
	}
}

type cardObsReader map[resourceview.DeviceIdentity]resourceview.DeviceObservation

func (r cardObsReader) DeviceObservations() resourceview.DeviceObservations {
	return resourceview.DeviceObservations(r)
}

func TestGlobalAuditFixesForCardsAndReservations(t *testing.T) {
	now := time.Now()
	npuA := &biz.DeviceInfo{
		Id: "NPU-0", AliasId: "NPU-0", Count: 2, Devmem: 32768, Devcore: 100,
		Type: "Ascend910", Health: true, NodeName: "node-a", NodeUid: "uid-a", Provider: biz.AscendGPUDevice,
	}
	npuB := &biz.DeviceInfo{
		Id: "NPU-0", AliasId: "NPU-0", Count: 2, Devmem: 32768, Devcore: 100,
		Type: "Ascend910", Health: true, NodeName: "node-b", NodeUid: "uid-b", Provider: biz.AscendGPUDevice,
	}
	mluDev := &biz.DeviceInfo{
		Id: "MLU-0", AliasId: "MLU-0", Count: 4, Devmem: 24576, Devcore: 100,
		Type: "MLU370", Health: true, NodeName: "node-a", NodeUid: "uid-a", Provider: biz.CambriconGPUDevice,
	}
	dcuDev := &biz.DeviceInfo{
		Id: "legacy-serial-03", AliasId: "node-a-dcu-3", RegistrationID: "DCU-3", Count: 4, Devmem: 16384, Devcore: 100,
		Type: "DCU", Health: true, NodeName: "node-a", NodeUid: "uid-a", Provider: biz.HygonGPUDevice,
	}
	nodeUC := biz.NewNodeUsecase(&resourcesTestNodeRepo{
		nodes: []*biz.Node{
			{Name: "node-a", Uid: "uid-a", Devices: []*biz.DeviceInfo{npuA, mluDev, dcuDev}},
			{Name: "node-b", Uid: "uid-b", Devices: []*biz.DeviceInfo{npuB}},
		},
		devices: []*biz.DeviceInfo{npuA, npuB, mluDev, dcuDev},
	}, log.NewStdLogger(io.Discard))
	podUC := biz.NewPodUseCase(&resourcesTestPodRepo{containers: []*biz.Container{
		{
			Name: "ctr-a", PodUID: "pod-a", NodeName: "node-a", NodeUID: "uid-a", CreateTime: now,
			ContainerDevices: biz.ContainerDevices{{UUID: "NPU-0", Usedcores: 50, Usedmem: 4096}},
		},
	}}, log.NewStdLogger(io.Discard))
	sumUC := biz.NewSummaryUseCase(
		&resourcesTestNodeRepo{devices: []*biz.DeviceInfo{npuA, npuB}},
		&resourcesTestPodRepo{containers: []*biz.Container{
			{Name: "ctr-a", PodUID: "pod-a", NodeName: "node-a", NodeUID: "uid-a", ContainerDevices: biz.ContainerDevices{{UUID: "NPU-0", Usedcores: 50, Usedmem: 4096}}},
		}},
		log.NewStdLogger(io.Discard),
	)
	q := func(v, unit string) resourceview.Quantity {
		qty := resource.MustParse(v)
		return resourceview.Quantity{
			Value: &qty, Unit: unit,
			State: &obsv1.FieldState{State: resourceview.Known, Source: "exporter", Scope: "device", ObservedAt: timestamppb.New(now)},
		}
	}
	mluObsID := resourceview.DeviceIdentity{Provider: biz.CambriconGPUDevice, Node: "node-a", NodeUID: "uid-a", DeviceID: "MLU-0"}
	cardSvc := NewCardService(nodeUC, podUC, resourceview.WithObservations(
		cardTestSnapshotSource{snap: resourceview.Snapshot{At: now}},
		cardObsReader{
			mluObsID: {
				UtilizationRatio:    q("0.37", "ratio"),
				MemoryUsedBytes:     q("4294967296", "bytes"),
				MemoryPhysicalBytes: q("25769803776", "bytes"),
			},
		},
	))
	nodeSvc := NewNodeService(nodeUC, podUC, sumUC, nil)

	// C3: unscoped list must keep node-a NPU-0 separate from node-b NPU-0, and include DCU RegistrationId (C4).
	listReply, err := cardSvc.GetAllGPUs(context.Background(), &pb.GetAllGpusReq{})
	if err != nil {
		t.Fatalf("GetAllGPUs() error = %v", err)
	}
	var seenNpuB, seenDcu bool
	for _, row := range listReply.List {
		if row.Uuid == "NPU-0" && row.NodeUid == "uid-b" {
			seenNpuB = true
			if row.MemoryUsed != 0 || row.VgpuUsed != 0 || row.CoreUsed != 0 {
				t.Fatalf("cross-node alias bleed on uid-b NPU-0: %+v", row)
			}
		}
		if row.Uuid == "legacy-serial-03" {
			seenDcu = true
			if row.GetRegistrationId() != "DCU-3" {
				t.Fatalf("DCU registrationId = %q, want DCU-3", row.GetRegistrationId())
			}
		}
	}
	if !seenNpuB || !seenDcu {
		t.Fatalf("missing expected rows (npuB=%t dcu=%t)", seenNpuB, seenDcu)
	}
	nodesReply, err := nodeSvc.GetAllNodes(context.Background(), &pb.GetAllNodesReq{})
	if err != nil || len(nodesReply.List) != 2 {
		t.Fatalf("GetAllNodes() = (%+v, %v)", nodesReply, err)
	}
	for _, n := range nodesReply.List {
		if n.Uid == "uid-b" && *n.MemoryUsed != 0 {
			t.Fatalf("uid-b node leaked memoryUsed = %d", *n.MemoryUsed)
		}
	}

	// C2: MLU absent from PhysicalInventory still gets utilization & memory from obsReader fallback.
	mluDetail, err := cardSvc.getDevice(context.Background(), "MLU-0", "uid-a", true)
	if err != nil {
		t.Fatalf("getDevice(MLU-0) error = %v", err)
	}
	if mluDetail.GetUtilizationRatio() != 0.37 || mluDetail.GetMemoryUsedBytes() != 4294967296 || mluDetail.GetMemoryPhysicalBytes() != 25769803776 {
		t.Fatalf("MLU detail telemetry = %+v", mluDetail)
	}
	if !mluDetail.GetNodeAllocationComplete() {
		t.Fatalf("MLU detail expected nodeAllocationComplete=true")
	}
}
