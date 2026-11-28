package service

import (
	"context"
	"fmt"
	"math"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/jinzhu/copier"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type MonitorService struct {
	promClient  *prom.Client
	nodeUsecase *biz.NodeUsecase
	podUsecase  *biz.PodUseCase

	pb.UnimplementedMonitorServer
}

func NewMonitorService(
	promClient *prom.Client,
	nodeUsecase *biz.NodeUsecase,
	podUsecase *biz.PodUseCase,
) *MonitorService {
	return &MonitorService{
		promClient:  promClient,
		nodeUsecase: nodeUsecase,
		podUsecase:  podUsecase,
	}
}

const (
	maxRangePointsPerSeries int64 = 11000
	maxRangeTotalPoints     int64 = 200000
)

func (s *MonitorService) QueryRange(ctx context.Context, req *pb.QueryRangeRequest) (*pb.RangeResponse, error) {
	if req.GetRange() == nil {
		return nil, pb.ErrorTransformError("range is required")
	}
	startTime, err := parseRangeTime(req.Range.GetStart())
	if err != nil {
		return nil, pb.ErrorTransformError("%s", err)
	}
	endTime, err := parseRangeTime(req.Range.GetEnd())
	if err != nil {
		return nil, pb.ErrorTransformError("%s", err)
	}
	step, err := time.ParseDuration(req.Range.GetStep())
	if err != nil {
		return nil, pb.ErrorTransformError("%s", err)
	}
	if step <= 0 {
		return nil, pb.ErrorTransformError("step must be positive")
	}
	if endTime.Before(startTime) {
		return nil, pb.ErrorTransformError("end time must not be before start time")
	}
	steps := int64(endTime.Sub(startTime) / step)
	if steps >= maxRangePointsPerSeries {
		return nil, pb.ErrorTransformError("range query exceeds maximum %d points per series", maxRangePointsPerSeries)
	}
	pointsPerSeries := steps + 1

	value, err := s.promClient.QueryRange(ctx, req.GetQuery(), v1.Range{Start: startTime, End: endTime, Step: step})
	if err != nil {
		return nil, pb.ErrorVgpuDomainError("%s", err)
	}
	matrixValue, ok := value.(model.Matrix)
	if !ok {
		return nil, pb.ErrorTransformError("Error casting result to model.Matrix")
	}
	var res = &pb.RangeResponse{}
	copier.Copy(&res.Data, &matrixValue)
	if len(res.Data) > 0 && int64(len(res.Data)) > maxRangeTotalPoints/pointsPerSeries {
		return nil, pb.ErrorTransformError("range query exceeds maximum %d total points", maxRangeTotalPoints)
	}

	for _, sample := range res.Data {
		filled, err := fillLessSamplePoint(ctx, startTime, endTime, step, sample.Values)
		if err != nil {
			return nil, pb.ErrorVgpuDomainError("%s", err)
		}
		sample.Values = filled
	}
	return res, nil
}

func parseRangeTime(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, nil
	}
	// Preserve the server-local interpretation used by older clients.
	return time.ParseInLocation(time.DateTime, value, time.Local)
}

func fillLessSamplePoint(ctx context.Context, startTime, endTime time.Time, step time.Duration, values []*pb.SamplePair) ([]*pb.SamplePair, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	existingPoints := make(map[int64]float32, len(values))
	for _, pair := range values {
		existingPoints[pair.Timestamp] = pair.Value
	}

	var filledValues []*pb.SamplePair
	currentTime := getSamplePointStartTime(startTime, step, values)
	for iterations := 0; !currentTime.After(endTime); iterations++ {
		if iterations&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if int64(len(filledValues)) >= maxRangePointsPerSeries {
			return nil, fmt.Errorf("expanded series exceeds maximum %d points", maxRangePointsPerSeries)
		}
		currentTimestamp := currentTime.UnixMilli()
		if value, exists := existingPoints[currentTimestamp]; exists {
			filledValues = append(filledValues, &pb.SamplePair{Value: value, Timestamp: currentTimestamp, Missing: false})
		} else {
			filledValues = append(filledValues, &pb.SamplePair{Value: 0, Timestamp: currentTimestamp, Missing: true})
		}
		currentTime = currentTime.Add(step)
	}
	return filledValues, nil
}

// Compatible with both Prometheus and VictoriaMetrics
func getSamplePointStartTime(startTime time.Time, step time.Duration, values []*pb.SamplePair) time.Time {
	if len(values) == 0 {
		return startTime
	}
	startTimeMilli := startTime.UnixMilli()
	firstValue := values[0]
	// Case: Prometheus
	if firstValue.Timestamp == startTimeMilli {
		return startTime
	}
	// Case: VictoriaMetrics
	stepMilli := step.Milliseconds()
	// exists startTime data point
	if math.Abs(float64(firstValue.Timestamp-startTimeMilli)) <= float64(stepMilli) {
		return time.UnixMilli(firstValue.Timestamp)
	}
	// data points within the query time range
	stepCount := (firstValue.Timestamp - startTimeMilli) / stepMilli
	return time.UnixMilli(firstValue.Timestamp - stepCount*stepMilli)
}

func (s *MonitorService) QueryInstant(ctx context.Context, req *pb.QueryInstantRequest) (*pb.InstantResponse, error) {
	at := time.Now()
	if requested := req.GetTime(); requested != "" {
		parsed, err := time.Parse(time.RFC3339Nano, requested)
		if err != nil {
			return nil, errors.BadRequest("INVALID_ARGUMENT", "time must be an RFC3339 timestamp")
		}
		at = parsed
	}
	value, err := s.promClient.QueryAt(ctx, req.GetQuery(), at)
	if err != nil {
		return nil, pb.ErrorVgpuDomainError("%s", err)
	}
	vectorValue, ok := value.(model.Vector)
	if !ok {
		return nil, pb.ErrorTransformError("Error casting result to model.Vector")
	}
	var res = &pb.InstantResponse{}
	copier.Copy(&res.Data, &vectorValue)
	return res, nil
}
