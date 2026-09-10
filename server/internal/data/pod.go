package data

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/provider/mthreads"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/jinzhu/copier"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

// wholeGPUPod captures one pod that holds vendor whole-card GPUs
// (mthreads.com/gpu). Such pods are delivered outside HAMi scheduling and
// carry no allocation annotations, so their occupancy is synthesized here:
// slots are assigned per node in a compact, deterministic order at read time.
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
}

type podRepo struct {
	data                    *Data
	podLister               listerscorev1.PodLister
	pods                    map[k8stypes.UID]*biz.PodInfo
	wholeGPUPods            map[k8stypes.UID]*wholeGPUPod
	mutex                   sync.RWMutex
	log                     *log.Helper
	podIndexer              cache.Indexer
	schedulingResourceNames map[corev1.ResourceName]struct{}
	schedulingEvents        *schedulingEventReader
}

func NewPodRepo(data *Data, logger log.Logger, config *conf.Bootstrap) (*podRepo, error) {
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
	}
	repo.init()
	return repo, nil
}

func (r *podRepo) init() {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(r.data.k8sCl, time.Hour*1)
	r.podLister = informerFactory.Core().V1().Pods().Lister()
	informer := informerFactory.Core().V1().Pods().Informer()
	// An index on the existing informer, not another watch.
	if err := informer.AddIndexers(cache.Indexers{schedulingGPUIndex: r.schedulingIndex}); err != nil {
		panic(err)
	}
	r.podIndexer = informer.GetIndexer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.onAddPod,
		UpdateFunc: r.onUpdatePod,
		DeleteFunc: r.onDeletedPod,
	})
	stopCh := make(chan struct{})
	informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)
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
	nodeUID, ascendMode := r.nodeAllocationContext(pod)
	bizPodDev := biz.PodDevices{}
	podDev, err := util.DecodePodDevices(pod, r.log, ascendMode)
	if err != nil {
		r.log.Errorf("cannot decode device allocations for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}
	copier.Copy(&bizPodDev, podDev)
	r.addPod(pod, nodeID, nodeUID, bizPodDev)
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
		if len(containerDevices) > 0 {
			c.Priority = containerDevices[0].Priority
		}
		containers = append(containers, c)
	}
	return containers
}

func mergeContainerDevicesBySlot(totalContainers int, podDevices biz.PodDevices) []biz.ContainerDevices {
	containerDevices := make([]biz.ContainerDevices, totalContainers)
	for _, devicesByContainer := range podDevices {
		for i, devices := range devicesByContainer {
			if i >= totalContainers {
				break
			}
			containerDevices[i] = append(containerDevices[i], devices...)
		}
	}
	return containerDevices
}

func (r *podRepo) nodeAllocationContext(pod *corev1.Pod) (string, util.AscendAllocationMode) {
	podMode := pod.Annotations[util.AscendVNPUModeAnnotation]
	if pod.Spec.NodeName == "" {
		return "", resolveAscendAllocationMode(podMode, "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	node, err := r.data.k8sCl.CoreV1().Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		r.log.Warnf("cannot resolve Ascend allocation mode for pod %s/%s: %v", pod.Namespace, pod.Name, err)
		return "", resolveAscendAllocationMode(podMode, "")
	}
	return string(node.UID), resolveAscendAllocationMode(podMode, node.Annotations[util.AscendNodeHamiCoreAnnotation])
}

func resolveAscendAllocationMode(podMode, nodeHamiCore string) util.AscendAllocationMode {
	if podMode != "" {
		if podMode == util.AscendVNPUModeHamiCore {
			return util.AscendAllocationModeHamiCore
		}
		return util.AscendAllocationModeTemplate
	}
	switch nodeHamiCore {
	case "true":
		// Released plugin images disagree on whether an unannotated Pod inherits
		// soft mode from the Node. The Node flag does not encode that runtime
		// version, so this allocation cannot be decoded without guessing.
		return util.AscendAllocationModeUnknown
	case "false":
		return util.AscendAllocationModeTemplate
	default:
		return util.AscendAllocationModeUnknown
	}
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
	r.mutex.Lock()
	defer r.mutex.Unlock()
	var containerList []*biz.Container
	for _, pod := range r.pods {
		containerList = append(containerList, pod.Ctrs...)
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

	nodeID, nodeUID, memMiB := r.wholeGPUNodeContext(pod.Spec.NodeName)
	r.mutex.Lock()
	defer r.mutex.Unlock()
	sort.Slice(counts, func(i, j int) bool { return counts[i].name < counts[j].name })
	r.wholeGPUPods[pod.UID] = &wholeGPUPod{
		pod: pod, nodeID: nodeID, nodeUID: nodeUID,
		nodeName: pod.Spec.NodeName, memMiB: memMiB, ctrs: counts,
	}
	r.log.Infof("Whole-GPU pod tracked: %s/%s on %s, %d container(s)", pod.Namespace, pod.Name, pod.Spec.NodeName, len(counts))
	return true
}

func (r *podRepo) removeWholeGPUPod(uid k8stypes.UID) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.wholeGPUPods, uid)
}

