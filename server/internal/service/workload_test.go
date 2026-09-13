package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

func workloadTestService(containers []*biz.Container, pods ...*biz.SchedulingPod) *WorkloadService {
	return NewWorkloadService(NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&containerTestPodRepo{containers: containers}, log.DefaultLogger),
	), biz.NewSchedulingUsecase(&schedulingTestRepository{pods: pods}))
}

func workloadTestContainer(pod, status string) *biz.Container {
	return &biz.Container{Name: "main", PodName: pod, Namespace: "dev", PodUID: pod + "-uid", NodeName: "node-a", NodeUID: "node-a-uid", Status: status,
		ContainerDevices: biz.ContainerDevices{{UUID: "gpu-a", Usedcores: 5, Usedmem: 256}}}
}

func workloadTestRequest(name, kind, value string) biz.SchedulingContainerRequest {
	return biz.SchedulingContainerRequest{Container: name, ContainerKind: kind,
		Resources: []biz.SchedulingResource{{Name: "nvidia.com/gpu", Value: value, Kind: "count"}}}
}

func workloadTestPending(name string, requests ...biz.SchedulingContainerRequest) *biz.SchedulingPod {
	return &biz.SchedulingPod{Name: name, UID: name + "-uid", Namespace: "dev", Stage: "waiting", Requests: requests}
}

func TestWorkloadJoinUsesContainerRowsWithoutChangingAllocationInventory(t *testing.T) {
	preallocated := workloadTestContainer("pending", biz.ContainerStatusWaiting)
	preallocated.NodeName = ""
	bound := workloadTestContainer("bound", biz.ContainerStatusSuccess)
	pending := workloadTestPending("pending", workloadTestRequest("main", "regular", "1"), workloadTestRequest("worker", "regular", "2"),
		workloadTestRequest("prepare", "init", "1"), workloadTestRequest("helper", "sidecar", "1"), workloadTestRequest("zero", "regular", "0"))
	pending.Preallocated = true
	staleBound := workloadTestPending("bound", workloadTestRequest("main", "regular", "1"))
	svc := workloadTestService([]*biz.Container{bound, preallocated}, pending, staleBound)
	before, err := svc.containers.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || result.Total != 5 || len(result.Items) != 5 {
		t.Fatalf("join = %v, %v", result, err)
	}
	kinds := map[string]string{}
	for _, row := range result.Items {
		if row.PodUid == bound.PodUID {
			if row.Pending || row.AllocatedMem != 256 || row.ContainerKind != "regular" {
				t.Fatalf("known binding must win: %v", row)
			}
			continue
		}
		if !row.Pending || row.Status != "pending" || row.Scheduling.Uid != pending.UID || row.NodeName != "" || len(row.DeviceIds) != 0 || row.AllocatedDevices != 0 || row.AllocatedMem != 0 || row.AllocatedCoresKnown != nil {
			t.Fatalf("request misrepresented as allocation: %v", row)
		}
		if row.Request.Container != row.Name {
			t.Fatalf("request belongs to a different container: %v", row)
		}
		kinds[row.Name] = row.ContainerKind
	}
	if !reflect.DeepEqual(kinds, map[string]string{"main": "regular", "worker": "regular", "prepare": "init", "helper": "sidecar"}) {
		t.Fatalf("GPU container expansion: %v", kinds)
	}
	after, err := svc.containers.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil || !reflect.DeepEqual(before, after) || len(after.Items) != 2 || after.Items[0].AllocatedCores != 5 {
		t.Fatalf("unified display changed existing allocation reply: before=%v after=%v err=%v", before, after, err)
	}
}

