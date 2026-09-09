package mthreads

import (
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

// Mthreads implements provider.Provider for Moore Threads GPUs.
//
// Device inventory is derived from the node extended resources advertised by
// the vendor device plugin (mthreads.com/sgpu-core, mthreads.com/sgpu-memory)
// using the same device-id convention as HAMi's scheduler
// (<node>-mthreads-<index>), so pod allocation annotations match without any
// Prometheus dependency.
type Mthreads struct {
	prom *prom.Client
	log  *log.Helper

	labelSelector string
}

func NewMthreads(promCl *prom.Client, log *log.Helper, labelSelector string) *Mthreads {
	return &Mthreads{
		prom:          promCl,
		log:           log,
		labelSelector: labelSelector,
	}
}

func (m *Mthreads) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(m.labelSelector)
}

func (m *Mthreads) GetProvider() string {
	return biz.MthreadsGPUDevice
}

func (m *Mthreads) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	cores, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUCoresResource), resource.DecimalSI).AsInt64()
	memoryUnits, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUMemoryResource), resource.DecimalSI).AsInt64()
	wholeGPUs, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeWholeGPUResource), resource.DecimalSI).AsInt64()

	devices := make([]*util.DeviceInfo, 0, 8)

	// sGPU-sliceable cards, scheduled and accounted by HAMi.
	cards := cores / CoresPerCard
	var devmemPerCard int64
	if cards > 0 {
		devmemPerCard = memoryUnits * MemoryFactorMiB / cards
		for i := int64(0); i < cards; i++ {
			id := fmt.Sprintf("%s-mthreads-%d", node.Name, i)
			devices = append(devices, &util.DeviceInfo{
				ID:      id,
				AliasId: id,
				Index:   uint(i),
				Count:   100,
				Devmem:  int32(devmemPerCard),
				Devcore: 100,
				Mode:    "sgpu",
				Type:    biz.MthreadsGPUDevice,
				Numa:    0,
				Health:  true,
			})
		}
	}

	// Whole-card GPUs, delivered directly by the vendor stack through
	// mthreads.com/gpu (outside HAMi scheduling). They are reported for
	// capacity visibility; per-card usage is not accounted by HAMi.
	perCardDevmem := devmemPerCard
	if perCardDevmem == 0 {
		perCardDevmem = int64(defaultPerCardMemoryMiB)
	}
	for j := int64(0); j < wholeGPUs; j++ {
		id := fmt.Sprintf("%s-mthreads-full-%d", node.Name, j)
		devices = append(devices, &util.DeviceInfo{
			ID:      id,
			AliasId: id,
			Index:   uint(j),
			Count:   1,
			Devmem:  int32(perCardDevmem),
			Devcore: 100,
			Mode:    "full",
			Type:    MthreadsWholeGPUType,
			Numa:    0,
			Health:  true,
		})
	}
	return devices, nil
}
