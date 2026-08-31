package ascend

import (
	"context"
	"fmt"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/common/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sort"
	"strings"
	"time"
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
				vdieID := string(d.Metric["vdie_id"])
				if vdieID == "" {
					continue
				}
				health := false
				if d.Value.Equal(1) {
					health = true
				}
				device[vdieID] = &util.DeviceInfo{
					ID:     vdieID,
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
	nodeDevices, err := decodeRegisteredDevices(node)
	if err != nil {
		return nil, err
	}
	if len(nodeDevices) == 0 {
		return []*util.DeviceInfo{}, nil
	}

	telemetryDevices := a.GetDevicesFromPrometheus(node)
	for i, matched := range reconcileRegisteredDevices(nodeDevices, telemetryDevices) {
		if !matched {
			log.Infof("Ascend device %s (index %d) not found in Prometheus telemetry", nodeDevices[i].AliasId, nodeDevices[i].Index)
		}
	}
	return nodeDevices, nil
}

func reconcileRegisteredDevices(nodeDevices []*util.DeviceInfo, telemetryDevices map[string]*util.DeviceInfo) []bool {
	matched := make([]bool, len(nodeDevices))
	for i, nodeDevice := range nodeDevices {
		nodeDevices[i].AliasId = nodeDevice.ID
		if telemetryDevice, exists := telemetryDevices[nodeDevice.ID]; exists {
			nodeDevices[i].Health = telemetryDevice.Health
			matched[i] = true
		}
	}
	return matched
}

func decodeRegisteredDevices(node *corev1.Node) ([]*util.DeviceInfo, error) {
	type candidate struct {
		device    *util.DeviceInfo
		reported  time.Time
		hasReport bool
		key       string
	}
	candidates := make(map[string]candidate)
	annotationKeys := make([]string, 0)
	const (
		registerBase   = "hami.io/node-register-"
		registerPrefix = registerBase + "Ascend"
		handshakeBase  = "hami.io/node-handshake-"
	)
	for annotationKey := range node.Annotations {
		if strings.HasPrefix(annotationKey, registerPrefix) && len(annotationKey) > len(registerBase) {
			annotationKeys = append(annotationKeys, annotationKey)
		}
	}
	sort.Strings(annotationKeys)
	for _, annotationKey := range annotationKeys {
		commonWord := strings.TrimPrefix(annotationKey, registerBase)
		annotation := node.Annotations[annotationKey]
		nodeDevices, err := util.UnMarshalNodeDevices(annotation)
		if err != nil {
			return nil, fmt.Errorf("decode %s on node %s: %w", annotationKey, node.Name, err)
		}
		reported, hasReport := parseAscendReportedTime(node.Annotations[handshakeBase+commonWord])
		for _, device := range nodeDevices {
			if device == nil {
				return nil, fmt.Errorf("decode %s on node %s: nil device record", annotationKey, node.Name)
			}
			if device.ID == "" || device.Type != commonWord {
				return nil, fmt.Errorf("decode %s on node %s: device id/type does not match commonWord %q", annotationKey, node.Name, commonWord)
			}
			// The plugin reports physical AI Core count in hard mode (for
			// example 20), while WebUI's allocation contract is a normalized
			// whole-card percentage. Keep the inventory denominator consistent
			// with template shares and hami-core mode.
			device.Devcore = 100
			incoming := candidate{device: device, reported: reported, hasReport: hasReport, key: annotationKey}
			existing, duplicate := candidates[device.ID]
			if !duplicate {
				candidates[device.ID] = incoming
				continue
			}
			replace := incoming.hasReport && (!existing.hasReport || incoming.reported.After(existing.reported))
			if replace {
				candidates[device.ID] = incoming
			}
			if existing.device.Type != incoming.device.Type {
				log.Warnf("conflicting Ascend registrations for device %s on node %s: %s and %s", device.ID, node.Name, existing.key, incoming.key)
			}
		}
	}
	ordered := make([]candidate, 0, len(candidates))
	for _, registered := range candidates {
		ordered = append(ordered, registered)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].device.Index != ordered[j].device.Index {
			return ordered[i].device.Index < ordered[j].device.Index
		}
		return ordered[i].device.ID < ordered[j].device.ID
	})
	devices := make([]*util.DeviceInfo, 0, len(ordered))
	for _, registered := range ordered {
		devices = append(devices, registered.device)
	}
	return devices, nil
}

func parseAscendReportedTime(value string) (time.Time, bool) {
	const prefix = "Reported_"
	if !strings.HasPrefix(value, prefix) {
		return time.Time{}, false
	}
	reported, err := time.Parse("2006.01.02 15:04:05", strings.TrimPrefix(value, prefix))
	return reported, err == nil
}
