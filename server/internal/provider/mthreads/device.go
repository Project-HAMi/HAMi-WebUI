package mthreads

import "vgpu/internal/provider/util"

const (
	// MthreadsDevice is the provider/common word used by HAMi for Moore
	// Threads GPUs ("Mthreads").
	MthreadsDevice = "Mthreads"

	// NodeSGPUCoresResource / NodeSGPUMemoryResource are the extended
	// resources advertised by the Moore Threads vendor device plugin that
	// HAMi accounts against (16 core units and N x 512MiB memory units per
	// physical card).
	NodeSGPUCoresResource  = "mthreads.com/sgpu-core"
	NodeSGPUMemoryResource = "mthreads.com/sgpu-memory"

	// coresPerCard is the vendor core-unit granularity of one physical card.
	coresPerCard = 16
	// memoryFactor converts one vendor memory unit into MiB.
	memoryFactor = 512
)

func init() {
	util.InRequestDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-to-allocate"
	util.SupportDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-allocated"
}