func TestWorkloadFiltersAndPaginationApplyToWholeJoinedList(t *testing.T) {
	abnormal := workloadTestContainer("failed", biz.ContainerStatusError)
	abnormal.Priority = "1"
	bound := workloadTestContainer("running", biz.ContainerStatusSuccess)
	bound.ContainerDevices = append(bound.ContainerDevices, biz.ContainerDevice{UUID: "gpu-b", Usedcores: 7, Usedmem: 512})
	pending := workloadTestPending("pending", workloadTestRequest("worker", "regular", "1"))
	pending.Gates = []string{"test.io/hold"}
	pending.Stage = "gated"
	svc := workloadTestService([]*biz.Container{bound, abnormal}, pending)
	for _, tc := range []struct {
		name    string
		filters *pb.ListWorkloadsRequest_Filters
		want    int32
	}{
		{"all", nil, 3},
		{"namespace", &pb.ListWorkloadsRequest_Filters{Namespace: "other"}, 0},
		{"container", &pb.ListWorkloadsRequest_Filters{Name: "worker"}, 1},
		{"pod", &pb.ListWorkloadsRequest_Filters{Name: "pending"}, 1},
		{"pending", &pb.ListWorkloadsRequest_Filters{Status: "pending"}, 1},
		{"abnormal", &pb.ListWorkloadsRequest_Filters{Status: "abnormal"}, 1},
		{"node", &pb.ListWorkloadsRequest_Filters{NodeName: "node-a"}, 2},
		{"node uid", &pb.ListWorkloadsRequest_Filters{NodeUid: "node-a-uid"}, 2},
		{"gpu", &pb.ListWorkloadsRequest_Filters{DeviceId: "gpu-a"}, 2},
		{"gpu priority", &pb.ListWorkloadsRequest_Filters{Priority: "1"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: tc.filters, PageSize: 1})
			if err != nil || result.Total != tc.want || len(result.Items) > 1 {
				t.Fatalf("filtered page = %v, %v", result, err)
			}
		})
	}
	var names []string
	for page := int32(1); page <= 3; page++ {
		result, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Page: page, PageSize: 1})
		if err != nil || result.Total != 3 || len(result.Items) != 1 {
			t.Fatalf("page %d = %v, %v", page, result, err)
		}
		names = append(names, result.Items[0].AppName)
	}
	if !reflect.DeepEqual(names, []string{"failed", "pending", "running"}) {
		t.Fatalf("global page order = %v", names)
	}
	result, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: &pb.ListWorkloadsRequest_Filters{DeviceId: "gpu-b"}})
	if err != nil || result.Total != 1 || result.Items[0].AllocatedMem != 512 || result.Items[0].AllocatedCores != 7 || result.Items[0].AllocatedDevices != 1 {
		t.Fatalf("per-GPU allocation projection changed: %v %v", result, err)
	}
	result, err = svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Page: 2147483647, PageSize: 2147483647})
	if err != nil || result.Total != 3 || len(result.Items) != 0 {
		t.Fatalf("overflow page = %v %v", result, err)
	}
}

func TestWorkloadStableStatusAndIdentityOrder(t *testing.T) {
	containers := []*biz.Container{
		workloadTestContainer("z-error", biz.ContainerStatusFailed),
		workloadTestContainer("a-error", biz.ContainerStatusNotReady),
		workloadTestContainer("unknown", biz.ContainerStatusUnknown),
		workloadTestContainer("b-starting", biz.ContainerStatusWaiting),
		workloadTestContainer("terminating", biz.ContainerStatusTerminating),
		workloadTestContainer("running", biz.ContainerStatusSuccess),
		workloadTestContainer("completed", biz.ContainerStatusClosed),
	}
	pending := []*biz.SchedulingPod{workloadTestPending("c-pending", workloadTestRequest("main", "regular", "1")), workloadTestPending("a-pending", workloadTestRequest("main", "regular", "1"))}
	want := []string{"a-error", "unknown", "z-error", "a-pending", "c-pending", "b-starting", "terminating", "running", "completed"}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := workloadTestService(containers, pending...).ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, row := range result.Items {
			names = append(names, row.AppName)
		}
		if !reflect.DeepEqual(names, want) {
			t.Fatalf("stable order = %v", names)
		}
		for i, j := 0, len(containers)-1; i < j; i, j = i+1, j-1 {
			containers[i], containers[j] = containers[j], containers[i]
		}
		pending[0], pending[1] = pending[1], pending[0]
	}
}

