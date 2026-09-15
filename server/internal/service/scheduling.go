package service

import (
	"context"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
)

type SchedulingService struct {
	pb.UnimplementedSchedulingServer
	scheduling *biz.SchedulingUsecase
}

func NewSchedulingService(scheduling *biz.SchedulingUsecase) *SchedulingService {
	return &SchedulingService{scheduling: scheduling}
}

func (s *SchedulingService) GetSchedulingPod(ctx context.Context, req *pb.GetSchedulingPodRequest) (*pb.SchedulingPodReply, error) {
	detail, err := s.scheduling.GetSchedulingPod(ctx, req.GetNamespace(), req.GetName(), req.GetUid())
	if err != nil {
		return nil, err
	}
	result := &pb.SchedulingPodReply{
		Pod: schedulingPodReply(detail.Pod), Events: []*pb.SchedulingEvent{}, EventStatus: detail.Events.Status,
		EventsIncomplete: detail.Events.Incomplete, EventsFetchedAt: schedulingTime(detail.Events.FetchedAt),
	}
	for _, event := range detail.Events.Items {
		result.Events = append(result.Events, &pb.SchedulingEvent{
			Uid: event.UID, Reason: event.Reason, Type: event.Type, Message: event.Message,
			Source: event.Source, LastObservedAt: schedulingTime(event.LastObservedAt), Count: event.Count,
			MessageTruncated: event.MessageTruncated,
		})
	}
	return result, nil
}

func schedulingTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func schedulingPodReply(pod *biz.SchedulingPod) *pb.SchedulingPod {
	result := &pb.SchedulingPod{
		Name: pod.Name, Namespace: pod.Namespace, Uid: pod.UID, CreatedAt: schedulingTime(pod.CreatedAt),
		SchedulerName: pod.SchedulerName, NodeName: pod.NodeName, Stage: pod.Stage,
		ReasonCodes: pod.ReasonCodes, ReasonSource: pod.ReasonSource, Gates: pod.Gates,
		Preallocated: pod.Preallocated, AllocatedContainers: pod.AllocatedContainers,
		Requests: []*pb.SchedulingContainerRequest{}, Constraints: []*pb.SchedulingConstraint{},
	}
	if condition := pod.Condition; condition != nil {
		result.Condition = &pb.SchedulingCondition{Status: condition.Status, Reason: condition.Reason,
			Message: condition.Message, TransitionAt: schedulingTime(condition.TransitionAt), MessageTruncated: condition.MessageTruncated}
	}
	for _, request := range pod.Requests {
		item := &pb.SchedulingContainerRequest{Container: request.Container, ContainerKind: request.ContainerKind}
		for _, resource := range request.Resources {
			item.Resources = append(item.Resources, &pb.SchedulingResource{Name: resource.Name, Value: resource.Value, Unit: resource.Unit, Kind: resource.Kind, Vendor: resource.Vendor})
		}
		result.Requests = append(result.Requests, item)
	}
	for _, constraint := range pod.Constraints {
		result.Constraints = append(result.Constraints, &pb.SchedulingConstraint{Name: constraint.Name, Value: constraint.Value})
	}
	return result
}
