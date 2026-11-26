package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

// AssignedNodeAnnotation identifies the node a HAMi-scheduled Pod is assigned to.
const AssignedNodeAnnotation = util.AssignedNodeAnnotations

// PodAllocationContext provides immutable inputs for extracting Pod allocations.
type PodAllocationContext struct {
	Pod            *corev1.Pod
	Node           *corev1.Node
	Catalog        *devicecatalog.Snapshot
	ContainerNames []string
	Logger         *log.Helper
}

// PodAllocationResult carries extracted Pod allocation snapshots, unreadable hints,
// recognized annotation keys, and accelerator resource names for one dialect.
type PodAllocationResult struct {
	Allocations              []podfacts.AllocationSnapshot
	UnreadableAllocations    []podfacts.UnreadableAllocation
	RecognizedAnnotationKeys []string
	ResourceNames            []corev1.ResourceName
}

// AllocationDialect encapsulates vendor-specific Pod allocation annotation
// decoding and catalog resource-name discovery behind a uniform contract.
type AllocationDialect interface {
	// Name returns the stable dialect identifier.
	Name() string
	// CatalogResourceNames returns catalog-configured Kubernetes resource names
	// belonging to this dialect for Pod candidate classification.
	CatalogResourceNames(catalog *devicecatalog.Snapshot) []corev1.ResourceName
	// ExtractPodAllocations extracts allocation facts from ctx.Pod and ctx.Node.
	ExtractPodAllocations(ctx PodAllocationContext) PodAllocationResult
}

// FactQuantity returns a known Pod allocation Quantity with decimal SI representation.
func FactQuantity(value int64, unit string) podfacts.Quantity {
	return podfacts.Quantity{
		Value: resource.NewQuantity(value, resource.DecimalSI),
		Unit:  unit,
		State: KnownState("hami_annotation", "container"),
	}
}

// FactString returns nil when value is empty, or a pointer to a copy otherwise.
func FactString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// RequestedSlots returns container slot indices (init containers first, then regular
// containers) requesting a positive quantity of countResource, matching HAMi Filter order.
func RequestedSlots(pod *corev1.Pod, countResource string) []int {
	if pod == nil || countResource == "" {
		return nil
	}
	key := corev1.ResourceName(countResource)
	var slots []int
	add := func(i int, c *corev1.Container) {
		q, ok := c.Resources.Limits[key]
		if !ok {
			q = c.Resources.Requests[key]
		}
		if q.Sign() > 0 {
			slots = append(slots, i)
		}
	}
	for i := range pod.Spec.InitContainers {
		add(i, &pod.Spec.InitContainers[i])
	}
	for i := range pod.Spec.Containers {
		add(i+len(pod.Spec.InitContainers), &pod.Spec.Containers[i])
	}
	return slots
}

// NewFactAllocationSnapshot initializes a Pod-peak allocation snapshot and marks
// identity_conflict when the assigned-node annotation is missing or disagrees with Spec.NodeName.
func NewFactAllocationSnapshot(pod *corev1.Pod, kind string) podfacts.AllocationSnapshot {
	source := &viewv1.SourceState{Source: "hami_annotation:" + kind, State: resourceview.Known}
	// Allocation context may precede binding. A conflicting bound node is not an
	// association we can repair using the Pod's requested/bound node.
	if assigned := pod.Annotations[AssignedNodeAnnotation]; assigned == "" || (pod.Spec.NodeName != "" && pod.Spec.NodeName != assigned) {
		source.State, source.Reason = resourceview.Unknown, "identity_conflict"
	}
	return podfacts.AllocationSnapshot{
		PodUID:          pod.UID,
		ResourceVersion: pod.ResourceVersion,
		Source:          source,
		Accounting:      podfacts.AccountingPodPeak,
	}
}

// AllocationLayout validates container-slot count and segment field formatting in a
// semicolon/colon-delimited HAMi allocation annotation string.
func AllocationLayout(raw string, count int) (valid, complete bool) {
	slots := strings.Split(raw, ";")
	if len(slots) > count && slots[len(slots)-1] == "" {
		slots = slots[:len(slots)-1]
	}
	complete = len(slots) == count
	valid = len(slots) <= count
	for _, slot := range slots {
		for _, entry := range strings.Split(slot, ":") {
			if entry == "" {
				continue
			}
			fields := strings.Split(entry, ",")
			if len(fields) < 4 || fields[0] == "" || fields[1] == "" {
				return false, false
			}
			for _, field := range fields[2:4] {
				v, err := strconv.ParseInt(field, 10, 32)
				if err != nil || v < 0 {
					return false, false
				}
			}
		}
	}
	return valid, complete
}

// UnreadableAllocations extracts card-scoped unreadable hints from an invalid
// HAMi allocation annotation string.
func UnreadableAllocations(providerName, raw string, normalizeID func(string) string) []podfacts.UnreadableAllocation {
	var hints []podfacts.UnreadableAllocation
	for _, record := range strings.FieldsFunc(raw, func(r rune) bool { return r == ':' || r == ';' }) {
		id, _, ok := strings.Cut(record, ",")
		if !ok || id == "" {
			return []podfacts.UnreadableAllocation{{Provider: providerName}}
		}
		if normalizeID != nil {
			id = normalizeID(id)
		}
		hints = append(hints, podfacts.UnreadableAllocation{Provider: providerName, DeviceID: id})
	}
	return hints
}

