package mlu

import (
	"context"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strings"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Cambricon struct {
	prom *prom.Client
	log  *log.Helper

	labelsSelector string
}

func NewCambricon(prom *prom.Client, log *log.Helper, labelSelector string) *Cambricon {
	return &Cambricon{
		prom:           prom,
		log:            log,
		labelsSelector: labelSelector,
	}
}

func (c *Cambricon) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(c.labelsSelector)
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

type smluResources struct {
	cards, memoryTotal int64
}

// smluCapacity pairs vcore with vmemory per resource name, which --enable-device-type changes.
// The plugin registers them separately, so a rename can leave a half-registered name on the node.
func smluCapacity(node *corev1.Node) (smluResources, error) {
	byName := map[string]smluResources{}
	for name, quantity := range node.Status.Capacity {
		resourceName, ok := strings.CutPrefix(string(name), CambriconResourcePrefix)
		if !ok {
			continue
		}
		if base, ok := strings.CutSuffix(resourceName, CambriconCoreResource); ok {
			resources := byName[base]
			resources.cards = quantity.Value()
			byName[base] = resources
		} else if base, ok := strings.CutSuffix(resourceName, CambriconMemResource); ok {
			resources := byName[base]
			resources.memoryTotal = quantity.Value()
			byName[base] = resources
		}
	}
	var found smluResources
	for _, resources := range byName {
		if resources.cards == 0 || resources.memoryTotal == 0 {
			continue
		}
		if found != (smluResources{}) && found != resources {
			return smluResources{}, fmt.Errorf("node %s has conflicting Cambricon capacity: %v", node.Name, byName)
		}
		found = resources
	}
	return found, nil
}

func (c *Cambricon) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	nodedevices := []*util.DeviceInfo{}
	i := 0
	capacity, err := smluCapacity(node)
	if err != nil {
		return nil, err
	}
	cards, memoryTotal := capacity.cards, capacity.memoryTotal
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
			Devmem:  int32(memoryTotal * CambriconMemoryFactor * 100 / cards),
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