func TestWorkloadBindingReplacesPendingWithoutDuplicateIdentity(t *testing.T) {
	pending := workloadTestPending("work", workloadTestRequest("main", "regular", "1"))
	container := workloadTestContainer("work", biz.ContainerStatusWaiting)
	container.NodeName = ""
	repo := &schedulingTestRepository{pods: []*biz.SchedulingPod{pending}}
	svc := workloadTestService([]*biz.Container{container})
	svc.scheduling = biz.NewSchedulingUsecase(repo)
	before, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || before.Total != 1 || !before.Items[0].Pending {
		t.Fatalf("pre-bind = %v, %v", before, err)
	}
	container.NodeName = "node-a"
	repo.pods = nil
	after, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || after.Total != 1 || after.Items[0].Pending || before.Items[0].PodUid != after.Items[0].PodUid || before.Items[0].Name != after.Items[0].Name {
		t.Fatalf("binding identity changed = %v, %v", after, err)
	}
}

type workloadTestNodes struct{ containerTestNodeRepo }

func (*workloadTestNodes) ListAll(context.Context) ([]*biz.Node, error) {
	return []*biz.Node{{Name: "node-a", Uid: "node-a-uid"}}, nil
}

func TestWorkloadBoundRequestRemainsVisibleWithoutInventingAllocations(t *testing.T) {
	request := workloadTestRequest("main", "regular", "1")
	request.Status = biz.ContainerStatusError
	request.StatusDetail = &biz.ContainerStatusDetail{ContainerState: "Waiting", Reason: "CreateContainerError"}
	pod := workloadTestPending("native-scheduled", request)
	pod.NodeName, pod.Stage, pod.SchedulerName = "node-a", "bound", "default-scheduler"
	svc := workloadTestService(nil, pod)
	svc.containers.node = biz.NewNodeUsecase(&workloadTestNodes{}, log.DefaultLogger)
	reply, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: &pb.ListWorkloadsRequest_Filters{NodeUid: "node-a-uid", Status: "abnormal"}})
	if err != nil || reply.Total != 1 {
		t.Fatalf("bound GPU request disappeared: %v, %v", reply, err)
	}
	row := reply.Items[0]
	if row.Pending || row.Status != biz.ContainerStatusError || row.NodeName != "node-a" || row.StatusDetail.Reason != "CreateContainerError" || row.Scheduling == nil || row.Request == nil {
		t.Fatalf("bound request status/identity: %v", row)
	}
	if row.AllocatedDevices != 0 || row.AllocatedCores != 0 || row.AllocatedMem != 0 || row.AllocatedCoresKnown != nil || len(row.DeviceIds) != 0 {
		t.Fatalf("invented GPU allocation: %v", row)
	}
	for _, filters := range []*pb.ListWorkloadsRequest_Filters{{DeviceId: "gpu-a"}, {Status: "pending"}, {Priority: "0"}} {
		reply, err = svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: filters})
		if err != nil || reply.Total != 0 {
			t.Fatalf("unknown allocation matched %v: %v, %v", filters, reply, err)
		}
	}
	old, err := svc.containers.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil || len(old.Items) != 0 {
		t.Fatalf("request entered allocation inventory: %v, %v", old, err)
	}
}

