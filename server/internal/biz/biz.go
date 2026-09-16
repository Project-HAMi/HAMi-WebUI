package biz

import (
	"github.com/google/wire"
	corev1 "k8s.io/api/core/v1"
)

var ProviderSet = wire.NewSet(
	NewNodeUsecase,
	NewPodUseCase,
	NewSummaryUseCase,
	NewSchedulingUsecase,
)

const (
	NvidiaGPUDevice = "NVIDIA"
	HygonGPUDevice  = "DCU"
	HygonHCUDevice  = "HCU"
	AscendGPUDevice = "Ascend"
	MetaxGPUDevice  = "Metax"

	CambriconGPUDevice = "MLU"

	ContainerStatusSuccess     = "success"
	ContainerStatusFailed      = "failed"
	ContainerStatusClosed      = "closed"
	ContainerStatusUnknown     = "unknown"
	ContainerStatusWaiting     = "waiting"
	ContainerStatusNotReady    = "not_ready"
	ContainerStatusError       = "error"
	ContainerStatusTerminating = "terminating"

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

	// Set by providers that interpret allocations against HAMi's device configuration.
	Vendor                string
	Shape                 string
	Template              string
	CoreAllocationUnknown bool
	CoreReason            string
	Ascend                *AscendFacts
}

type AscendFacts struct {
	AnnotatedCore int32
	Template      string
	Recorded      bool
	CardMemory    int64
	NodeRead      bool
	Mode          string
	ModeReason    string
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
