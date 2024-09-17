package biz

import (
	"github.com/google/wire"
	corev1 "k8s.io/api/core/v1"
)

var ProviderSet = wire.NewSet(
	NewNodeUsecase,
	NewPodUseCase,
	NewSummaryUseCase,
)

const (
	NvidiaGPUDevice = "NVIDIA"
	HygonGPUDevice  = "DCU"
	AscendGPUDevice = "Ascend"

	CambriconGPUDevice = "MLU"

	ContainerStatusSuccess = "success"
	ContainerStatusFailed  = "failed"
	ContainerStatusClosed  = "closed"
	ContainerStatusUnknown = "unknown"

	ComponentTypeDeployment  = "deployment"
	ComponentTypeStatefulSet = "statefulset"
	ComponentTypeDaemonSet   = "daemonset"
)

type ContainerDevice struct {
	Idx       int
	UUID      string
	Type      string
	Usedmem   int32
	Usedcores int32
	Priority  string
}

type ContainerDeviceRequest struct {
	Nums             int32
	Type             string
	Memreq           int32
	MemPercentagereq int32
	Coresreq         int32
}

type ContainerDevices []ContainerDevice
type ContainerDeviceRequests map[string]ContainerDeviceRequest

type PodSingleDevice []ContainerDevices

type PodDeviceRequests []ContainerDeviceRequests
type PodDevices map[string]PodSingleDevice

func IsPodInTerminatedState(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded
}
