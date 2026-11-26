package registry

import (
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider"
	"vgpu/internal/provider/ascend"
	"vgpu/internal/provider/hygon"
	"vgpu/internal/provider/metax"
	"vgpu/internal/provider/mlu"
	"vgpu/internal/provider/nvidia"
)

type resourcePoolDialect struct {
	name     string
	exact    map[string]bool
	prefixes []string
}

func (d resourcePoolDialect) Name() string        { return d.name }
func (d resourcePoolDialect) SelectorKey() string { return "" }

func (d resourcePoolDialect) OwnsResource(name string, _ *devicecatalog.Snapshot) bool {
	if d.exact[name] {
		return true
	}
	for _, prefix := range d.prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (resourcePoolDialect) OwnsRegistrationAnnotation(string, *devicecatalog.Snapshot) bool {
	return false
}

func (resourcePoolDialect) ExtractNode(_ *corev1.Node, _ *devicecatalog.Snapshot, _ bool, _ *log.Helper, _ time.Time) provider.NodeInventory {
	return provider.NodeInventory{}
}

// DefaultInventoryDialects returns all built-in vendor InventoryDialects
// plus native resource-pool accelerator dialects.
func DefaultInventoryDialects() []provider.InventoryDialect {
	return []provider.InventoryDialect{
		nvidia.NewInventoryDialect(),
		hygon.NewHCUDialect(),
		hygon.NewDCUDialect(),
		metax.NewSGPUDialect(),
		ascend.NewDialect(),
		mlu.NewDialect(),
		resourcePoolDialect{
			name: "native-accelerator-pools",
			exact: map[string]bool{
				"mthreads.com/gpu":         true,
				"mthreads.com/vgpu":        true,
				"mthreads.com/sgpu-core":   true,
				"mthreads.com/sgpu-memory": true,
				"amd.com/gpu":              true,
			},
			prefixes: []string{"amd.com/gpu-"},
		},
	}
}

// AllocationConfig supplies runtime policy inputs for Pod allocation dialects.
type AllocationConfig struct {
	AscendAnnotationlessPolicy string
}

// DefaultAllocationDialects returns all built-in vendor AllocationDialects.
func DefaultAllocationDialects(cfg AllocationConfig) []provider.AllocationDialect {
	return []provider.AllocationDialect{
		nvidia.NewAllocationDialect(),
		hygon.NewHCUAllocationDialect(),
		hygon.NewDCUAllocationDialect(),
		metax.NewAllocationDialect(),
		mlu.NewAllocationDialect(),
		ascend.NewAllocationDialect(cfg.AscendAnnotationlessPolicy),
	}
}

// DefaultTelemetryDialects returns all built-in vendor TelemetryDialects.
func DefaultTelemetryDialects() []provider.TelemetryDialect {
	return []provider.TelemetryDialect{
		nvidia.NewTelemetryDialect(),
		ascend.NewTelemetryDialect(),
		hygon.NewHCUTelemetryDialect(),
		hygon.NewDCUTelemetryDialect(),
		metax.NewTelemetryDialect(),
	}
}
