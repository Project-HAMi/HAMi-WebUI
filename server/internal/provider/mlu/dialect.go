package mlu

import (
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider"
)

type inventoryDialect struct{}

// NewDialect returns the InventoryDialect for Cambricon MLU resource pools.
func NewDialect() provider.InventoryDialect {
	return inventoryDialect{}
}

func (inventoryDialect) Name() string        { return CambriconMLUDevice }
func (inventoryDialect) SelectorKey() string { return "" }

func (inventoryDialect) OwnsResource(name string, catalog *devicecatalog.Snapshot) bool {
	if strings.HasPrefix(name, CambriconResourcePrefix) &&
		(strings.HasSuffix(name, CambriconCoreResource) || strings.HasSuffix(name, CambriconMemResource)) {
		return true
	}
	if catalog == nil || catalog.Cambricon == nil || !catalog.Loaded() {
		return false
	}
	return name == catalog.Cambricon.ResourceCountName ||
		name == catalog.Cambricon.ResourceCoreName ||
		name == catalog.Cambricon.ResourceMemoryName
}

func (inventoryDialect) OwnsRegistrationAnnotation(string, *devicecatalog.Snapshot) bool {
	return false
}

func (inventoryDialect) ExtractNode(_ *corev1.Node, _ *devicecatalog.Snapshot, _ bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	return provider.NodeInventory{}
}
