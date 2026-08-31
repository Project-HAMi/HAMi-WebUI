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
	CreateTime       time.Time
	Priority         string
	NodeUID          string
	Namespace        string
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

func ContainersStatisticsInfo(containers []*Container, deviceId string) (int32, int32, int32, bool) {
	var vGPU int32 = 0
	var core int32 = 0
	var memory int32 = 0
	coreKnown := true
	for _, t := range containers {
		for _, cd := range t.ContainerDevices {
			if deviceId != "" && !strings.HasPrefix(cd.UUID, deviceId) {
				continue
			}
			vGPU = vGPU + 1
			core = core + cd.Usedcores
			memory = memory + cd.Usedmem
			if strings.HasPrefix(cd.Type, AscendGPUDevice) && !cd.CoreAllocationKnown {
				coreKnown = false
			}
		}
	}
	return vGPU, core, memory, coreKnown
}
