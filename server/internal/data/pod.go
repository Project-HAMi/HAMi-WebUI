package data

import (
	"context"
	"fmt"
	"sync"
	"time"
	"vgpu/internal/biz"
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
	data      *Data
	podLister listerscorev1.PodLister
	pods      map[k8stypes.UID]*biz.PodInfo
	mutex     sync.RWMutex
	log       *log.Helper
}

func NewPodRepo(data *Data, logger log.Logger) biz.PodRepo {
	repo := &podRepo{
		data: data,
		pods: make(map[k8stypes.UID]*biz.PodInfo),
		log:  log.NewHelper(logger),
	}
	repo.init()
	return repo
}

func (r *podRepo) init() {
	informerFactory := informers.NewSharedInformerFactoryWithOptions(r.data.k8sCl, time.Hour*1)
	r.podLister = informerFactory.Core().V1().Pods().Lister()
	informer := informerFactory.Core().V1().Pods().Informer()
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
	bizPodDev := biz.PodDevices{}
	podDev, _ := util.DecodePodDevices(pod, r.log)
	copier.Copy(&bizPodDev, podDev)
	r.addPod(pod, nodeID, bizPodDev)
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

func (r *podRepo) addPod(pod *corev1.Pod, nodeID string, devices biz.PodDevices) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	ctrs := r.fetchContainerInfo(pod)
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

func (r *podRepo) fetchContainerInfo(pod *corev1.Pod) []*biz.Container {
	containers := []*biz.Container{}
	pdevices, err := util.DecodePodDevices(pod, r.log)
	if err != nil {
		return containers
	}
	bizContainerDevices := []biz.ContainerDevices{}
	for _, pds := range pdevices {
		copier.Copy(&bizContainerDevices, pds)
	}
	if len(bizContainerDevices) < 1 {
		return containers
	}

	containerStat := map[string]string{}
	for _, ctr := range pod.Status.ContainerStatuses {
		containerStat[ctr.Name] = biz.ContainerStatusUnknown
		if pod.Status.Phase == corev1.PodRunning && ctr.Ready {
			containerStat[ctr.Name] = biz.ContainerStatusSuccess
		} else if pod.Status.Phase == corev1.PodFailed {
			containerStat[ctr.Name] = biz.ContainerStatusFailed
		}
	}
	ctrIdMaps := map[string]string{}
	for _, ctr := range pod.Status.ContainerStatuses {
		ctrIdMaps[ctr.Name] = ctr.ContainerID
	}
	for i, ctr := range pod.Spec.Containers {
		c := &biz.Container{
			Name:             ctr.Name,
			UUID:             ctrIdMaps[ctr.Name],
			ContainerIdx:     i,
			NodeName:         pod.Spec.NodeName,
			PodName:          pod.Name,
			PodUID:           string(pod.UID),
			Status:           containerStat[ctr.Name],
			NodeUID:          r.GetNodeUUID(pod),
			Priority:         bizContainerDevices[i][0].Priority,
			Namespace:        pod.Namespace,
			CreateTime:       r.GetCreateTime(pod),
			ContainerDevices: bizContainerDevices[i],
		}
		containers = append(containers, c)
	}
	return containers
}

func (r *podRepo) GetNodeUUID(pod *corev1.Pod) string {
	node, err := r.data.k8sCl.CoreV1().Nodes().Get(context.Background(), pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		//p.log.Warnf("Error getting node: %s", err)
		return ""
	} else {
		return string(node.UID)
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

	for _, container := range r.pods[k8stypes.UID(podUID)].Ctrs {
		if container.Name == name {
			return container, nil
		}
	}

	return nil, fmt.Errorf("not found")
}
