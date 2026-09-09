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
	cores, ok := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUCoresResource), resource.DecimalSI).AsInt64()
	if !ok || cores <= 0 {
		return nil, nil
	}
	memoryUnits, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUMemoryResource), resource.DecimalSI).AsInt64()

	cards := cores / coresPerCard
	if cards <= 0 {
		return nil, nil
	}
	devmemPerCard := int32(memoryUnits * memoryFactor / cards)

	devices := make([]*util.DeviceInfo, 0, cards)
	for i := int64(0); i < cards; i++ {
		id := fmt.Sprintf("%s-mthreads-%d", node.Name, i)
		devices = append(devices, &util.DeviceInfo{
			ID:      id,
			AliasId: id,
			Index:   uint(i),
			Count:   100,
			Devmem:  devmemPerCard,
			Devcore: 100,
			Mode:    "sgpu",
			Type:    biz.MthreadsGPUDevice,
			Numa:    0,
			Health:  true,
		})
	}
	return devices, nil
}