func TestWorkloadRealAllocationWinsPerContainerWithoutHidingActiveBoundInit(t *testing.T) {
	allocated := workloadTestContainer("mixed", biz.ContainerStatusWaiting)
	main, init := workloadTestRequest("main", "regular", "1"), workloadTestRequest("prepare", "init", "1")
	init.Status = biz.ContainerStatusError
	pod := workloadTestPending("mixed", main, init)
	pod.NodeName, pod.Stage = "node-a", "bound"
	reply, err := workloadTestService([]*biz.Container{allocated}, pod).ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || reply.Total != 2 {
		t.Fatalf("container merge = %v, %v", reply, err)
	}
	for _, row := range reply.Items {
		if row.Name == "main" && (row.Request != nil || row.AllocatedMem != 256 || len(row.DeviceIds) != 1) {
			t.Fatalf("actual allocation was overridden: %v", row)
		}
		if row.Name == "prepare" && (row.Request == nil || row.ContainerKind != "init" || row.Pending || row.Status != biz.ContainerStatusError) {
			t.Fatalf("unrecorded bound init disappeared: %v", row)
		}
	}
}

func TestWorkloadPendingRowsPutLongestWaitFirst(t *testing.T) {
	older := workloadTestPending("z-older", workloadTestRequest("main", "regular", "1"))
	older.CreatedAt = time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	newer := workloadTestPending("a-newer", workloadTestRequest("main", "regular", "1"))
	newer.CreatedAt = time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	starting := workloadTestContainer("m-starting", biz.ContainerStatusWaiting)
	failed := workloadTestContainer("y-failed", biz.ContainerStatusFailed)
	result, err := workloadTestService([]*biz.Container{starting, failed}, newer, older).ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, row := range result.Items {
		names = append(names, row.AppName)
	}
	if want := []string{"y-failed", "z-older", "a-newer", "m-starting"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("pending order = %v, want %v", names, want)
	}
}

func TestWorkloadOmitsCompletedContainersWithoutAllocation(t *testing.T) {
	allocated := workloadTestContainer("train", biz.ContainerStatusClosed)
	request := func(name, kind, status string) biz.SchedulingContainerRequest {
		r := workloadTestRequest(name, kind, "1")
		r.Status = status
		return r
	}
	pod := &biz.SchedulingPod{Name: "train", UID: allocated.PodUID, Namespace: "dev", Stage: "bound", NodeName: "node-a", Requests: []biz.SchedulingContainerRequest{
		request("main", "regular", biz.ContainerStatusClosed),
		request("warmup", "init", biz.ContainerStatusClosed),
		request("export", "regular", biz.ContainerStatusClosed),
		request("helper", "sidecar", biz.ContainerStatusSuccess),
	}}
	result, err := workloadTestService([]*biz.Container{allocated}, pod).ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, row := range result.Items {
		rows[row.Name] = row.Status
	}
	if want := map[string]string{"main": biz.ContainerStatusClosed, "helper": biz.ContainerStatusSuccess}; !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
}

func TestWorkloadStatusCountsApplyEveryFilterExceptStatus(t *testing.T) {
	running := workloadTestContainer("running", biz.ContainerStatusSuccess)
	starting := workloadTestContainer("starting", biz.ContainerStatusWaiting)
	failed := workloadTestContainer("failed", biz.ContainerStatusFailed)
	lost := workloadTestContainer("lost", biz.ContainerStatusUnknown)
	stopping := workloadTestContainer("stopping", biz.ContainerStatusTerminating)
	other := workloadTestContainer("other-node", biz.ContainerStatusSuccess)
	other.NodeName, other.NodeUID = "node-b", "node-b-uid"
	pending := workloadTestPending("pending", workloadTestRequest("main", "regular", "1"))
	svc := workloadTestService([]*biz.Container{running, starting, failed, lost, stopping, other}, pending)
	for _, tc := range []struct {
		name    string
		filters *pb.ListWorkloadsRequest_Filters
		total   int32
		want    map[string]int32
	}{
		{"whole list", nil, 7, map[string]int32{"all": 7, "pending": 1, "waiting": 1, "success": 2, "abnormal": 2}},
		{"status filter keeps every count", &pb.ListWorkloadsRequest_Filters{Status: "abnormal"}, 2, map[string]int32{"all": 7, "pending": 1, "waiting": 1, "success": 2, "abnormal": 2}},
		{"node scope excludes unbound requests", &pb.ListWorkloadsRequest_Filters{NodeName: "node-a"}, 5, map[string]int32{"all": 5, "waiting": 1, "success": 1, "abnormal": 2}},
		{"name scope", &pb.ListWorkloadsRequest_Filters{Name: "pending", Status: "success"}, 0, map[string]int32{"all": 1, "pending": 1}},
		{"all is the unfiltered count key", &pb.ListWorkloadsRequest_Filters{Status: "all"}, 7, map[string]int32{"all": 7, "pending": 1, "waiting": 1, "success": 2, "abnormal": 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: tc.filters})
			if err != nil {
				t.Fatal(err)
			}
			if reply.Total != tc.total || !reflect.DeepEqual(reply.StatusCounts, tc.want) {
				t.Fatalf("total = %d, counts = %v; want %d, %v", reply.Total, reply.StatusCounts, tc.total, tc.want)
			}
		})
	}
}