// wholeGPUNodeContext resolves node identifiers and the per-card memory of
// the node, derived from the sGPU pool when present (same card model).
func (r *podRepo) wholeGPUNodeContext(nodeName string) (string, string, int32) {
	// Bound the request: this runs inside informer callbacks, and an
	// unbounded API call would delay subsequent pod events.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	node, err := r.data.k8sCl.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		r.log.Warnf("cannot read node %s for whole-GPU context: %v", nodeName, err)
		return nodeName, "", 0
	}
	cores, _ := node.Status.Capacity.Name(corev1.ResourceName(mthreads.NodeSGPUCoresResource), resource.DecimalSI).AsInt64()
	memUnits, _ := node.Status.Capacity.Name(corev1.ResourceName(mthreads.NodeSGPUMemoryResource), resource.DecimalSI).AsInt64()
	var memMiB int32
	if cards := cores / mthreads.CoresPerCard; cards > 0 {
		memMiB = int32(memUnits * mthreads.MemoryFactorMiB / cards)
	}
	return nodeName, string(node.UID), memMiB
}

// listWholeGPUContainers synthesizes container/device records for whole-GPU
// pods. Slots are assigned per node in pod-name order so the mapping stays
// compact and stable across WebUI restarts; the vendor kubelet allocation
// does not expose which physical slot a pod received, so the specific index
// is an approximation while the per-node occupancy counts are exact.
func (r *podRepo) listWholeGPUContainers() []*biz.Container {
	if len(r.wholeGPUPods) == 0 {
		return nil
	}
	entries := make([]*wholeGPUPod, 0, len(r.wholeGPUPods))
	for _, e := range r.wholeGPUPods {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].pod.Name < entries[j].pod.Name })

	slots := map[string]int64{} // node -> next free whole-GPU slot
	out := make([]*biz.Container, 0, len(entries))
	for _, e := range entries {
		for _, wc := range e.ctrs {
			cds := biz.ContainerDevices{}
			for k := int64(0); k < wc.count; k++ {
				slot := slots[e.nodeName]
				slots[e.nodeName] = slot + 1
				cds = append(cds, biz.ContainerDevice{
					Idx:       int(slot),
					UUID:      fmt.Sprintf("%s-mthreads-full-%d", e.nodeName, slot),
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
				Status:           r.wholeGPUStatus(e.pod),
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

func (r *podRepo) wholeGPUStatus(pod *corev1.Pod) string {
	if pod.Status.Phase == corev1.PodRunning {
		return biz.ContainerStatusSuccess
	}
	if pod.Status.Phase == corev1.PodFailed {
		return biz.ContainerStatusFailed
	}
	return biz.ContainerStatusUnknown
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
			return container, nil
		}
	}

	return nil, fmt.Errorf("not found")
}
