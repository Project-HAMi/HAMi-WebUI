package nvidia

import (
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

type Nvidia struct {
	prom *prom.Client
	log  *log.Helper

	labelSelector string
}

func NewNvidia(prom *prom.Client, log *log.Helper, labelSelector string) *Nvidia {
	return &Nvidia{
		prom:          prom,
		log:           log,
		labelSelector: labelSelector,
	}
}

func (n *Nvidia) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(n.labelSelector)
}

func (n *Nvidia) GetProvider() string {
	return biz.NvidiaGPUDevice
}

func (n *Nvidia) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	encoded, ok := node.Annotations[RegisterAnnos]
	if !ok {
		n.log.Warnf("%s node cloud not get hami.io/node-nvidia-register annotation", node.Name)
		return nil, nil
	}
	return registeredDevices(encoded, n.log)
}
