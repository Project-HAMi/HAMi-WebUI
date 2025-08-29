// Copyright (c) 2025 MetaX Integrated Circuits (Shanghai) Co., Ltd.  All rights reserved.
package metax

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"

	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

type Metax struct {
	prom *prom.Client
	log  *log.Helper

	labelSelector string
}

type sDeviceInfo struct {
	Uuid          string `json:"uuid"`
	Bdf           string `json:"bdf"`
	Model         string `json:"model"`
	TotalDevCount int32  `json:"totalDevCount"`
	TotalCompute  int32  `json:"totalCompute"`
	TotalVRam     int32  `json:"totalVRam"`
	Numa          int    `json:"numa"`
	Healthy       bool   `json:"healthy"`
}

type deviceLables struct {
	deviceId  uint
	driver    string
	modelName string
	status    int
}

func NewMetax(prom *prom.Client, log *log.Helper, labelSelector string) *Metax {
	return &Metax{
		prom:          prom,
		log:           log,
		labelSelector: labelSelector,
	}
}

func (m *Metax) GetNodeDevicePluginLabels() (labels.Selector, error) {
	return labels.Parse(m.labelSelector)
}

func (m *Metax) GetProvider() string {
	return biz.MetaxGPUDevice
}

func (m *Metax) FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	var err error
	var deviceInfos []*util.DeviceInfo

	deviceLableMap := m.getDeviceLabelsFromPrometheus(node)
	if len(deviceLableMap) == 0 {
		m.log.Warnf("device labels of node %s is empty, check remote mx-export or prometheus available", node.Name)
		return deviceInfos, nil
	}

	deviceCnt, _ := node.Status.Capacity.Name(corev1.ResourceName(MeteaxDevice), resource.DecimalSI).AsInt64()
	if deviceCnt == 0 {
		// get sdevice info from  annotation "metax-tech.com/node-gpu-devices"
		sdeviceListEncode, ok := node.Annotations[MetaxSDeviceAnno]
		if !ok {
			m.log.Warnf("%s node cloud not get %s annotation", node.Name, MetaxSDeviceAnno)
			return deviceInfos, nil
		}

		var sDeviceList []*sDeviceInfo
		err := json.Unmarshal([]byte(sdeviceListEncode), &sDeviceList)
		if err != nil {
			m.log.Warnf("%s node unmarshal sdevice info with %s failed", node.Name, MetaxSDeviceAnno)
			return deviceInfos, err
		}

		return m.convertSDevice2Device(sDeviceList, deviceLableMap), nil
	}

	for uuid, deviceLabels := range deviceLableMap {

		deviceInfos = append(deviceInfos, &util.DeviceInfo{
			ID:      uuid,
			AliasId: node.Name + "-metax-" + fmt.Sprint(deviceLabels.deviceId),
			Index:   deviceLabels.deviceId,
			Count:   1,                                                  // if sgpu is disabled, vgpu is gpu
			Devmem:  m.getDeviceMemoryFromPrometheus(node, uuid) / 1024, // KB -> MB
			Devcore: 100,
			Mode:    "hami-core",
			Type:    deviceLabels.modelName,
			Health:  deviceLabels.status == DeviceAvaliable,
			Driver:  deviceLabels.driver,
		})
	}

	return deviceInfos, err
}

func (m *Metax) convertSDevice2Device(sDeviceList []*sDeviceInfo, lablesMap map[string]*deviceLables) []*util.DeviceInfo {
	var deviceInfos []*util.DeviceInfo

	for _, sDevice := range sDeviceList {

		deviceInfo := &util.DeviceInfo{
			ID:      sDevice.Uuid,
			Count:   sDevice.TotalDevCount,
			Devmem:  sDevice.TotalVRam, // MB
			Devcore: sDevice.TotalCompute,
			Numa:    sDevice.Numa,
			Mode:    "hami-core",
			Health:  sDevice.Healthy,
		}

		if deviceLalels, ok := lablesMap[sDevice.Uuid]; ok {
			deviceInfo.Index = deviceLalels.deviceId
			deviceInfo.AliasId = sDevice.Uuid
			deviceInfo.Driver = deviceLalels.driver
			deviceInfo.Type = deviceLalels.modelName
		}

		deviceInfos = append(deviceInfos, deviceInfo)
	}

	return deviceInfos
}

func (m *Metax) getDeviceMemoryFromPrometheus(node *corev1.Node, deviceUUID string) int32 {
	var totalMem int32

	query := fmt.Sprintf("mx_memory_total{Hostname=\"%s\", uuid=\"%s\", type=\"vram\"}", node.Name, deviceUUID)
	res, err := m.prom.Query(context.Background(), query)
	if err != nil {
		m.log.Warnf("Failed to query %s: %v", query, err)
		return totalMem
	}

	resVec, ok := res.(model.Vector)
	if !ok {
		m.log.Warnf("Unexpect result type: %v", res)
		return totalMem
	}

	if len(resVec) > 0 {
		totalMem = int32(resVec[0].Value)
	}

	return totalMem
}

func (m *Metax) getDeviceLabelsFromPrometheus(node *corev1.Node) map[string]*deviceLables {
	labelsMap := make(map[string]*deviceLables)

	query := fmt.Sprintf("mx_gpu_state{Hostname=\"%s\"}", node.Name)
	res, err := m.prom.Query(context.Background(), query)
	if err != nil {
		m.log.Warnf("Failed to query %s: %v", query, err)
		return labelsMap
	}

	resVec, ok := res.(model.Vector)
	if !ok {
		m.log.Warnf("Unexpect result type: %v", res)
		return labelsMap
	}

	for _, sample := range resVec {
		uuid := string(sample.Metric["uuid"])
		if uuid == "" {
			continue
		}

		deviceIdStr := string(sample.Metric["deviceId"])
		deviceId, err := strconv.Atoi(deviceIdStr)
		if err != nil {
			m.log.Warnf("Convert deviceId string %s to integer failed", deviceIdStr)
			continue
		}

		deviceStatus, _ := strconv.Atoi(sample.Value.String())

		labelsMap[uuid] = &deviceLables{
			deviceId:  uint(deviceId),
			driver:    string(sample.Metric["driver_version"]),
			modelName: string(sample.Metric["modelName"]),
			status:    deviceStatus,
		}
	}

	return labelsMap
}
