package mthreads

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/go-kratos/kratos/v2/log"

	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

// Mthreads implements provider.Provider for Moore Threads GPUs.
//
// Device inventory is derived from vendor-published node state only: the
// sgpu.cores / sgpu.memory extended resources (sliced pool), the gpu.count
// and per-card memory labels (physical cards), so no Prometheus queries are
// needed and the view matches what the HAMi scheduler sees.
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
	slicedIDs, wholeIDs, perCardMemMiB := NodeCardInventory(node)

	devices := make([]*util.DeviceInfo, 0, len(slicedIDs)+len(wholeIDs))
	for _, cardID := range slicedIDs {
		id := fmt.Sprintf("%s-mthreads-%d", node.Name, cardID)
		devices = append(devices, &util.DeviceInfo{
			ID:      id,
			AliasId: id,
			Index:   uint(cardID),
			Count:   100,
			Devmem:  int32(perCardMemMiB),
			Devcore: 100,
			Mode:    "sgpu",
			Type:    biz.MthreadsGPUDevice,
			Numa:    0,
			Health:  true,
		})
	}
	// Whole-card GPUs are delivered directly by the vendor stack through
	// mthreads.com/gpu (outside HAMi scheduling). They carry their real
	// physical ordinal so DCGM-style telemetry maps 1:1.
	for _, cardID := range wholeIDs {
		id2 := fmt.Sprintf("%s-mthreads-full-%d", node.Name, cardID)
		devices = append(devices, &util.DeviceInfo{
			ID:      id2,
			AliasId: id2,
			Index:   uint(cardID),
			Count:   1,
			Devmem:  int32(perCardMemMiB),
			Devcore: 100,
			Mode:    "full",
			Type:    MthreadsWholeGPUType,
			Numa:    0,
			Health:  true,
		})
	}
	return devices, nil
}