// CheckAllocationRows verifies requested container slots against encoded annotation rows
// and marks invalid sources as unverified while recording unreadable hints.
func CheckAllocationRows(
	pod *corev1.Pod,
	source *podfacts.AllocationSnapshot,
	unreadableProvider, countKey, raw string,
	valid bool,
	normalizeID func(string) string,
) []podfacts.UnreadableAllocation {
	var unreadable []podfacts.UnreadableAllocation
	rows := strings.Split(raw, ";")
	for _, slot := range RequestedSlots(pod, countKey) {
		if slot >= len(rows) || strings.Trim(rows[slot], ":") == "" {
			// The missing device cannot be scoped to a particular card or phase.
			unreadable = append(unreadable, podfacts.UnreadableAllocation{Provider: unreadableProvider})
			break
		}
	}
	if !valid {
		unreadable = append(unreadable, UnreadableAllocations(unreadableProvider, raw, normalizeID)...)
		source.Source.State, source.Source.Reason = resourceview.Unknown, "unverified"
		source.Complete = false
	}
	return unreadable
}

// StandardAllocationSpec parameterizes decoding for a standard semicolon/colon-delimited
// HAMi Pod allocation annotation.
type StandardAllocationSpec struct {
	Kind               string
	AnnotationKey      string
	UnreadableProvider string
	CountResource      string
	DecodeContainer    func(rawSlot, nodeName string) (util.ContainerDevices, error)
	NormalizeID        func(id string) string
	EnrichRecord       func(source *podfacts.AllocationSnapshot, record *podfacts.Allocation, slot, index int, device util.ContainerDevice)
}

// DecodeStandardPodAllocation decodes a standard HAMi Pod allocation annotation into
// a PodAllocationResult using the shared slot-layout and unreadable-hint contract.
func DecodeStandardPodAllocation(ctx PodAllocationContext, spec StandardAllocationSpec) PodAllocationResult {
	if ctx.Pod == nil || spec.AnnotationKey == "" {
		return PodAllocationResult{}
	}
	raw, exists := ctx.Pod.Annotations[spec.AnnotationKey]
	if !exists {
		return PodAllocationResult{}
	}
	unreadableProvider := spec.UnreadableProvider
	if unreadableProvider == "" {
		unreadableProvider = spec.Kind
	}
	decodeContainer := spec.DecodeContainer
	if decodeContainer == nil {
		decodeContainer = func(rawSlot, _ string) (util.ContainerDevices, error) {
			return util.DecodeContainerDevices(rawSlot, "")
		}
	}

	nodeName := ctx.Pod.Annotations[AssignedNodeAnnotation]
	slotStrings := strings.Split(raw, util.OnePodMultiContainerSplitSymbol)
	decoded := make(util.PodSingleDevice, 0, len(slotStrings))
	decodeOK := true
	for i, s := range slotStrings {
		if i >= len(ctx.ContainerNames) {
			break
		}
		if s == "" {
			decoded = append(decoded, util.ContainerDevices{})
			continue
		}
		cd, err := decodeContainer(s, nodeName)
		if err != nil {
			if ctx.Logger != nil {
				ctx.Logger.Infof("Decoded %s allocation failed on slot %d: %v", spec.Kind, i, err)
			}
			decoded = nil
			decodeOK = false
			break
		}
		decoded = append(decoded, cd)
	}

	source := NewFactAllocationSnapshot(ctx.Pod, spec.Kind)
	source.HasAllocationEvidence = strings.Trim(raw, ";:") != ""
	valid, complete := AllocationLayout(raw, len(ctx.ContainerNames))
	source.Complete = complete
	unreadable := CheckAllocationRows(ctx.Pod, &source, unreadableProvider, spec.CountResource, raw, valid && decodeOK, spec.NormalizeID)

	for slot, devices := range decoded {
		if slot >= len(ctx.ContainerNames) {
			source.Complete = false
			break
		}
		for index, device := range devices {
			record := podfacts.Allocation{
				Container:   ctx.ContainerNames[slot],
				RecordID:    fmt.Sprintf("%d/%d", slot, index),
				Node:        nodeName,
				Resource:    spec.Kind,
				DeviceID:    device.UUID,
				RawDeviceID: device.UUID,
				States:      map[string]*viewv1.FieldState{},
				Quantities: map[string]podfacts.Quantity{
					"memory":  FactQuantity(int64(device.Usedmem)*1024*1024, "bytes"),
					"compute": FactQuantity(int64(device.RecordedCores), "percent"),
					"slots":   FactQuantity(1, "slots"),
				},
			}
			if spec.EnrichRecord != nil {
				spec.EnrichRecord(&source, &record, slot, index, device)
			}
			if ctx.Node != nil {
				record.NodeUID = ctx.Node.UID
			}
			source.Records = append(source.Records, record)
		}
	}

	return PodAllocationResult{
		Allocations:              []podfacts.AllocationSnapshot{source},
		UnreadableAllocations:    unreadable,
		RecognizedAnnotationKeys: []string{spec.AnnotationKey},
		ResourceNames:            []corev1.ResourceName{corev1.ResourceName(spec.Kind)},
	}
}
