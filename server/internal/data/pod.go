package data

import (
	"context"
	"fmt"
	"sync"
	"time"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/jinzhu/copier"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type podRepo struct {
	data                    *Data
	podLister               listerscorev1.PodLister
	pods                    map[k8stypes.UID]*biz.PodInfo
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
	newContainer := func(ctr corev1.Container, kind string, slot int, observed *corev1.ContainerStatus, status string, statusDetail *biz.ContainerStatusDetail) *biz.Container {
		c := &biz.Container{
			Name:             ctr.Name,
			Kind:             kind,
			ContainerIdx:     slot,
			NodeName:         pod.Spec.NodeName,
			PodName:          pod.Name,
			PodUID:           string(pod.UID),
			Image:            ctr.Image,
			Status:           status,
			StatusDetail:     statusDetail,
			NodeUID:          nodeUID,
			Namespace:        pod.Namespace,
			CreateTime:       r.GetCreateTime(pod),
			ContainerDevices: bizContainerDevices[slot],
		}
		if observed != nil {
			c.UUID = observed.ContainerID
		}
		if len(c.ContainerDevices) > 0 {
			c.Priority = c.ContainerDevices[0].Priority
		}
		return c
	}

	// HAMi releases init allocations once every regular init container has succeeded.
	initReleased := allNonSidecarInitContainersSucceeded(pod)
	for i, ctr := range pod.Spec.InitContainers {
		kind := initContainerKind(ctr)
		if len(bizContainerDevices[i]) == 0 || (kind == biz.ContainerKindInit && initReleased) {
			continue
		}
		status, statusDetail := schedulingContainerStatus(pod, ctr, kind)
		observed := findContainerStatus(pod.Status.InitContainerStatuses, ctr.Name)
		containers = append(containers, newContainer(ctr, kind, i, observed, status, statusDetail))
	}
	for i, ctr := range pod.Spec.Containers {
		observed := findContainerStatus(pod.Status.ContainerStatuses, ctr.Name)
		status, statusDetail := classifyContainerStatus(pod, observed)
		containers = append(containers, newContainer(ctr, biz.ContainerKindRegular, len(pod.Spec.InitContainers)+i, observed, status, statusDetail))
	}
	return containers
}

func findContainerStatus(statuses []corev1.ContainerStatus, name string) *corev1.ContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	return nil
}

func initContainerKind(ctr corev1.Container) string {
	if ctr.RestartPolicy != nil && *ctr.RestartPolicy == corev1.ContainerRestartPolicyAlways {
		return biz.ContainerKindSidecar
	}
	return biz.ContainerKindInit
}

// Mirrors HAMi's util.AllNonSidecarInitContainersSucceeded.
func allNonSidecarInitContainersSucceeded(pod *corev1.Pod) bool {
	if len(pod.Spec.InitContainers) == 0 {
		return false
	}
	for _, ctr := range pod.Spec.InitContainers {
		if initContainerKind(ctr) == biz.ContainerKindSidecar {
			continue
		}
		observed := findContainerStatus(pod.Status.InitContainerStatuses, ctr.Name)
		if observed == nil || observed.State.Terminated == nil || observed.State.Terminated.ExitCode != 0 {
			return false
		}
	}
	return true
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
	node, err := r.data.k8sCl.CoreV1().Nodes().Get(context.Background(), pod.Spec.NodeName, metav1.GetOptions{})
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
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	var containerList []*biz.Container
	for _, pod := range r.pods {
		containerList = append(containerList, pod.Ctrs...)
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
			return container, nil
		}
	}

	return nil, fmt.Errorf("not found")
}
