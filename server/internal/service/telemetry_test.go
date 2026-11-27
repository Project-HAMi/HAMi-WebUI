package service

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	obsv1 "vgpu/api/observability/v1"
	"vgpu/internal/biz"
	"vgpu/internal/telemetry"
)

type telemetryTestPodRepo struct {
	containerTestPodRepo
}

func (r *telemetryTestPodRepo) FindOne(_ context.Context, podUID, name string) (*biz.Container, error) {
	for _, item := range r.containers {
		if item.PodUID == podUID && item.Name == name {
			return item, nil
		}
	}
	return nil, nil
}

type telemetryTestQuerier struct {
	queries []string
}

func (q *telemetryTestQuerier) QueryRange(_ context.Context, query string, _ promv1.Range) (model.Value, error) {
	q.queries = append(q.queries, query)
	return model.Matrix{}, nil
}

func TestTelemetryServiceGetTrendsEnforcesContainerExpectedAllocation(t *testing.T) {
	querier := &telemetryTestQuerier{}
	containers := NewContainerService(
		biz.NewNodeUsecase(&containerTestNodeRepo{}, log.DefaultLogger),
		biz.NewPodUseCase(&telemetryTestPodRepo{
			containerTestPodRepo: containerTestPodRepo{
				containers: []*biz.Container{
					{
						Name:      "worker",
						PodName:   "trainer-0",
						PodUID:    "pod-shared",
						Namespace: "ml",
						ContainerDevices: biz.ContainerDevices{
							{UUID: "gpu-0"},
							{UUID: "gpu-0"},
						},
					},
				},
			},
		}, log.DefaultLogger),
	)
	svc := &TelemetryService{
		reader:     telemetry.NewHistoricalObservationReaderWithQuerier(querier),
		containers: containers,
	}

	if _, err := svc.GetTrends(context.Background(), &obsv1.TrendsRequest{
		Scope:     "container",
		Namespace: "ml",
		Pod:       "trainer-0",
		Container: "worker",
		PodUid:    "pod-shared",
		Start:     "2026-01-01T10:00:00Z",
		End:       "2026-01-01T11:00:00Z",
	}); err != nil {
		t.Fatalf("GetTrends(live shared container) error = %v", err)
	}
	if len(querier.queries) != 2 {
		t.Fatalf("live shared container queries = %d, want 2", len(querier.queries))
	}
	for _, query := range querier.queries {
		if !strings.Contains(query, ") == 1)") || !strings.Contains(query, ") == 2)") {
			t.Fatalf("shared device container should enforce unique device count 1 and vGPU sum 2: %s", query)
		}
	}

	// Historical/unmatched containers fall back to device-set matching cleanly.
	querier.queries = nil
	if _, err := svc.GetTrends(context.Background(), &obsv1.TrendsRequest{
		Scope:     "container",
		Namespace: "ml",
		Pod:       "trainer-0",
		Container: "worker",
		PodUid:    "pod-historical",
		Start:     "2026-01-01T10:00:00Z",
		End:       "2026-01-01T11:00:00Z",
	}); err != nil {
		t.Fatalf("GetTrends(historical container) error = %v", err)
	}
	if len(querier.queries) != 2 || !strings.Contains(querier.queries[0], "== count(") {
		t.Fatalf("historical fallback queries = %v, want count == count guard", querier.queries)
	}
}
