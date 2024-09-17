package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/gookit/goutil/arrutil"
)

type SummaryRepo interface {
}

type SummaryUseCase struct {
	node NodeRepo
	pod  PodRepo
	log  *log.Helper
}

type SummaryInfo struct {
	VgpuUsed    int32
	VgpuTotal   int32
	CoreUsed    int32
	CoreTotal   int32
	MemoryUsed  int32
	MemoryTotal int32
	GpuCount    int32
	NodeCount   int32
}

func NewSummaryUseCase(node NodeRepo, pod PodRepo, logger log.Logger) *SummaryUseCase {
	return &SummaryUseCase{node: node, pod: pod, log: log.NewHelper(logger)}
}

func (t *SummaryUseCase) GetGPUSummary(ctx context.Context, deviceId string, nodeUID string, deviceType string) (SummaryInfo, error) {
	var res SummaryInfo
	deviceInfos, err := t.node.ListAllDevices(ctx)
	if err != nil {
		return res, err
	}
	containers, err := t.pod.ListAll(ctx)
	if err != nil {
		return res, err
	}
	var nodeList []string
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
		res.CoreTotal = res.CoreTotal + device.Devcore
		res.MemoryTotal = res.MemoryTotal + device.Devmem
		res.VgpuTotal = res.VgpuTotal + device.Count

		vGPU, core, memory := ContainersStatisticsInfo(containers, device.AliasId)
		res.CoreUsed = res.CoreUsed + core
		res.MemoryUsed = res.MemoryUsed + memory
		res.VgpuUsed = res.VgpuUsed + vGPU
		res.GpuCount++
		nodeList = append(nodeList, device.NodeUid)
	}
	nodeList = arrutil.Unique(nodeList)
	res.NodeCount = int32(len(nodeList))
	return res, nil
}
