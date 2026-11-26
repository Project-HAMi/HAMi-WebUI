package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

type SummaryRepo interface {
}

type SummaryUseCase struct {
	node NodeRepo
	pod  PodRepo
	log  *log.Helper
}

type SummaryInfo struct {
	VgpuUsed      int32
	VgpuTotal     int32
	CoreUsed      int32
	CoreUsedKnown bool
	CoreTotal     int32
	MemoryUsed    int32
	MemoryTotal   int32
	GpuCount      int32
	NodeCount     int32
}

func NewSummaryUseCase(node NodeRepo, pod PodRepo, logger log.Logger) *SummaryUseCase {
	return &SummaryUseCase{node: node, pod: pod, log: log.NewHelper(logger)}
}

func (t *SummaryUseCase) GetGPUSummary(ctx context.Context, deviceId string, nodeUID string, deviceType string) (SummaryInfo, error) {
	res := SummaryInfo{CoreUsedKnown: true}
	deviceInfos, err := t.node.ListAllDevices(ctx)
	if err != nil {
		return res, err
	}
	containers, err := t.pod.ListAll(ctx)
	if err != nil {
		return res, err
	}
	reservations := AggregateDeviceReservations(containers)
	seenNodes := make(map[string]struct{})
	for _, device := range deviceInfos {
		if deviceType != "" && deviceType != device.Type {
			continue
		}
		if nodeUID != "" && nodeUID != device.NodeUid {
			continue
		}
		if deviceId != "" && deviceId != device.Id {
			continue
		}
		// Existing allocations still count as used.
		if !device.Unconfigured {
			res.CoreTotal += PhysicalCoreBaselinePerDevice
			res.MemoryTotal = res.MemoryTotal + device.Devmem
			res.VgpuTotal = res.VgpuTotal + device.Count
		}

		reservation := reservations[device.AliasId]
		res.CoreUsed += reservation.Cores
		if reservation.CoreUnknown {
			res.CoreUsedKnown = false
		}
		res.MemoryUsed += reservation.MemoryMiB
		res.VgpuUsed += reservation.Slots
		res.GpuCount++
		seenNodes[device.NodeUid] = struct{}{}
	}
	res.NodeCount = int32(len(seenNodes))
	return res, nil
}
