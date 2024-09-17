package nvidia

import (
	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Nvidia struct {
	prom *prom.Client
	log  *log.Helper
}

func NewNvidia(prom *prom.Client, log *log.Helper) *Nvidia {
	return &Nvidia{
		prom: prom,
		log:  log,
	}
}

func (n *Nvidia) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse("gpu=on")
}

func (n *Nvidia) GetProvider() string {
	return biz.NvidiaGPUDevice
}

func (n *Nvidia) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	var err error
	var deviceInfos []*util.DeviceInfo

	deviceEncode, ok := node.Annotations[RegisterAnnos]
	if !ok {
		n.log.Warnf("%s node cloud not get hami.io/node-nvidia-register annotation", node.Name)
		return deviceInfos, nil
	}
	deviceInfos, err = util.DecodeNodeDevices(deviceEncode, n.log)
	return deviceInfos, err
}
