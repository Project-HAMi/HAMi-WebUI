package mthreads

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"vgpu/internal/provider/util"
)

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
	// sliced card).
	NodeSGPUCoresResource  = "mthreads.com/sgpu-core"
	NodeSGPUMemoryResource = "mthreads.com/sgpu-memory"

	// NodeWholeGPUResource is the vendor whole-card delivery resource.
	NodeWholeGPUResource = "mthreads.com/gpu"

	// SGPUCoresLabel lists the physical card ids bound to sgpu_km,
	// e.g. "0-2-3" for cards 0, 2 and 3.
	SGPUCoresLabel = "mthreads.com/sgpu.cores"

	// GPUCountLabel is the vendor per-node card count label.
	GPUCountLabel = "mthreads.com/gpu.count"

	// CoresPerCard is the vendor core-unit granularity of one physical card.
	CoresPerCard = int64(16)

	// MemoryFactorMiB converts one vendor memory unit into MiB.
	MemoryFactorMiB = int64(512)
)

func init() {
	util.InRequestDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-to-allocate"
	util.SupportDevices[MthreadsDevice] = "hami.io/mthreads-vgpu-devices-allocated"
}

// GPUMemoryLabelName returns the vendor per-card memory label key
// (value in bytes) for the given physical card id.
func GPUMemoryLabelName(id int64) string {
	return fmt.Sprintf("mthreads.com/gpu%d.memory", id)
}

// ParseCardIDList parses the vendor card-id list label. Accepts '-' or ','
// separators, deduplicates and drops unparsable or negative entries while
// preserving the first-seen order.
func ParseCardIDList(raw string) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '-' || r == ',' }) {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id < 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// PerCardMemoryMiB reports the framebuffer capacity of one physical card in
// MiB, derived from the sGPU pool when present (units x 512MiB / cards) and
// otherwise from the vendor per-card memory labels (bytes). It returns 0
// only when neither source is available.
func PerCardMemoryMiB(node *corev1.Node) int32 {
	cores, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUCoresResource), resource.DecimalSI).AsInt64()
	memUnits, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUMemoryResource), resource.DecimalSI).AsInt64()
	if cards := cores / CoresPerCard; cards > 0 && memUnits > 0 {
		return int32(memUnits * MemoryFactorMiB / cards)
	}
	var sum, cnt int64
	for id := int64(0); ; id++ {
		raw, ok := node.Labels[GPUMemoryLabelName(id)]
		if !ok {
			break
		}
		if b, err := strconv.ParseInt(raw, 10, 64); err == nil && b > 0 {
			sum += b
			cnt++
		}
	}
	if cnt == 0 {
		return 0
	}
	return int32(sum / cnt / (1024 * 1024))
}

// NodeCardInventory resolves the physical card layout of a node: the ids of
// the sGPU-sliced cards and the ids of the whole-card pool, plus the
// framebuffer capacity of one card in MiB.
//
// Sliced ids prefer the vendor sgpu.cores label (non-contiguous bindings
// such as gpu_ids=0,2,3 are common) and fall back to the contiguous 0..N-1
// derivation from the sgpu-core capacity. Whole-card ids are the complement
// within the vendor card count.
func NodeCardInventory(node *corev1.Node) (slicedIDs []int64, wholeIDs []int64, perCardMemMiB int64) {
	cores, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUCoresResource), resource.DecimalSI).AsInt64()
	memUnits, _ := node.Status.Capacity.Name(corev1.ResourceName(NodeSGPUMemoryResource), resource.DecimalSI).AsInt64()

	slicedIDs = ParseCardIDList(node.Labels[SGPUCoresLabel])
	if len(slicedIDs) == 0 && cores > 0 {
		for i := int64(0); i*CoresPerCard < cores; i++ {
			slicedIDs = append(slicedIDs, i)
		}
	}
	if len(slicedIDs) == 0 {
		return nil, nil, 0
	}

	total := int64(0)
	if raw, ok := node.Labels[GPUCountLabel]; ok {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
			total = v
		}
	}
	if total < int64(len(slicedIDs)) {
		total = int64(len(slicedIDs))
	}
	slicedSet := make(map[int64]bool, len(slicedIDs))
	for _, id := range slicedIDs {
		slicedSet[id] = true
	}
	for id := int64(0); id < total; id++ {
		if !slicedSet[id] {
			wholeIDs = append(wholeIDs, id)
		}
	}

	perCardMemMiB = 0
	if memUnits > 0 {
		perCardMemMiB = memUnits * MemoryFactorMiB / int64(len(slicedIDs))
	}
	return slicedIDs, wholeIDs, perCardMemMiB
}

var _ = fmt.Sprintf // keep fmt for future error wrapping
