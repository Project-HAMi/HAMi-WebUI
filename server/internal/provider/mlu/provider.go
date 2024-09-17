package mlu

import (
	"context"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Cambricon struct {
	prom *prom.Client
	log  *log.Helper
}

func NewCambricon(prom *prom.Client, log *log.Helper) *Cambricon {
	return &Cambricon{
		prom: prom,
		log:  log,
	}
}

func (c *Cambricon) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse("mlu=on")
}

func (c *Cambricon) GetProvider() string {
	return CambriconMLUDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func (c *Cambricon) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	device := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("mlu_health{node=\"%s\"}", node.Name)
	vs, err := c.prom.Query(context.Background(), queryString)
	if err != nil {
		c.log.Warnf("query %s failed", queryString)
	} else {
		ds, ok := vs.(model.Vector)
		if !ok {
			c.log.Warnf("vectorValue: %v, failed", vs)
		} else {
			for _, d := range ds {
				id := d.Metric["mlu"]
				health := false
				if d.Value.Equal(1) {
					health = true
				}
				device[string(id)] = &util.DeviceInfo{
					ID:     string(d.Metric["uuid"]),
					Type:   string(d.Metric["model"]),
					Driver: string(d.Metric["driver"]),
					Health: health,
				}
			}
		}
	}
	return device
}

func (c *Cambricon) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	nodedevices := []*util.DeviceInfo{}
	i := 0
	cards, _ := node.Status.Capacity.Name(corev1.ResourceName(CambriconDeviceCoreAnnos), resource.DecimalSI).AsInt64()
	memoryTotal, _ := node.Status.Capacity.Name(corev1.ResourceName(CambriconDeviceMemAnnos), resource.DecimalSI).AsInt64()
	tmpDevice := c.GetDevicesFromPrometheus(node)
	for int64(i)*100 < cards {
		index := fmt.Sprintf("%d", i)
		if _, ok := tmpDevice[index]; !ok {
			i++
			continue
		}
		nodedevices = append(nodedevices, &util.DeviceInfo{
			Index:   uint(i),
			ID:      tmpDevice[index].ID,
			AliasId: node.Name + "-cambricon-mlu-" + fmt.Sprint(i),
			Count:   10,
			Devmem:  int32(memoryTotal * CambriconMemUnit * 100 / cards),
			Devcore: 100,
			Type:    tmpDevice[index].Type,
			Numa:    0,
			Health:  tmpDevice[index].Health,
			Driver:  tmpDevice[index].Driver,
		})
		i++
	}
	return nodedevices, nil
}
