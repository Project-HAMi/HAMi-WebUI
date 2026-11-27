package service

import (
	"context"
	"errors"
	"strings"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"

	pb "vgpu/api/observability/v1"
	v1 "vgpu/api/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/telemetry"
)

type TelemetryService struct {
	pb.UnimplementedTelemetryServer
	reader     *telemetry.HistoricalObservationReader
	containers *ContainerService
}

func NewTelemetryService(promClient *prom.Client, containers *ContainerService) *TelemetryService {
	return &TelemetryService{
		reader:     telemetry.NewHistoricalObservationReader(promClient),
		containers: containers,
	}
}

func NewTelemetryServiceWithReader(reader *telemetry.HistoricalObservationReader) *TelemetryService {
	return &TelemetryService{reader: reader}
}

func containerExpectedAllocation(ctr *v1.ContainerReply) (int, int) {
	if ctr == nil {
		return 0, 0
	}
	vgpuCount := int(ctr.GetAllocatedDevices())
	deviceIDs := ctr.GetDeviceIds()
	if vgpuCount <= 0 || len(deviceIDs) != vgpuCount {
		return 0, 0
	}
	unique := make(map[string]struct{}, len(deviceIDs))
	for _, rawID := range deviceIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return 0, 0
		}
		unique[id] = struct{}{}
	}
	return len(unique), vgpuCount
}

func (s *TelemetryService) expectedContainerAllocation(ctx context.Context, req *pb.TrendsRequest) (int, int) {
	if s.containers == nil || req == nil || strings.TrimSpace(req.GetScope()) != "container" {
		return 0, 0
	}
	namespace := strings.TrimSpace(req.GetNamespace())
	pod := strings.TrimSpace(req.GetPod())
	containerName := strings.TrimSpace(req.GetContainer())
	podUID := strings.TrimSpace(req.GetPodUid())
	if namespace == "" || pod == "" || containerName == "" || podUID == "" {
		return 0, 0
	}
	ctr, err := s.containers.GetContainer(ctx, &v1.GetContainerReq{
		PodUid: podUID,
		Name:   containerName,
	})
	if err != nil || ctr == nil {
		return 0, 0
	}
	if ctr.GetPodUid() != podUID || ctr.GetName() != containerName || ctr.GetNamespace() != namespace || ctr.GetAppName() != pod {
		return 0, 0
	}
	return containerExpectedAllocation(ctr)
}

func (s *TelemetryService) GetTrends(ctx context.Context, req *pb.TrendsRequest) (*pb.TrendsReply, error) {
	if s.reader == nil {
		return nil, kratoserrors.InternalServer("TELEMETRY_READER_UNAVAILABLE", "Historical telemetry reader is not configured")
	}
	expectedDeviceCount, expectedVgpuCount := s.expectedContainerAllocation(ctx, req)
	var (
		reply *pb.TrendsReply
		err   error
	)
	if expectedDeviceCount > 0 {
		reply, err = s.reader.TrendsWithExpectedAllocation(ctx, req, expectedDeviceCount, expectedVgpuCount)
	} else {
		reply, err = s.reader.Trends(ctx, req)
	}
	if err != nil {
		if errors.Is(err, telemetry.ErrInvalidTrendsQuery) {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", err.Error())
		}
		return nil, err
	}
	return reply, nil
}
