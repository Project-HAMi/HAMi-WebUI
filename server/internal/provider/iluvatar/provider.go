package iluvatar

import (
	"fmt"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Iluvatar struct {
	prom *prom.Client
	log  *log.Helper

	nodeSelectors string
}

func NewIluvatar(prom *prom.Client, log *log.Helper, nodeSelectors string) *Iluvatar {
	return &Iluvatar{
		prom:          prom,
		log:           log,
		nodeSelectors: nodeSelectors,
	}
}

func (i *Iluvatar) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(i.nodeSelectors)
}

func (i *Iluvatar) GetProvider() string {
	return biz.IluvatarGPUDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func (i *Iluvatar) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	for _, anno := range IluvatarNodeRegisterAnnos {
		anno, ok := node.Annotations[anno]
		if !ok {
			log.Infof("anno %s not found", anno)
			continue
		}
		deviceInfos, err := util.DecodeNodeDevices(anno, i.log)
		return deviceInfos, err
	}
	return []*util.DeviceInfo{}, fmt.Errorf("")
}
