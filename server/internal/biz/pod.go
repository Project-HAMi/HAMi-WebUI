package biz

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"strings"
	"time"
)

type Container struct {
	Name             string
	UUID             string
	ContainerIdx     int
	NodeName         string
	PodUID           string
	PodName          string
	Image            string
	ContainerDevices ContainerDevices
	Status           string
	StatusDetail     *ContainerStatusDetail
	CreateTime       time.Time
	Priority         string
	NodeUID          string
	Namespace        string
}

type ContainerStatusDetail struct {
	ContainerState         string
	Reason                 string
	Message                string
	Ready                  *bool
	RestartCount           int32
	ExitCode               *int32
	PodPhase               string
	PodReady               string
	PodReadyReason         string
	PodReadyMessage        string
	RestartPending         bool
	LastTerminationReason  string
	LastExitCode           *int32
	LastTerminationMessage string
	PodReason              string
	PodMessage             string
}

type PodInfo struct {
	AllocationRevision string // Node and catalog used to decode Devices and Ctrs.
	ResourceVersion    string
	Namespace          string
	Name               string
	UID                k8stypes.UID
	NodeID             string
	Devices            PodDevices
	CtrIDs             []string
	Ctrs               []*Container
}

// NodeHolders contains only allocations attributable to the requested Node UID.
// An ambiguous reservation prevents the affected device from reporting a total.
type NodeHolders struct {
	Containers         []*Container
	UncertainDeviceIDs map[string]bool
}
type NodeHolderReader interface {
	NodeHolders(ctx context.Context, name, uid string) (NodeHolders, error)
}
type PodRepo interface {
	NodeHolderReader
	ListAll(context.Context) ([]*Container, error)
	FindOne(context.Context, string, string) (*Container, error)
}

type PodUseCase struct {
	repo PodRepo
	log  *log.Helper
}

func NewPodUseCase(repo PodRepo, logger log.Logger) *PodUseCase {
	return &PodUseCase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *PodUseCase) NodeHolders(ctx context.Context, name, uid string) (NodeHolders, error) {
	return uc.repo.NodeHolders(ctx, name, uid)
}

func (uc *PodUseCase) ListAllContainers(ctx context.Context) ([]*Container, error) {
	return uc.repo.ListAll(ctx)
}

func (uc *PodUseCase) FindOneContainer(ctx context.Context, podUID string, name string) (*Container, error) {
	return uc.repo.FindOne(ctx, podUID, name)
}

func (uc *PodUseCase) StatisticsByDeviceId(ctx context.Context, deviceId string) (int32, int32, int32, bool, error) {
	containers, err := uc.repo.ListAll(ctx)
	var vGPU int32 = 0
	var core int32 = 0
	var memory int32 = 0
	if err != nil {
		return vGPU, core, memory, false, err
	}
	vGPU, core, memory, coreKnown := ContainersStatisticsInfo(containers, deviceId)
	return vGPU, core, memory, coreKnown, nil
}

func (uc *PodUseCase) ListAll(ctx context.Context) ([]*Container, error) {
	return uc.repo.ListAll(ctx)
}

func ContainersStatisticsInfo(containers []*Container, deviceId string) (int32, int32, int32, bool) {
	return containerStatistics(containers, func(id string) bool { return deviceId == "" || strings.HasPrefix(id, deviceId) })
}

func ExactDeviceStatistics(containers []*Container, deviceID string) (int32, int32, int32, bool) {
	return containerStatistics(containers, func(id string) bool { return deviceID != "" && id == deviceID })
}

func containerStatistics(containers []*Container, matches func(string) bool) (int32, int32, int32, bool) {
	var vGPU, core, memory int32
	coreKnown := true
	for _, t := range containers {
		for _, cd := range t.ContainerDevices {
			if !matches(cd.UUID) {
				continue
			}
			vGPU = vGPU + 1
			core = core + cd.Usedcores
			memory = memory + cd.Usedmem
			if cd.CoreAllocationUnknown {
				coreKnown = false
			}
		}
	}
	return vGPU, core, memory, coreKnown
}
