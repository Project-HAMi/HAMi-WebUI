package mthreads

import "vgpu/internal/provider/util"

const (
	// MthreadsDevice is the provider/common word used by HAMi for Moore
	// Threads GPUs ("Mthreads"). It covers sGPU-sliceable cards.
	MthreadsDevice = "Mthreads"

	// MthreadsWholeGPUType labels whole-card (non-sliced) GPUs delivered
	// through the vendor mthreads.com/gpu resource outside HAMi scheduling.
	MthreadsWholeGPUType = "Mthreads-GPU"

	// NodeSGPUCoresResource / NodeSGPUMemoryResource are the extended
	// resources advertised by the Moore Threads vendor device plugin that
	// HAMi accounts against (16 core units and N x 512MiB memory units per
	// physical card).
	NodeSGPUCoresResource  = "mthreads.com/sgpu-core"
	NodeSGPUMemoryResource = "mthreads.com/sgpu-memory"

	// NodeWholeGPUResource is the vendor whole-card delivery resource.
	NodeWholeGPUResource = "mthreads.com/gpu"

	// defaultPerCardMemoryMiB is only used for whole-card reporting on
	// nodes that expose no sGPU pool from which the per-card memory could
	// be derived.
	defaultPerCardMemoryMiB = 0

	// CoresPerCard is the vendor core-unit granularity of one physical card.
	CoresPerCard = 16
	// MemoryFactorMiB converts one vendor memory unit into MiB.
	MemoryFactorMiB = 512
)

func init() {
	util.InRequestDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-to-allocate"
	util.SupportDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-allocated"
}
