package biz

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"vgpu/internal/podfacts"
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

type DeviceReservation struct {
	Slots, Cores, MemoryMiB int32
	CoreUnknown, HasInit    bool
}

// NodeHolders contains only allocations attributable to the requested Node UID.
// An ambiguous reservation prevents the affected device from reporting a total.
type NodeHolders struct {
	Reservations          map[string]DeviceReservation
	Containers            []*Container
	UncertainDeviceIDs    map[string]bool
	UnreadableAllocations []podfacts.UnreadableAllocation
}

// UnreadableDeviceIDs resolves hints against the current registration. If a
// damaged record cannot identify a card, any card of that provider on this
// node may be affected. Return public IDs so equal aliases across vendors stay separate.
func (h NodeHolders) UnreadableDeviceIDs(devices []*DeviceInfo, node, uid string) map[string]bool {
	if len(h.UnreadableAllocations) == 0 {
		return nil
	}
	byProvider := map[string]map[string]string{}
	for _, d := range devices {
		if d.NodeName != node || d.NodeUid != uid {
			continue
		}
		if byProvider[d.Provider] == nil {
			byProvider[d.Provider] = map[string]string{}
		}
		byProvider[d.Provider][d.AliasId] = d.Id
	}
	affected := map[string]bool{}
	for _, hint := range h.UnreadableAllocations {
		aliases := byProvider[hint.Provider]
		if id, ok := aliases[hint.DeviceID]; ok && hint.DeviceID != "" {
			affected[id] = true
			continue
		}
		for _, id := range aliases {
			affected[id] = true
		}
	}
	return affected
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
	var vGPU, core, memory int32
	coreKnown := true
	for _, t := range containers {
		for _, cd := range t.ContainerDevices {
			if deviceId != "" && !strings.HasPrefix(cd.UUID, deviceId) {
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
