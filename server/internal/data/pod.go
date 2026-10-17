package data

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/mthreads"
	"vgpu/internal/provider/nvidia"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	k8stypes "k8s.io/apimachinery/pkg/types"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// wholeGPUPod captures one pod that holds vendor whole-card GPUs
// (mthreads.com/gpu). Such pods are delivered outside HAMi scheduling and
// carry no allocation annotations, so their occupancy is synthesized here:
// card ids are allocated per node when the pod is first tracked and retained
// across refreshes.
type wholeGPUPod struct {
	pod      *corev1.Pod
	nodeID   string
	nodeUID  string
	nodeName string
	memMiB   int32
	ctrs     []wholeGPUContainer // container idx -> requested whole-GPU count
}

type wholeGPUContainer struct {
	name  string
	count int64
	cards []int64 // stable whole-card ids: physical id, or negative when overcommitted
}

type podRepo struct {
	data                    *Data
	podLister               listerscorev1.PodLister
	nodeLister              listerscorev1.NodeLister
	pods                    map[k8stypes.UID]*biz.PodInfo
	wholeGPUPods            map[k8stypes.UID]*wholeGPUPod
	mutex                   sync.RWMutex
	log                     *log.Helper
	podIndexer              cache.Indexer
	schedulingResourceNames map[corev1.ResourceName]struct{}
	schedulingEvents        *schedulingEventReader
	ascend                  ascend.Decoder
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
		data:                    data,
		pods:                    make(map[k8stypes.UID]*biz.PodInfo),
		wholeGPUPods:            make(map[k8stypes.UID]*wholeGPUPod),
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

func (r *podRepo) onAddPod(obj interface{}) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.log.Error("unknown add object type")
		return
	}
	if r.refreshWholeGPUPod(pod) {
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
	nodeUID := ""
	if node != nil {
		nodeUID = string(node.UID)
	}
	podDev, err := util.DecodePodDevices(pod, r.log)
	if err != nil {
		r.log.Errorf("cannot decode device allocations for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}
	gpus := nvidia.ReadAllocations(pod, nvidia.RegisteredModes(node, r.log))
	if err := gpus.Err(); err != nil {
		r.log.Warnf("ignore MIG reservations of pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	bizPodDev := bizPodDevices(gpus, podDev)
	ascendDevices, err := r.ascend.Decode(pod, node)
	if err != nil {
		r.log.Errorf("cannot decode Ascend allocations for pod %s/%s: %v", pod.Namespace, pod.Name, err)
	}
	for word, slots := range ascendDevices {
		bizPodDev[word] = ascendPodSingleDevice(slots)
	}
	r.addPod(pod, nodeID, nodeUID, bizPodDev)
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

func (r *podRepo) onUpdatePod(_ interface{}, new interface{}) {
	r.onAddPod(new)
}

func (r *podRepo) onDeletedPod(obj interface{}) {
	// A watch that missed a delete delivers a tombstone wrapping the last
	// known state; both ledgers must still be cleaned up.
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.log.Error("unknown delete object type")
		return
	}
	// Drop the whole-GPU ledger entry before the annotation check: these
	// pods carry no assigned-node annotation by design.
	r.removeWholeGPUPod(pod.UID)
	_, ok = pod.Annotations[util.AssignedNodeAnnotations]
	if !ok {
		return
	}
	r.delPod(pod)
}

func (r *podRepo) addPod(pod *corev1.Pod, nodeID, nodeUID string, devices biz.PodDevices) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	ctrs := r.fetchContainerInfo(pod, devices, nodeUID)
	pi := &biz.PodInfo{Name: pod.Name, UID: pod.UID, Namespace: pod.Namespace, NodeID: nodeID, Devices: devices, Ctrs: ctrs}
	r.pods[pod.UID] = pi
	r.log.Infof("Pod added: Name: %s, UID: %s, Namespace: %s, NodeID: %s", pod.Name, pod.UID, pod.Namespace, nodeID)
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
		// The node watch can lag the Pod watch, and nothing re-reads this Pod once the node arrives.
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
	containerList = append(containerList, r.listWholeGPUContainers()...)
	return containerList, nil
}

// refreshWholeGPUPod upserts the ledger entry for a pod that requests vendor
// whole-card GPUs. It returns true when the pod is whole-GPU-delivered (HAMi
// annotation absent, mthreads.com/gpu present), false when it should flow
// through the regular HAMi annotation path.
func (r *podRepo) refreshWholeGPUPod(pod *corev1.Pod) bool {
	if _, ham := pod.Annotations[util.AssignedNodeAnnotations]; ham {
		// The pod moved to HAMi-managed delivery; drop any stale ledger
		// entry from an earlier spec revision.
		r.removeWholeGPUPod(pod.UID)
		return false
	}
	counts := make([]wholeGPUContainer, 0, len(pod.Spec.Containers))
	for _, ctr := range pod.Spec.Containers {
		q, ok := ctr.Resources.Limits[corev1.ResourceName(mthreads.NodeWholeGPUResource)]
		if !ok {
			q, ok = ctr.Resources.Requests[corev1.ResourceName(mthreads.NodeWholeGPUResource)]
		}
		if !ok {
			continue
		}
		if n, ok := q.AsInt64(); ok && n > 0 {
			counts = append(counts, wholeGPUContainer{name: ctr.Name, count: n})
		}
	}
	if len(counts) == 0 {
		// The pod no longer requests vendor whole cards; purge the ledger.
		r.removeWholeGPUPod(pod.UID)
		return false
	}
	if biz.IsPodInTerminatedState(pod) || pod.Spec.NodeName == "" {
		r.removeWholeGPUPod(pod.UID)
		return true
	}

	nodeObj, ok := r.wholeGPUNodeContext(pod.Spec.NodeName)
	if !ok {
		// Node unresolved (informer cache miss and live read failed). Drop any
		// stale entry and skip storing until a later event resolves it, so
		// readers never receive a nil node context.
		r.removeWholeGPUPod(pod.UID)
		return true
	}
	_, wholeIDs, _ := mthreads.NodeCardInventory(nodeObj)

	r.mutex.Lock()
	defer r.mutex.Unlock()
	sort.Slice(counts, func(i, j int) bool { return counts[i].name < counts[j].name })
	r.assignWholeCardIDs(pod.UID, pod.Spec.NodeName, wholeIDs, counts)
	r.wholeGPUPods[pod.UID] = &wholeGPUPod{
		pod: pod, nodeID: nodeObj.Name, nodeUID: string(nodeObj.UID),
		nodeName: pod.Spec.NodeName, memMiB: mthreads.PerCardMemoryMiB(nodeObj), ctrs: counts,
	}
	r.log.Infof("Whole-GPU pod tracked: %s/%s on %s, %d container(s)", pod.Namespace, pod.Name, pod.Spec.NodeName, len(counts))
	return true
}

// assignWholeCardIDs picks a stable whole-card id set for each container of a
// pod, reusing ids the pod already holds and otherwise taking the lowest ids
// free on the node. Retention matters because consumers resolve the
// synthesized device UUID back to a physical card; recomputing from map order
// would let unrelated pods move between cards. Ids beyond the physical pool
// (accounting-layer overcommit) are synthesized as negative values.
//
// Callers must hold r.mutex.
func (r *podRepo) assignWholeCardIDs(uid k8stypes.UID, nodeName string, wholeIDs []int64, counts []wholeGPUContainer) {
	used := map[int64]bool{}
	for otherUID, e := range r.wholeGPUPods {
		if otherUID == uid || e.nodeName != nodeName {
			continue
		}
		for _, c := range e.ctrs {
			for _, id := range c.cards {
				used[id] = true
			}
		}
	}
	var prev map[string][]int64
	if e, ok := r.wholeGPUPods[uid]; ok && e.nodeName == nodeName {
		prev = make(map[string][]int64, len(e.ctrs))
		for _, c := range e.ctrs {
			prev[c.name] = c.cards
		}
	}
	for i := range counts {
		c := &counts[i]
		cards := make([]int64, 0, c.count)
		for _, id := range prev[c.name] {
			if int64(len(cards)) >= c.count {
				break
			}
			if !used[id] {
				cards = append(cards, id)
				used[id] = true
			}
		}
		for _, id := range wholeIDs {
			if int64(len(cards)) >= c.count {
				break
			}
			if !used[id] {
				cards = append(cards, id)
				used[id] = true
			}
		}
		for synth := int64(-1); int64(len(cards)) < c.count; synth-- {
			if !used[synth] {
				cards = append(cards, synth)
				used[synth] = true
			}
		}
		sort.Slice(cards, func(a, b int) bool { return cards[a] < cards[b] })
		c.cards = cards
	}
}

func (r *podRepo) removeWholeGPUPod(uid k8stypes.UID) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// Dropping the entry releases its card ids: allocation is derived from the
	// live ledger, so the next pod to refresh can reclaim them.
	delete(r.wholeGPUPods, uid)
}

// wholeGPUNodeContext resolves the Node hosting a whole-GPU pod, preferring
// the shared informer cache (this runs inside informer callbacks, where a slow
// live API call would delay subsequent pod events) and falling back to a
// bounded live read. It reports false when neither source resolves, so callers
// never store a nil node.
func (r *podRepo) wholeGPUNodeContext(nodeName string) (*corev1.Node, bool) {
	if r.nodeLister != nil {
		if node, err := r.nodeLister.Get(nodeName); err == nil {
			return node, true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	node, err := r.data.k8sCl.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		r.log.Warnf("cannot read node %s for whole-GPU context: %v", nodeName, err)
		return nil, false
	}
	return node, true
}

// listWholeGPUContainers synthesizes container/device records for whole-GPU
// pods from the stable card ids retained when each ledger entry was created.
// The vendor kubelet allocation does not expose which physical slot a pod
// received, so the ids are an ordered approximation while the per-node
// occupancy counts are exact.
func (r *podRepo) listWholeGPUContainers() []*biz.Container {
	if len(r.wholeGPUPods) == 0 {
		return nil
	}
	entries := make([]*wholeGPUPod, 0, len(r.wholeGPUPods))
	for _, e := range r.wholeGPUPods {
		entries = append(entries, e)
	}
	// Deterministic output order; unrelated to id assignment, which is now
	// retained per pod.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].nodeName != entries[j].nodeName {
			return entries[i].nodeName < entries[j].nodeName
		}
		if entries[i].pod.Namespace != entries[j].pod.Namespace {
			return entries[i].pod.Namespace < entries[j].pod.Namespace
		}
		if entries[i].pod.Name != entries[j].pod.Name {
			return entries[i].pod.Name < entries[j].pod.Name
		}
		return entries[i].pod.UID < entries[j].pod.UID
	})

	out := make([]*biz.Container, 0, len(entries))
	for _, e := range entries {
		for _, wc := range e.ctrs {
			var observed *corev1.ContainerStatus
			for i := range e.pod.Status.ContainerStatuses {
				if e.pod.Status.ContainerStatuses[i].Name == wc.name {
					observed = &e.pod.Status.ContainerStatuses[i]
					break
				}
			}
			status, statusDetail := classifyContainerStatus(e.pod, observed)
			cds := biz.ContainerDevices{}
			for i, cardID := range wc.cards {
				uuid := fmt.Sprintf("%s-mthreads-full-%d", e.nodeName, cardID)
				if cardID < 0 {
					// Accounting-layer overcommit: no physical card backs
					// this synthesized id.
					uuid = fmt.Sprintf("%s-mthreads-full-ovc-%d", e.nodeName, -cardID-1)
				}
				cds = append(cds, biz.ContainerDevice{
					Idx:       i,
					UUID:      uuid,
					Type:      mthreads.MthreadsWholeGPUType,
					Usedmem:   e.memMiB,
					Usedcores: biz.PhysicalCoreBaselinePerDevice,
				})
			}
			out = append(out, &biz.Container{
				Name:             wc.name,
				NodeName:         e.nodeName,
				PodName:          e.pod.Name,
				PodUID:           string(e.pod.UID),
				NodeUID:          e.nodeUID,
				Namespace:        e.pod.Namespace,
				Image:            r.containerImage(e.pod, wc.name),
				Status:           status,
				StatusDetail:     statusDetail,
				CreateTime:       r.GetCreateTime(e.pod),
				ContainerDevices: cds,
			})
		}
	}
	return out
}

func (r *podRepo) containerImage(pod *corev1.Pod, name string) string {
	for _, ctr := range pod.Spec.Containers {
		if ctr.Name == name {
			return ctr.Image
		}
	}
	for _, ctr := range pod.Spec.InitContainers {
		if ctr.Name == name {
			return ctr.Image
		}
	}
	return ""
}

func (r *podRepo) FindOne(_ context.Context, podUID string, name string) (*biz.Container, error) {
	if podUID == "" || name == "" {
		return nil, fmt.Errorf("podUID or name is empty")
	}

	r.mutex.RLock()
	defer r.mutex.RUnlock()
	pod, ok := r.pods[k8stypes.UID(podUID)]
	if !ok {
		// Whole-card pods are synthesized at read time and are not stored
		// in r.pods; fall back to the whole-GPU ledger.
		for _, container := range r.listWholeGPUContainers() {
			if container.PodUID == podUID && container.Name == name {
				return container, nil
			}
		}
		return nil, fmt.Errorf("not found")
	}
	for _, container := range pod.Ctrs {
		if container.Name == name {
			return r.interpretContainer(r.ascend.Snapshot(), container), nil
		}
	}

	return nil, fmt.Errorf("not found")
}
