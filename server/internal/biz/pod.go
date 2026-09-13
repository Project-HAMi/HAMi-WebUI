package biz

import (
	"cmp"
	"context"
	"github.com/go-kratos/kratos/v2/log"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"slices"
	"strings"
	"time"
)

const (
	ContainerKindRegular = "regular"
	ContainerKindInit    = "init"
	ContainerKindSidecar = "sidecar"
)

type Container struct {
	Name             string
	UUID             string
	Kind             string
	ContainerIdx     int // Position in HAMi's device annotation: init containers first.
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
	Namespace string
	Name      string
	UID       k8stypes.UID
	NodeID    string
	Devices   PodDevices
	CtrIDs    []string
	Ctrs      []*Container
}

type PodRepo interface {
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

type allocationUsage struct{ vGPU, core, memory int32 }

func (u allocationUsage) plus(o allocationUsage) allocationUsage {
	return allocationUsage{u.vGPU + o.vGPU, u.core + o.core, u.memory + o.memory}
}

func (u allocationUsage) atLeast(o allocationUsage) allocationUsage {
	return allocationUsage{max(u.vGPU, o.vGPU), max(u.core, o.core), max(u.memory, o.memory)}
}

// ContainersStatisticsInfo follows HAMi's CollapseInitContainerUsage: init at peak, sidecars with apps.
func ContainersStatisticsInfo(containers []*Container, deviceId string) (int32, int32, int32, bool) {
	type deviceState struct{ sidecar, peak, app allocationUsage }
	ordered := slices.Clone(containers)
	slices.SortStableFunc(ordered, func(a, b *Container) int {
		return cmp.Or(strings.Compare(a.PodUID, b.PodUID), cmp.Compare(a.ContainerIdx, b.ContainerIdx))
	})
	states := map[string]*deviceState{}
	coreKnown := true
	for _, t := range ordered {
		for _, cd := range t.ContainerDevices {
			if deviceId != "" && !strings.HasPrefix(cd.UUID, deviceId) {
				continue
			}
			if strings.HasPrefix(cd.Type, AscendGPUDevice) && !cd.CoreAllocationKnown {
				coreKnown = false
			}
			key := t.PodUID + "/" + cd.UUID
			state := states[key]
			if state == nil {
				state = &deviceState{}
				states[key] = state
			}
			usage := allocationUsage{1, cd.Usedcores, cd.Usedmem}
			switch t.Kind {
			case ContainerKindSidecar:
				state.sidecar = state.sidecar.plus(usage)
				state.peak = state.peak.atLeast(state.sidecar)
			case ContainerKindInit:
				state.peak = state.peak.atLeast(state.sidecar.plus(usage))
			default:
				state.app = state.app.plus(usage)
			}
		}
	}
	var total allocationUsage
	for _, state := range states {
		total = total.plus(state.peak.atLeast(state.sidecar.plus(state.app)))
	}
	return total.vGPU, total.core, total.memory, coreKnown
}
