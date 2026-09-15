package biz

import (
	"context"
	"time"
)

// Read-only; never part of allocation accounting.
type SchedulingPod struct {
	Name, Namespace, UID, SchedulerName, NodeName, Stage string
	CreatedAt                                            time.Time
	ReasonCodes                                          []string
	ReasonSource                                         string
	Condition                                            *SchedulingCondition
	Requests                                             []SchedulingContainerRequest
	Gates                                                []string
	Constraints                                          []SchedulingConstraint
	Preallocated                                         bool
	AllocatedContainers                                  []string
}

type SchedulingCondition struct {
	Status, Reason, Message string
	TransitionAt            time.Time
	MessageTruncated        bool
}

type SchedulingContainerRequest struct {
	Container, ContainerKind string
	// Observed state for the workload list, not allocations.
	Status       string
	StatusDetail *ContainerStatusDetail
	Resources    []SchedulingResource
}

type SchedulingResource struct {
	Name, Value, Unit, Kind, Vendor string
}

type SchedulingConstraint struct{ Name, Value string }

type SchedulingEvent struct {
	UID, Reason, Type, Message, Source string
	LastObservedAt                     time.Time
	Count                              int32
	MessageTruncated                   bool
}

type SchedulingEvents struct {
	Items      []SchedulingEvent
	Status     string
	Incomplete bool
	FetchedAt  time.Time
}

type SchedulingDetail struct {
	Pod    *SchedulingPod
	Events SchedulingEvents
}

type SchedulingRepo interface {
	ListSchedulingPods(context.Context) ([]*SchedulingPod, error)
	GetSchedulingPod(context.Context, string, string, string) (*SchedulingDetail, error)
}

type SchedulingUsecase struct{ repo SchedulingRepo }

func NewSchedulingUsecase(repo SchedulingRepo) *SchedulingUsecase {
	return &SchedulingUsecase{repo: repo}
}

func (uc *SchedulingUsecase) ListSchedulingPods(ctx context.Context) ([]*SchedulingPod, error) {
	return uc.repo.ListSchedulingPods(ctx)
}

func (uc *SchedulingUsecase) GetSchedulingPod(ctx context.Context, namespace, name, uid string) (*SchedulingDetail, error) {
	return uc.repo.GetSchedulingPod(ctx, namespace, name, uid)
}
