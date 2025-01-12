package ascend

import (
	"context"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strconv"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Ascend struct {
	prom *prom.Client
	log  *log.Helper

	nodeSelectors string
}

func NewAscend(prom *prom.Client, log *log.Helper, nodeSelectors string) *Ascend {
	return &Ascend{
		prom:          prom,
		log:           log,
		nodeSelectors: nodeSelectors,
	}
}

func (a *Ascend) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(a.nodeSelectors)
}

func (a *Ascend) GetProvider() string {
	return AscendDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func (a *Ascend) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	device := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("npu_chip_info_health_status{node=\"%s\"}", node.Name)
	vs, err := a.prom.Query(context.Background(), queryString)
	if err != nil {
		a.log.Warnf("query %s failed", queryString)
	} else {
		ds, ok := vs.(model.Vector)
		if !ok {
			a.log.Warnf("vectorValue: %v, failed", vs)
		} else {
			for _, d := range ds {
				id := d.Metric["id"]
				health := false
				if d.Value.Equal(1) {
					health = true
				}
				device[string(id)] = &util.DeviceInfo{
					ID:     string(d.Metric["vdie_id"]),
					Type:   string(d.Metric["model_name"]),
					Driver: "-",
					Health: health,
				}
			}
		}
	}
	return device
}

func (a *Ascend) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	for _, anno := range AscendNodeRegisterAnnos {
		tmpDevice := a.GetDevicesFromPrometheus(node)
		anno, ok := node.Annotations[anno]
		if !ok {
			log.Infof("anno %s not found", anno)
			continue
		}
		nodeDevices, err := util.UnMarshalNodeDevices(anno)
		if err != nil {
			return []*util.DeviceInfo{}, err
		}
		for i, nodedevice := range nodeDevices {
			nodeDevices[i].AliasId = nodedevice.ID
			if device, exists := tmpDevice[strconv.Itoa(i)]; exists {
				nodeDevices[i].ID = device.ID
			} else {
				log.Infof("Key %d not found in tmpDevice", i)
			}
		}
		return nodeDevices, nil
	}
	return []*util.DeviceInfo{}, fmt.Errorf("")
}
