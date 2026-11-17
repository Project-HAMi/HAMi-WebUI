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

type deviceLabels struct {
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
	deviceCnt, _ := node.Status.Capacity.Name(corev1.ResourceName(MeteaxDevice), resource.DecimalSI).AsInt64()
	if deviceCnt == 0 {
		// sGPU registrations identify each card before mx-exporter is scraped.
		sdeviceListEncode, ok := node.Annotations[MetaxSDeviceAnno]
		if !ok {
			m.log.Warnf("%s node missing %s annotation", node.Name, MetaxSDeviceAnno)
			return nil, nil
		}

		var sDeviceList []sDeviceInfo
		err := json.Unmarshal([]byte(sdeviceListEncode), &sDeviceList)
		if err != nil {
			m.log.Warnf("%s node unmarshal sdevice info with %s failed", node.Name, MetaxSDeviceAnno)
			return nil, err
		}

		return m.convertSDevice2Device(sDeviceList, m.getDeviceLabelsFromPrometheus(node)), nil
	}

	var deviceInfos []*util.DeviceInfo
	for uuid, deviceLabels := range m.getDeviceLabelsFromPrometheus(node) {
		deviceInfos = append(deviceInfos, &util.DeviceInfo{
			ID:      uuid,
			AliasId: node.Name + "-metax-" + fmt.Sprint(deviceLabels.deviceId),
			Index:   deviceLabels.deviceId,
			Count:   1,                                                  // if sgpu is disabled, vgpu is gpu
			Devmem:  m.getDeviceMemoryFromPrometheus(node, uuid) / 1024, // KB -> MB
			Devcore: 100,
			Type:    deviceLabels.modelName,
			Health:  deviceLabels.status == DeviceAvaliable,
			Driver:  deviceLabels.driver,
		})
	}

	return deviceInfos, nil
}

func (m *Metax) convertSDevice2Device(sDeviceList []sDeviceInfo, labelsByUUID map[string]*deviceLabels) []*util.DeviceInfo {
	var deviceInfos []*util.DeviceInfo

	for _, sDevice := range sDeviceList {
		if sDevice.Uuid == "" {
			continue
		}

		deviceInfo := &util.DeviceInfo{
			ID:      sDevice.Uuid,
			AliasId: sDevice.Uuid,
			Count:   sDevice.TotalDevCount,
			Devmem:  sDevice.TotalVRam, // MB
			Devcore: sDevice.TotalCompute,
			Numa:    sDevice.Numa,
			Health:  sDevice.Healthy,
		}

		// Some plugin versions register the sharing mode in the model field.
		if sDevice.Model != "sgpu" && sDevice.Model != "native" {
			deviceInfo.Type = sDevice.Model
		}

		if sample, ok := labelsByUUID[sDevice.Uuid]; ok {
			deviceInfo.Index = sample.deviceId
			deviceInfo.Driver = sample.driver
			if sample.modelName != "" {
				deviceInfo.Type = sample.modelName
			}
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

func (m *Metax) getDeviceLabelsFromPrometheus(node *corev1.Node) map[string]*deviceLabels {
	labelsMap := make(map[string]*deviceLabels)

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
		deviceId, err := strconv.ParseUint(deviceIdStr, 10, strconv.IntSize)
		if err != nil {
			m.log.Warnf("Convert deviceId string %s to integer failed", deviceIdStr)
			continue
		}

		deviceStatus, _ := strconv.Atoi(sample.Value.String())

		labelsMap[uuid] = &deviceLabels{
			deviceId:  uint(deviceId),
			driver:    string(sample.Metric["driver_version"]),
			modelName: string(sample.Metric["modelName"]),
			status:    deviceStatus,
		}
	}

	return labelsMap
}
