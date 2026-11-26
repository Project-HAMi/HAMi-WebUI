package data

import (
	"context"
	"fmt"
	"maps"
	"math"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/provider/registry"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type podRepo struct {
	facts                   *podfacts.Ledger
	factsCatalog            devicecatalog.Source
	data                    *Data
	podLister               listerscorev1.PodLister
	nodeLister              listerscorev1.NodeLister
	pods                    map[k8stypes.UID]*biz.PodInfo
	mutex                   sync.RWMutex
	log                     *log.Helper
	podIndexer              cache.Indexer
	schedulingResourceNames map[corev1.ResourceName]struct{}
	schedulingEvents        *schedulingEventReader
	ascend                  ascend.Decoder
	allocationDialects      []provider.AllocationDialect
}

func (r *podRepo) factAllocationDialects() []provider.AllocationDialect {
	if len(r.allocationDialects) > 0 {
		return r.allocationDialects
	}
	return registry.DefaultAllocationDialects(registry.AllocationConfig{
		AscendAnnotationlessPolicy: r.ascend.Policy,
	})
}

func NewPodRepo(data *Data, logger log.Logger, config *conf.Bootstrap, catalog devicecatalog.Source) (*podRepo, error) {
	resources, err := schedulingResources(config.GetScheduling())
	if err != nil {
		return nil, err
	}
	eventsClient := data.eventsCl
	if eventsClient == nil {
		eventsClient = data.k8sCl
	}
	repo := &podRepo{
		facts: podfacts.NewLedger(), factsCatalog: catalog,
		data:                    data,
		pods:                    make(map[k8stypes.UID]*biz.PodInfo),
		log:                     log.NewHelper(logger),
		schedulingResourceNames: resources,
		schedulingEvents:        newSchedulingEventReader(eventsClient),
		ascend:                  ascend.Decoder{Catalog: catalog, Policy: config.GetAscend().GetAnnotationlessPodMode()},
	}
	if err := repo.init(catalog); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *podRepo) init(catalog devicecatalog.Source) error {
	// Pod events read their node, so the node cache is filled first.
	r.nodeLister = r.data.informers.Core().V1().Nodes().Lister()
	r.data.startInformers()

	pods := r.data.informers.Core().V1().Pods()
	r.podLister = pods.Lister()
	// After the lister exists and before the first sync, so no configuration change is missed.
	catalog.Subscribe(r.onCatalogChange)
	informer := pods.Informer()
	// An index on the existing informer, not another watch.
	if err := informer.AddIndexers(cache.Indexers{schedulingGPUIndex: r.schedulingIndex}); err != nil {
		return fmt.Errorf("index pods: %w", err)
	}
	r.podIndexer = informer.GetIndexer()
	if err := r.initFacts(informer, catalog); err != nil {
		return err
	}
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAddPod,
		UpdateFunc: r.onUpdatePod,
		DeleteFunc: r.onDeletedPod,
	}); err != nil {
		return fmt.Errorf("watch pods: %w", err)
	}
	r.data.startInformers()
	return nil
}

var allocationDecodeErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "hami_webui_allocation_decode_errors_total",
	Help: "Failed attempts to decode a Pod allocation annotation, by device type.",
}, []string{"device_type"})

func init() {
	prometheus.MustRegister(allocationDecodeErrors)
}

// client-go re-panics a handler panic, so one malformed object would otherwise restart the process.
func (r *podRepo) recoverEvent(event string, obj interface{}) {
	if p := recover(); p != nil {
		name := "unknown object"
		if pod, ok := obj.(*corev1.Pod); ok && pod != nil {
			name = pod.Namespace + "/" + pod.Name
		}
		r.log.Errorf("pod %s handler panicked for %s: %v\n%s", event, name, p, debug.Stack())
	}
}

func (r *podRepo) onAddPod(obj interface{}) {
	defer r.recoverEvent("add", obj)
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.log.Error("unknown add object type")
		return
	}
	nodeID, ok := pod.Annotations[util.AssignedNodeAnnotations]
	if !ok {
		return
	}
	if biz.IsPodInTerminatedState(pod) {
		r.delPod(pod)
		return
	}
	node := r.assignedNode(pod)
	info := r.decodePodInfo(pod, node, r.ascend.Snapshot())
	r.mutex.Lock()
	r.pods[pod.UID] = info
	r.mutex.Unlock()
	r.log.Infof("Pod added: Name: %s, UID: %s, Namespace: %s, NodeID: %s", pod.Name, pod.UID, pod.Namespace, nodeID)
}

