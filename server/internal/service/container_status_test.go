package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/go-kratos/kratos/v2/encoding"
	_ "github.com/go-kratos/kratos/v2/encoding/json"
	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

type statusTestPodRepo struct {
	containerTestPodRepo
}

func (r *statusTestPodRepo) FindOne(_ context.Context, podUID, name string) (*biz.Container, error) {
	for _, container := range r.containers {
		if container.PodUID == podUID && container.Name == name {
			return container, nil
		}
	}
	return nil, nil
}

func workloadStatusService(containers []*biz.Container) *ContainerService {
	return NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&statusTestPodRepo{containerTestPodRepo{containers: containers}}, log.DefaultLogger),
	)
}

func TestWorkloadStatusFiltersMatchReturnedStatusAndPreserveAllocationScope(t *testing.T) {
	states := []string{
		biz.ContainerStatusClosed, biz.ContainerStatusSuccess, biz.ContainerStatusTerminating,
		biz.ContainerStatusWaiting, biz.ContainerStatusNotReady, biz.ContainerStatusUnknown,
		biz.ContainerStatusFailed, biz.ContainerStatusError,
	}
	containers := make([]*biz.Container, 0, len(states)+2)
	for _, state := range append(states, "", "future-status") {
		containers = append(containers, &biz.Container{
			Name: state + "-worker", PodUID: state + "-pod", Status: state,
			ContainerDevices: biz.ContainerDevices{{UUID: "GPU-1", Usedmem: 1024, Usedcores: 10}},
		})
	}
	// A container without assigned devices is outside this endpoint's inventory,
	// regardless of whether it would otherwise match a visible status filter.
	containers = append(containers, &biz.Container{Name: "unassigned", Status: biz.ContainerStatusWaiting})
	service := workloadStatusService(containers)
	for _, state := range states {
		reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{
			Filters: &pb.GetAllContainersReq_Filters{Status: state},
		})
		if err != nil {
			t.Fatal(err)
		}
		wantCount := 1
		if state == biz.ContainerStatusUnknown {
			wantCount = 3
		}
		if len(reply.Items) != wantCount {
			t.Fatalf("filter %q returned %d rows, want %d", state, len(reply.Items), wantCount)
		}
		for _, row := range reply.Items {
			if row.Status != state || row.AllocatedMem != 1024 || row.AllocatedCores != 10 {
				t.Fatalf("filter and returned state/allocation disagree: %#v", row)
			}
		}
	}
	reply, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(reply.Items))
	for _, row := range reply.Items {
		got = append(got, row.Status)
	}
	want := []string{"error", "failed", "unknown", "unknown", "unknown", "not_ready", "waiting", "terminating", "success", "closed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status priority = %#v, want %#v", got, want)
	}
	for _, container := range containers {
		if container.Status != "" && container.Status != "future-status" {
			continue
		}
		detail, err := service.GetContainer(context.Background(), &pb.GetContainerReq{Name: container.Name, PodUid: container.PodUID})
		if err != nil || detail.Status != biz.ContainerStatusUnknown {
			t.Fatalf("detail must normalize unknown in the same way as list: %#v, %v", detail, err)
		}
	}
}

func TestWorkloadStatusDetailsMatchListAndDetailAndKeepOptionalZeroValues(t *testing.T) {
	ready, exitCode, previousExit := false, int32(0), int32(137)
	container := &biz.Container{
		Name: "worker", PodName: "training", PodUID: "pod-1", Status: biz.ContainerStatusClosed,
		ContainerDevices: biz.ContainerDevices{{UUID: "GPU-1"}},
		StatusDetail: &biz.ContainerStatusDetail{
			ContainerState: "Terminated", Reason: "Completed", Message: "all work completed",
			Ready: &ready, ExitCode: &exitCode, PodPhase: "Running", PodReady: "False",
			PodReadyReason: "ContainersNotReady", PodReadyMessage: "containers with unready status: [worker]",
			LastTerminationReason: "OOMKilled", LastExitCode: &previousExit, LastTerminationMessage: "previous execution",
		},
	}
	service := workloadStatusService([]*biz.Container{container})
	list, err := service.GetAllContainers(context.Background(), &pb.GetAllContainersReq{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list: %#v, %v", list, err)
	}
	detail, err := service.GetContainer(context.Background(), &pb.GetContainerReq{Name: "worker", PodUid: "pod-1"})
	if err != nil || !proto.Equal(list.Items[0].StatusDetail, detail.StatusDetail) {
		t.Fatalf("list and detail must expose identical status evidence: %#v, %v", detail, err)
	}
	for _, marshal := range []struct {
		name string
		fn   func() ([]byte, error)
	}{
		{"protobuf JSON", func() ([]byte, error) { return protojson.Marshal(detail) }},
		{"production HTTP JSON codec", func() ([]byte, error) { return encoding.GetCodec("json").Marshal(detail) }},
	} {
		t.Run(marshal.name, func(t *testing.T) {
			encoded, err := marshal.fn()
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			var evidence map[string]any
			if err := json.Unmarshal(payload["statusDetail"], &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence["ready"] != false || evidence["exitCode"] != float64(0) || evidence["lastExitCode"] != float64(137) {
				t.Fatalf("optional false/zero evidence lost in JSON: %s", encoded)
			}
			if evidence["reason"] != "Completed" || evidence["podReady"] != "False" || evidence["podPhase"] != "Running" {
				t.Fatalf("current container and Pod context lost in JSON: %s", encoded)
			}
			if marshal.name == "production HTTP JSON codec" && evidence["restartCount"] != float64(0) {
				t.Fatalf("HTTP codec must preserve zero restart count: %s", encoded)
			}
		})
	}

	container.StatusDetail = &biz.ContainerStatusDetail{PodPhase: "Pending"}
	detail, err = service.GetContainer(context.Background(), &pb.GetContainerReq{Name: "worker", PodUid: "pod-1"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encoding.GetCodec("json").Marshal(detail.StatusDetail)
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"ready", "exitCode", "lastExitCode"} {
		if _, present := evidence[field]; present {
			t.Fatalf("unobserved %s must remain absent rather than becoming false/zero: %s", field, encoded)
		}
	}
}
