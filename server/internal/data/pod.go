package data

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/ascend"
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

type podRepo struct {
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
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		r.log.Error("unknown add object type")
		return
	}
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
	return containerList, nil
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
