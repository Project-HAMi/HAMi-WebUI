package hygon

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"strconv"
	"strings"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Hygon struct {
	prom *prom.Client
	log  *log.Helper
}

func NewHygon(prom *prom.Client, log *log.Helper) *Hygon {
	return &Hygon{
		prom: prom,
		log:  log,
	}
}

func (c *Hygon) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse("dcu=on")
}

func (c *Hygon) GetProvider() string {
	return HygonDCUDevice
}

type DeviceMeta struct {
	UUID   string
	Type   string
	Driver string
}

func (c *Hygon) GetDevicesFromPrometheus(node *corev1.Node) map[string]*util.DeviceInfo {
	deviceMap := make(map[string]*util.DeviceInfo)
	queryString := fmt.Sprintf("dcu_temp{node=\"%s\"}", node.Name)

	vs, err := c.prom.Query(context.Background(), queryString)
	if err != nil {
		c.log.Warnf("Failed to query %s: %v", queryString, err)
		return deviceMap
	}

	vector, ok := vs.(model.Vector)
	if !ok {
		c.log.Warnf("Unexpected result type: %v", vs)
		return deviceMap
	}

	for _, sample := range vector {
		minorNumber := string(sample.Metric["minor_number"])
		index, _ := strconv.Atoi(minorNumber)
		deviceMap[minorNumber] = &util.DeviceInfo{
			ID:    string(sample.Metric["device_id"]),
			Index: uint(index),
		}
	}

	return deviceMap
}

func (c *Hygon) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	devEncoded, ok := node.Annotations[RegisterAnnos]
	if !ok {
		return []*util.DeviceInfo{}, errors.New("annos not found " + RegisterAnnos)
	}
	nodedevices, err := util.DecodeNodeDevices(devEncoded, c.log)
	if err != nil {
		c.log.Errorw("failed to decode node devices", err, "node", node.Name, "device annotation", devEncoded)
		return []*util.DeviceInfo{}, err
	}
	if len(nodedevices) == 0 {
		c.log.Infow("event", "no gpu device found", "node", node.Name, "device annotation", devEncoded)
		return []*util.DeviceInfo{}, errors.New("no gpu found on node")
	}
	devDecoded := util.EncodeNodeDevices(nodedevices, c.log)
	c.log.Infow("event", "nodes device information", "node", node.Name, "nodedevices", devDecoded)
	devDetail := c.GetDevicesFromPrometheus(node)
	for _, nodedevice := range nodedevices {
		idParts := strings.Split(nodedevice.ID, "-")
		if len(idParts) < 2 {
			c.log.Warnf("Invalid nodedevice.ID format: %s", nodedevice.ID)
			continue
		}

		devDetailID := idParts[1]
		devInfo, exists := devDetail[devDetailID]
		if !exists {
			c.log.Warnf("Device ID %s not found in devDetail", devDetailID)
			continue
		}

		nodedevice.ID = devInfo.ID
		nodedevice.AliasId = fmt.Sprintf("%s-dcu-%d", node.Name, devInfo.Index)
	}
	return nodedevices, nil
}
