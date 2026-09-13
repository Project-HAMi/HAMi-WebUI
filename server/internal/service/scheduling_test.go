package service

import (
	"context"
	"testing"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

type schedulingTestRepository struct {
	pods []*biz.SchedulingPod
}

func (r *schedulingTestRepository) ListSchedulingPods(context.Context) ([]*biz.SchedulingPod, error) {
	return r.pods, nil
}

func (r *schedulingTestRepository) GetSchedulingPod(context.Context, string, string, string) (*biz.SchedulingDetail, error) {
	return &biz.SchedulingDetail{Pod: r.pods[0], Events: biz.SchedulingEvents{Status: "forbidden", Incomplete: true}}, nil
}

func TestSchedulingDetailKeepsPodEvidenceWhenEventsDenied(t *testing.T) {
	repo := &schedulingTestRepository{pods: []*biz.SchedulingPod{{Name: "p", UID: "u", Stage: "waiting", ReasonCodes: []string{"CardInsufficientCore"},
		Condition: &biz.SchedulingCondition{Message: "CardInsufficientCore", TransitionAt: time.Unix(10, 0)},
		Requests:  []biz.SchedulingContainerRequest{{Container: "init", ContainerKind: "init", Resources: []biz.SchedulingResource{{Name: "custom.io/card", Value: "2", Kind: "raw"}}}},
	}}}
	svc := NewSchedulingService(biz.NewSchedulingUsecase(repo))
	reply, err := svc.GetSchedulingPod(context.Background(), &pb.GetSchedulingPodRequest{Namespace: "dev", Name: "p", Uid: "u"})
	if err != nil || reply.Pod.ReasonCodes[0] != "CardInsufficientCore" || reply.EventStatus != "forbidden" || !reply.EventsIncomplete {
		t.Fatalf("denied events erased Pod evidence: %#v, %v", reply, err)
	}
	if reply.EventsFetchedAt != "" || reply.Pod.CreatedAt != "" || reply.Pod.Requests[0].Resources[0].Unit != "" {
		t.Fatalf("unknown values were fabricated: %#v", reply)
	}
}