func TestWorkloadKeepsReservationsThatNoRequestRowRepresents(t *testing.T) {
	reserved := workloadTestContainer("other-vendor", biz.ContainerStatusWaiting)
	reserved.NodeName, reserved.NodeUID = "", ""
	represented := workloadTestContainer("represented", biz.ContainerStatusWaiting)
	represented.NodeName, represented.NodeUID = "", ""
	svc := workloadTestService([]*biz.Container{reserved, represented}, workloadTestPending("represented", workloadTestRequest("main", "regular", "1")))
	reply, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || reply.Total != 2 {
		t.Fatalf("reservations = %v, %v", reply, err)
	}
	rows := map[string]*pb.ContainerReply{}
	for _, row := range reply.Items {
		rows[row.AppName] = row
	}
	if row := rows["other-vendor"]; row == nil || row.Pending || row.Request != nil || row.AllocatedMem != 256 {
		t.Fatalf("unrepresented reservation hidden or changed: %v", row)
	}
	if row := rows["represented"]; row == nil || !row.Pending || row.Request == nil {
		t.Fatalf("pending projection must replace its reservation: %v", row)
	}
}

type workloadFailingNodes struct{ containerTestNodeRepo }

func (*workloadFailingNodes) ListAll(context.Context) ([]*biz.Node, error) {
	return nil, errors.New("node cache unavailable")
}

func TestWorkloadNodeUIDFilterFailsWhenNodeCacheFails(t *testing.T) {
	pod := workloadTestPending("bound-request", workloadTestRequest("main", "regular", "1"))
	pod.NodeName, pod.Stage = "node-a", "bound"
	svc := workloadTestService(nil, pod)
	svc.containers.node = biz.NewNodeUsecase(&workloadFailingNodes{}, log.DefaultLogger)
	if _, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{Filters: &pb.ListWorkloadsRequest_Filters{NodeUid: "node-a-uid"}}); err == nil {
		t.Fatal("a node UID filter must not silently drop rows whose node UID is unknown")
	}
	reply, err := svc.ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || reply.Total != 1 {
		t.Fatalf("list without a node UID filter = %v, %v", reply, err)
	}
}

func TestWorkloadAllocationMatchesRequestRegardlessOfContainerKind(t *testing.T) {
	allocated := workloadTestContainer("sidecar-pod", biz.ContainerStatusSuccess)
	allocated.Name = "helper"
	pod := workloadTestPending("sidecar-pod", workloadTestRequest("helper", "sidecar", "1"))
	pod.NodeName, pod.Stage = "node-a", "bound"
	reply, err := workloadTestService([]*biz.Container{allocated}, pod).ListWorkloads(context.Background(), &pb.ListWorkloadsRequest{})
	if err != nil || reply.Total != 1 || reply.Items[0].Request != nil || reply.Items[0].AllocatedMem != 256 {
		t.Fatalf("allocation and request for one container = %v, %v", reply, err)
	}
}
