package provider

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"vgpu/internal/provider/util"
)

type Provider interface {
	GetNodeDevicePluginLabels() (labels.Selector, error)
	GetProvider() string
	FetchDevices(node *corev1.Node) ([]*util.DeviceInfo, error)
}
