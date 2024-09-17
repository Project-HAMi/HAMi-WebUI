package ascend

import (
	"context"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"strings"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Ascend struct {
	prom *prom.Client
	log  *log.Helper
}

func NewAscend(prom *prom.Client, log *log.Helper) *Ascend {
	return &Ascend{
		prom: prom,
		log:  log,
	}
}

func (c *Ascend) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse("servertype=Ascend910B-20")
}

func (c *Ascend) GetProvider() string {
	return AscendDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func (c *Ascend) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	device := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("npu_chip_info_health_status{node=\"%s\"}", node.Name)
	vs, err := c.prom.Query(context.Background(), queryString)
	if err != nil {
		c.log.Warnf("query %s failed", queryString)
	} else {
		ds, ok := vs.(model.Vector)
		if !ok {
			c.log.Warnf("vectorValue: %v, failed", vs)
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

func (c *Ascend) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {

	nodedevices := []*util.DeviceInfo{}
	i := 0
	cards, _ := node.Status.Capacity.Name(corev1.ResourceName(AscendResourceCoreCount), resource.DecimalSI).AsInt64()
	tmpDevice := c.GetDevicesFromPrometheus(node)
	for int64(i)*10 < cards {
		index := fmt.Sprintf("%d", i)
		if _, ok := tmpDevice[index]; !ok {
			i++
			continue
		}
		mode := strings.Split(tmpDevice[index].Type, "-")
		nodedevices = append(nodedevices, &util.DeviceInfo{
			Index:   uint(i),
			ID:      tmpDevice[index].ID,
			AliasId: node.Name + "-Ascend910-" + fmt.Sprint(i),
			Count:   10,
			Devmem:  int32(65536),
			Devcore: 100,
			Type:    fmt.Sprintf("%s-%s", mode[1], mode[0]),
			Numa:    0,
			Health:  true,
			Driver:  "xxx",
		})
		i++
	}
	return nodedevices, nil
}
