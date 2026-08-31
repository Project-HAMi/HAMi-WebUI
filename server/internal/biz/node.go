package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// PhysicalCoreBaselinePerDevice is the normalized physical-compute denominator
// used by WebUI allocation metrics. It represents one accelerator at 100
// percentage points; it is not a CUDA-core, AI-core, or schedulable-vCore count.
const PhysicalCoreBaselinePerDevice int32 = 100

type Node struct {
	Name                    string
	IP                      string
	IsSchedulable           bool
	IsReady                 bool
	Uid                     string
	OSImage                 string
	OperatingSystem         string
	KernelVersion           string
	ContainerRuntimeVersion string
	KubeletVersion          string
	KubeProxyVersion        string
	Architecture            string
	CreationTimestamp       string
	Devices                 []*DeviceInfo
}

type DeviceInfo struct {
	Index    int
	Id       string
	AliasId  string
	Count    int32
	Devmem   int32
	Devcore  int32
	Type     string
	Numa     int
	Mode     string
	Health   bool
	NodeName string
	NodeUid  string
	Provider string
	Driver   string
}

type DeviceTotal struct {
	VgpuCount int32
	Cores     int32
	Memory    int32
}

type NodeRepo interface {
	ListAll(context.Context) ([]*Node, error)
	GetNode(context.Context, string) (*Node, error)
	ListAllDevices(context.Context) ([]*DeviceInfo, error)
	FindDeviceByAliasId(string) (*DeviceInfo, error)
}

type NodeUsecase struct {
	repo NodeRepo
	log  *log.Helper
}

func NewNodeUsecase(repo NodeRepo, logger log.Logger) *NodeUsecase {
	return &NodeUsecase{repo: repo, log: log.NewHelper(logger)}
}

func (uc *NodeUsecase) ListAllNodes(ctx context.Context) ([]*Node, error) {
	return uc.repo.ListAll(ctx)
}

func (uc *NodeUsecase) GetNode(ctx context.Context, nodeName string) (*Node, error) {
	return uc.repo.GetNode(ctx, nodeName)
}

func (uc *NodeUsecase) ListAllDevices(ctx context.Context) ([]*DeviceInfo, error) {
	return uc.repo.ListAllDevices(ctx)
}

func (uc *NodeUsecase) FindDeviceByAliasId(aliasId string) (*DeviceInfo, error) {
	return uc.repo.FindDeviceByAliasId(aliasId)
}
