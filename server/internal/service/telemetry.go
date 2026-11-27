package service

import (
	"context"
	"errors"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/telemetry"
)

type TelemetryService struct {
	pb.UnimplementedTelemetryServer
	reader *telemetry.HistoricalObservationReader
}

func NewTelemetryService(promClient *prom.Client) *TelemetryService {
	return NewTelemetryServiceWithReader(telemetry.NewHistoricalObservationReader(promClient))
}

func NewTelemetryServiceWithReader(reader *telemetry.HistoricalObservationReader) *TelemetryService {
	return &TelemetryService{reader: reader}
}

func (s *TelemetryService) GetTrends(ctx context.Context, req *pb.TrendsRequest) (*pb.TrendsReply, error) {
	if s.reader == nil {
		return nil, kratoserrors.InternalServer("TELEMETRY_READER_UNAVAILABLE", "Historical telemetry reader is not configured")
	}
	reply, err := s.reader.Trends(ctx, req)
	if err != nil {
		if errors.Is(err, telemetry.ErrInvalidTrendsQuery) {
			return nil, kratoserrors.BadRequest("INVALID_ARGUMENT", err.Error())
		}
		return nil, err
	}
	return reply, nil
}