func (r *podRepo) decodePodInfo(pod *corev1.Pod, node *corev1.Node, snapshot *devicecatalog.Snapshot) *biz.PodInfo {
	pod = nvidia.AlignPodAllocation(pod, snapshot)
	nodeUID := ""
	if node != nil && node.Name == pod.Spec.NodeName {
		nodeUID = string(node.UID)
	}
	podDev, failures := util.DecodePodDevices(pod, r.log)
	for _, failure := range failures {
		allocationDecodeErrors.WithLabelValues(failure.DeviceType).Inc()
		r.log.Warnf("cannot decode %s allocation of pod %s/%s: %v", failure.DeviceType, pod.Namespace, pod.Name, failure.Err)
	}
	gpus := nvidia.ReadAllocations(pod, nvidia.RegisteredModes(node, r.log))
	if err := gpus.Err(); err != nil {
		r.log.Warnf("ignore MIG reservations of pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	bizPodDev := bizPodDevices(gpus, podDev)
	decoder := ascend.Decoder{Catalog: devicecatalog.Static{Current: snapshot}, Policy: r.ascend.Policy}
	ascendDevices, err := decoder.Decode(pod, node)
	if err != nil {
		r.log.Errorf("cannot decode Ascend allocations for pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	for word, slots := range ascendDevices {
		bizPodDev[word] = ascendPodSingleDevice(slots)
	}
	return &biz.PodInfo{
		ResourceVersion: pod.ResourceVersion, AllocationRevision: factInputRevision(snapshot, node),
		Name: pod.Name, UID: pod.UID, Namespace: pod.Namespace, NodeID: pod.Annotations[util.AssignedNodeAnnotations],
		Devices: bizPodDev, Ctrs: r.fetchContainerInfo(pod, bizPodDev, nodeUID),
	}
}

// Explicit conversion: copier silently drops devices once the structs differ.
func bizPodDevices(gpus nvidia.Allocations, devices util.PodDevices) biz.PodDevices {
	result := make(biz.PodDevices, len(devices))
	for deviceType, slots := range devices {
		converted := make(biz.PodSingleDevice, len(slots))
		for i, slot := range slots {
			converted[i] = make(biz.ContainerDevices, len(slot))
			for j, device := range slot {
				allocated := biz.ContainerDevice{
					Idx:       device.Idx,
					UUID:      device.UUID,
					Type:      device.Type,
					Usedmem:   device.Usedmem,
					Usedcores: device.Usedcores,
					Priority:  device.Priority,
					// Non-Ascend providers key their allocations by vendor.
					Vendor: deviceType,
				}
				if deviceType == biz.NvidiaGPUDevice {
					gpu := gpus.Device(i, j, device.UUID)
					allocated.UUID = gpu.UUID
					allocated.Shape = nvidiaShapes[gpu.Mode]
					allocated.ShapeReason = gpu.Reason
					allocated.Template = gpu.Profile
					allocated.MigPlacement = biz.MigPlacement{Start: gpu.Start, Size: gpu.Size}
				}
				converted[i][j] = allocated
			}
		}
		result[deviceType] = converted
	}
	return result
}

var nvidiaShapes = map[string]string{
	nvidia.ModeHamiCore: biz.SplitShapeSoft,
	nvidia.ModeMig:      biz.SplitShapeMig,
	"":                  biz.SplitShapeUnknown,
}

func ascendPodSingleDevice(slots [][]ascend.Device) biz.PodSingleDevice {
	result := make(biz.PodSingleDevice, 0, len(slots))
	for _, slot := range slots {
		devices := make(biz.ContainerDevices, 0, len(slot))
		for _, device := range slot {
			facts := device.Facts
			devices = append(devices, biz.ContainerDevice{
				Idx:     device.Index,
				UUID:    device.UUID,
				Type:    facts.CommonWord,
				Usedmem: int32(facts.Memory),
				Vendor:  biz.AscendGPUDevice,
				Ascend: &biz.AscendFacts{
					AnnotatedCore: facts.AnnotatedCore,
					Template:      facts.Template,
					Recorded:      facts.Recorded,
					CardMemory:    facts.CardMemory,
					NodeRead:      facts.NodeRead,
					Mode:          facts.Mode,
					ModeReason:    facts.ModeReason,
				},
			})
		}
		result = append(result, devices)
	}
	return result
}

// Interpreting at read time applies configuration edits without decoding Pods again.
// Containers without Ascend devices are returned as they are, so clusters
// without NPUs copy nothing on every read.
func (r *podRepo) interpretContainer(snapshot *devicecatalog.Snapshot, container *biz.Container) *biz.Container {
	interprets := false
	for _, device := range container.ContainerDevices {
		if device.Ascend != nil {
			interprets = true
			break
		}
	}
	if !interprets {
		return container
	}
	interpreted := *container
	interpreted.ContainerDevices = make(biz.ContainerDevices, len(container.ContainerDevices))
	for i, device := range container.ContainerDevices {
		if device.Ascend != nil {
			result := ascend.Interpret(snapshot, ascend.Facts{
				CommonWord:    device.Type,
				Memory:        int64(device.Usedmem),
				AnnotatedCore: device.Ascend.AnnotatedCore,
				Template:      device.Ascend.Template,
				Recorded:      device.Ascend.Recorded,
				CardMemory:    device.Ascend.CardMemory,
				NodeRead:      device.Ascend.NodeRead,
				Mode:          device.Ascend.Mode,
				ModeReason:    device.Ascend.ModeReason,
			}, r.ascend.Policy)
			device.Usedcores, device.Shape, device.Template = result.Cores, result.Shape, result.Template
			device.CoreAllocationUnknown, device.CoreReason = !result.Known, result.Reason
		}
		interpreted.ContainerDevices[i] = device
	}
	return &interpreted
}

// Only newly configured words without the Ascend prefix were skipped when decoding.
func (r *podRepo) onCatalogChange(previous, current *devicecatalog.Snapshot) {
	var added []string
	for _, word := range current.AscendCommonWords() {
		if _, known := previous.AscendModel(word); !known && !strings.HasPrefix(word, "Ascend") {
			added = append(added, word)
		}
	}
	if len(added) == 0 {
		return
	}
	pods, err := r.podLister.List(labels.Everything())
	if err != nil {
		r.log.Warnf("list pods after a device configuration change: %v", err)
		return
	}
	for _, pod := range pods {
		for _, word := range added {
			if _, ok := pod.Annotations["hami.io/"+word+"-devices-allocated"]; ok {
				r.onAddPod(pod)
				break
			}
		}
	}
}

func (r *podRepo) onUpdatePod(old interface{}, new interface{}) {
	defer r.recoverEvent("update", new)
	previous, wasPod := old.(*corev1.Pod)
	current, isPod := new.(*corev1.Pod)
	// The informer keys by name, so a re-create the watch missed arrives as an update.
	if wasPod && isPod && previous.UID != current.UID {
		r.delPod(previous)
	}
	r.onAddPod(new)
}

func (r *podRepo) onDeletedPod(obj interface{}) {
	defer r.recoverEvent("delete", obj)
	// A delete the watch missed arrives wrapped once a relist finds the Pod gone.
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.log.Error("unknown delete object type")
		return
	}
	r.delPod(pod)
}

func (r *podRepo) delPod(pod *corev1.Pod) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	pi, ok := r.pods[pod.UID]
	if ok {
		r.log.Infof("Deleted pod %s with node ID %s", pi.Name, pi.NodeID)
		delete(r.pods, pod.UID)
	}
}

func (r *podRepo) fetchContainerInfo(pod *corev1.Pod, pdevices biz.PodDevices, nodeUID string) []*biz.Container {
	containers := []*biz.Container{}
	totalContainers := len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	bizContainerDevices := mergeContainerDevicesBySlot(totalContainers, pdevices)
	if len(pdevices) == 0 {
		return containers
	}

	containerStatuses := map[string]*corev1.ContainerStatus{}
	for i := range pod.Status.ContainerStatuses {
		ctr := &pod.Status.ContainerStatuses[i]
		containerStatuses[ctr.Name] = ctr
	}

	initContainerOffset := len(pod.Spec.InitContainers)
	for i, ctr := range pod.Spec.Containers {
		observed := containerStatuses[ctr.Name]
		status, statusDetail := classifyContainerStatus(pod, observed)
		containerID := ""
		if observed != nil {
			containerID = observed.ContainerID
		}
		deviceIdx := initContainerOffset + i
		var containerDevices biz.ContainerDevices
		if deviceIdx < len(bizContainerDevices) {
			containerDevices = bizContainerDevices[deviceIdx]
		}
		c := &biz.Container{
			Name:             ctr.Name,
			UUID:             containerID,
			ContainerIdx:     i,
			NodeName:         pod.Spec.NodeName,
			PodName:          pod.Name,
			PodUID:           string(pod.UID),
			Image:            ctr.Image,
			Status:           status,
			StatusDetail:     statusDetail,
			NodeUID:          nodeUID,
			Namespace:        pod.Namespace,
			CreateTime:       r.GetCreateTime(pod),
			ContainerDevices: containerDevices,
		}
		// Not every device type carries a priority, and those that do may not sort first.
		for _, device := range containerDevices {
			if device.Priority != "" {
				c.Priority = device.Priority
				break
			}
		}
		containers = append(containers, c)
	}
	return containers
}

func mergeContainerDevicesBySlot(totalContainers int, podDevices biz.PodDevices) []biz.ContainerDevices {
	containerDevices := make([]biz.ContainerDevices, totalContainers)
	// Sorted so a mixed-type container keeps a stable device order.
	for _, deviceType := range slices.Sorted(maps.Keys(podDevices)) {
		for i, devices := range podDevices[deviceType] {
			if i >= totalContainers {
				break
			}
			containerDevices[i] = append(containerDevices[i], devices...)
		}
	}
	return containerDevices
}

const nodeReadTimeout = 10 * time.Second

// assignedNode reads the Pod's node from the shared cache, so Pod events cost no API requests.
func (r *podRepo) assignedNode(pod *corev1.Pod) *corev1.Node {
	if pod.Spec.NodeName == "" {
		return nil
	}
	node, err := r.nodeLister.Get(pod.Spec.NodeName)
	if apierrors.IsNotFound(err) {
		// The node watch can lag the Pod watch on the initial decode.
		// Read directly as a detached object; never write fallback Nodes back into the informer cache.
		ctx, cancel := context.WithTimeout(context.Background(), nodeReadTimeout)
		node, err = r.data.k8sCl.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
		cancel()
	}
	if err != nil {
		r.log.Warnf("cannot read node %s for pod %s/%s: %v", pod.Spec.NodeName, pod.Namespace, pod.Name, err)
		return nil
	}
	return node
}

func (r *podRepo) GetCreateTime(pod *corev1.Pod) time.Time {
	for _, status := range pod.Status.Conditions {
		if status.Type == corev1.PodScheduled {
			return status.LastTransitionTime.Time
		}
	}
	return time.Now()
}

func (r *podRepo) GetStartTime(pod *corev1.Pod) time.Time {
	for _, status := range pod.Status.Conditions {
		if status.Type == corev1.PodInitialized {
			return status.LastTransitionTime.Time
		}
	}
	return time.Now()
}

func (r *podRepo) ListAll(context.Context) ([]*biz.Container, error) {
	snapshot := r.ascend.Snapshot()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	var containerList []*biz.Container
	for _, pod := range r.pods {
		for _, container := range pod.Ctrs {
			containerList = append(containerList, r.interpretContainer(snapshot, container))
		}
	}
	return containerList, nil
}

func (r *podRepo) NodeHolders(_ context.Context, name, uid string) (biz.NodeHolders, error) {
	node, err := r.nodeLister.Get(name)
	if err != nil {
		return biz.NodeHolders{}, err
	}
	if string(node.UID) != uid {
		return biz.NodeHolders{}, fmt.Errorf("node %s no longer has the requested UID", name)
	}
	pods, err := r.podIndexer.ByIndex(factsNodeIndex, name)
	if err != nil {
		return biz.NodeHolders{}, err
	}
	result := biz.NodeHolders{Reservations: make(map[string]biz.DeviceReservation), UncertainDeviceIDs: make(map[string]bool)}
	snapshot := r.ascend.Snapshot()
	revision := factInputRevision(snapshot, node)
	for _, obj := range pods {
		current := obj.(*corev1.Pod)
		if current.Annotations[util.AssignedNodeAnnotations] != name || biz.IsPodInTerminatedState(current) {
			continue
		}
		// Pod events replace the immutable cache entry.
		r.mutex.RLock()
		pod := r.pods[current.UID]
		r.mutex.RUnlock()
		if pod == nil || pod.ResourceVersion != current.ResourceVersion {
			return biz.NodeHolders{}, fmt.Errorf("waiting for allocation cache for %s/%s", current.Namespace, current.Name)
		}
		// The Node may have changed without a new Pod event. Refresh the decoded
		// card modes from the same Node snapshot used to validate the facts.
		if pod.AllocationRevision != revision {
			decoded := r.decodePodInfo(current, node, snapshot)
			r.mutex.Lock()
			if r.pods[current.UID] == pod {
				r.pods[current.UID] = decoded
			}
			r.mutex.Unlock()
			pod = decoded
		}
		hasInit := len(current.Spec.InitContainers) > 0
		allocation, ready := r.facts.NodeAllocation(current.UID, current.ResourceVersion, revision, hasInit)
		if !ready {
			// A dependent replay may have raced this Pod update. Reuse the
			// guarded informer path before publishing any allocation totals.
			r.onFactsPod(current)
			allocation, ready = r.facts.NodeAllocation(current.UID, current.ResourceVersion, revision, hasInit)
			if !ready {
				return biz.NodeHolders{}, fmt.Errorf("waiting for allocation facts for %s/%s", current.Namespace, current.Name)
			}
		}
		result.UnreadableAllocations = append(result.UnreadableAllocations, allocation.Unreadable...)
		initDevices := activeInitDevices(current.Spec.InitContainers, pod.Devices, allocation.InitReleased)
		countedPeaks := addInitReservations(&result, initDevices, allocation, name, uid)
		for _, container := range pod.Ctrs {
			if string(allocation.UID) != uid || (container.NodeName != "" && container.NodeName != name) {
				for _, device := range container.ContainerDevices {
					result.UncertainDeviceIDs[device.UUID] = true
				}
				continue
			}
			holder := *r.interpretContainer(snapshot, container)
			holder.NodeUID = uid
			result.Containers = append(result.Containers, &holder)
			for _, d := range holder.ContainerDevices {
				// The Pod peak already includes concurrent app containers on this card.
				if countedPeaks[d.UUID] {
					continue
				}
				total := result.Reservations[d.UUID]
				total.Slots++
				total.MemoryMiB += d.Usedmem
				total.Cores += d.Usedcores
				total.CoreUnknown = total.CoreUnknown || d.CoreAllocationUnknown
				result.Reservations[d.UUID] = total
			}
		}
	}
	return result, nil
}

// The legacy workload list contains app containers only. Keep its links while
// using scheduler phase peaks for cards shared with an active init container.
func activeInitDevices(init []corev1.Container, devices biz.PodDevices, released bool) map[string]bool {
	if len(init) == 0 {
		return nil
	}
	ids := make(map[string]bool)
	for _, slots := range devices {
		for i, slot := range slots[:min(len(init), len(slots))] {
			if released && (init[i].RestartPolicy == nil || *init[i].RestartPolicy != corev1.ContainerRestartPolicyAlways) {
				continue
			}
			for _, d := range slot {
				ids[d.UUID] = true
			}
		}
	}
	return ids
}

func addInitReservations(h *biz.NodeHolders, ids map[string]bool, allocation podfacts.NodeAllocation, name, uid string) map[string]bool {
	if len(ids) == 0 {
		return nil
	}
	// One card cannot take its peak from two different allocation authorities
	// or resource names in the same Pod.
	peaks := make(map[string]*podfacts.Occupancy, len(ids))
	for i := range allocation.Peaks {
		peak := &allocation.Peaks[i]
		id := peak.DeviceID
		if !ids[id] || string(allocation.UID) != uid || peak.Node != name || string(peak.NodeUID) != uid {
			continue
		}
		if _, duplicate := peaks[id]; duplicate {
			peaks[id] = nil
		} else {
			peaks[id] = peak
		}
	}
	counted := make(map[string]bool, len(ids))
	for id := range ids {
		peak := peaks[id]
		if peak == nil {
			h.UncertainDeviceIDs[id] = true
			continue
		}
		slots, memory, cores := peak.Quantities["slots"], peak.Quantities["memory"], peak.Quantities["compute"]
		if slots.Value == nil || memory.Value == nil || slots.Unit != "slots" || memory.Unit != "bytes" {
			h.UncertainDeviceIDs[id] = true
			continue
		}
		total := h.Reservations[id]
		total.HasInit = true
		total.Slots += int32(slots.Value.Value())
		total.MemoryMiB += int32(memory.Value.Value() / (1 << 20))
		if cores.Value == nil || cores.Unit != "percent" {
			total.CoreUnknown = true
		} else {
			// Device details report whole percentages, including for fractional Ascend templates.
			total.Cores += int32(math.Round(cores.Value.AsApproximateFloat64()))
		}
		h.Reservations[id] = total
		counted[id] = true
	}
	return counted
}

func (r *podRepo) FindOne(_ context.Context, podUID string, name string) (*biz.Container, error) {
	if podUID == "" || name == "" {
		return nil, fmt.Errorf("podUID or name is empty")
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()
	pod, ok := r.pods[k8stypes.UID(podUID)]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	for _, container := range pod.Ctrs {
		if container.Name == name {
			return r.interpretContainer(r.ascend.Snapshot(), container), nil
		}
	}

	return nil, fmt.Errorf("not found")
}

func (r *podRepo) isCachedVersion(pod *corev1.Pod) bool {
	current, err := r.podLister.Pods(pod.Namespace).Get(pod.Name)
	return err == nil && current != nil && current.UID == pod.UID && current.ResourceVersion == pod.ResourceVersion
}
