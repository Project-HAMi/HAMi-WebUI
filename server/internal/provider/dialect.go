package provider

import (
	"math/big"
	"slices"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/util"
	"vgpu/internal/resourceview"
)

// NodeInventory carries registered accelerator devices and source states
// extracted by one InventoryDialect from a single Kubernetes Node.
type NodeInventory struct {
	Devices []resourceview.Device
	Sources []*viewv1.SourceState
}

// InventoryDialect encapsulates vendor-specific node registration parsing,
// resource ownership, and catalog mapping behind a uniform contract.
type InventoryDialect interface {
	// Name returns the stable dialect identifier.
	Name() string
	// SelectorKey returns the key in the node-selector map gating registration
	// extraction, or "" when the dialect has no registration selector.
	SelectorKey() string
	// OwnsResource reports whether a node Capacity/Allocatable resource name
	// belongs to this accelerator family.
	OwnsResource(name string, catalog *devicecatalog.Snapshot) bool
	// OwnsRegistrationAnnotation reports whether a node annotation key belongs
	// to this dialect's registration surface.
	OwnsRegistrationAnnotation(key string, catalog *devicecatalog.Snapshot) bool
	// ExtractNode extracts registered devices and source states from node.
	// When selected is false, registration devices must not be materialized.
	ExtractNode(node *corev1.Node, catalog *devicecatalog.Snapshot, selected bool, logger *log.Helper, observedAt time.Time) NodeInventory
}

// KnownState returns a known FieldState with the given source and scope.
func KnownState(source, scope string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Known, Source: source, Scope: scope}
}

// UnknownState returns an unknown FieldState with the given source, scope, and reason.
func UnknownState(source, scope, reason string) *viewv1.FieldState {
	return &viewv1.FieldState{State: resourceview.Unknown, Source: source, Scope: scope, Reason: reason}
}

// UnknownQuantity returns an absent Quantity with the given unit and provenance.
func UnknownQuantity(unit, source, scope, reason string) resourceview.Quantity {
	return resourceview.Quantity{Unit: unit, State: UnknownState(source, scope, reason)}
}

// ExactRatQuantity parses an exact non-repeating decimal representation of a rational share.
func ExactRatQuantity(num, den int64, unit, source, scope string) (resourceview.Quantity, bool) {
	if den <= 0 {
		return resourceview.Quantity{}, false
	}
	ratio := new(big.Rat).SetFrac64(num, den)
	decimal := ratio.FloatString(9)
	exact, ok := new(big.Rat).SetString(decimal)
	if !ok || exact.Cmp(ratio) != 0 {
		return resourceview.Quantity{}, false
	}
	val := resource.MustParse(decimal)
	return resourceview.Quantity{Value: &val, Unit: unit, State: KnownState(source, scope)}, true
}

// SetPercentCompute sets ComputeCapacityShare from devcore percentage when known and exact.
func SetPercentCompute(dev *resourceview.Device, registered util.RegistrationDevice) {
	if !registered.Known["devcore"] {
		return
	}
	if q, ok := ExactRatQuantity(int64(registered.Device.Devcore), 100, "share", "node_registration", "device"); ok {
		dev.ComputeCapacityShare = q
	} else {
		dev.ComputeCapacityShare = UnknownQuantity("share", "allocation", "device", "unverified")
	}
}

// HasNodeResource reports whether node exposes resourceName in Capacity or Allocatable.
func HasNodeResource(node *corev1.Node, resourceName string) bool {
	if node == nil || resourceName == "" {
		return false
	}
	key := corev1.ResourceName(resourceName)
	if _, ok := node.Status.Capacity[key]; ok {
		return true
	}
	_, ok := node.Status.Allocatable[key]
	return ok
}

// ApplyCatalogResourceNames populates Configured, Resource, and ResourceKeys from explicit names.
func ApplyCatalogResourceNames(dev *resourceview.Device, node *corev1.Node, configured bool, names devicecatalog.DeviceResourceNames) {
	dev.Configured = &configured
	dev.States["configured"] = KnownState("device_config", "device")
	if names.Count != "" {
		if HasNodeResource(node, names.Count) {
			dev.Resource = names.Count
			dev.States["resource"] = KnownState("device_config_and_node_status", "device")
		} else {
			dev.States["resource"] = UnknownState("device_config_and_node_status", "device", "unverified")
			dev.States["resource"].Params = map[string]string{"expectedResource": names.Count}
		}
	}
	for _, key := range []string{names.Count, names.Memory, names.Compute} {
		if key != "" && !slices.Contains(dev.ResourceKeys, key) {
			dev.ResourceKeys = append(dev.ResourceKeys, key)
		}
	}
}

// ApplyNodeResourceMapping populates Resource, Configured, and ResourceKeys from catalog.ResourceNames.
func ApplyNodeResourceMapping(dev *resourceview.Device, node *corev1.Node, catalog *devicecatalog.Snapshot, configKey string, fallbackKeys ...string) {
	if catalog == nil || !catalog.Loaded() {
		return
	}
	names, present := catalog.ResourceNames[configKey]
	for _, fallback := range fallbackKeys {
		if present || fallback == "" {
			break
		}
		names, present = catalog.ResourceNames[fallback]
	}
	if present {
		ApplyCatalogResourceNames(dev, node, present, names)
	}
}

// NewRegistrationDevice builds a baseline resourceview.Device from a decoded registration record.
func NewRegistrationDevice(node *corev1.Node, catalog *devicecatalog.Snapshot, providerName, vendor, configKey string, registered util.RegistrationDevice, fallbackConfigKeys ...string) resourceview.Device {
	device := registered.Device
	dev := resourceview.Device{
		Identity: resourceview.DeviceIdentity{
			NodeUID:  node.UID,
			Node:     node.Name,
			Provider: providerName,
			DeviceID: device.ID,
		},
		Kind:                 "physical",
		Vendor:               vendor,
		Model:                device.Type,
		Mode:                 device.Mode,
		ComputeCapacityShare: UnknownQuantity("share", "node_registration", "device", "unverified"),
		MemoryPhysicalBytes:  UnknownQuantity("bytes", "telemetry", "device", "unverified"),
		MemoryUsedBytes:      UnknownQuantity("bytes", "telemetry", "device", "unverified"),
		UtilizationRatio:     UnknownQuantity("ratio", "telemetry", "device", "unverified"),
		Sources:              []*viewv1.SourceState{{Source: "hami_registration:" + providerName, State: resourceview.Known}},
		States: map[string]*viewv1.FieldState{
			"kind":       KnownState("node_registration", "device"),
			"model":      KnownState("node_registration", "device"),
			"mode":       KnownState("node_registration", "device"),
			"health":     KnownState("node_registration", "device"),
			"uuid":       UnknownState("node_registration", "device", "unverified"),
			"configured": UnknownState("device_config", "device", "missing_config"),
			"resource":   UnknownState("device_config", "device", "missing_config"),
		},
	}
	if !registered.Known["type"] {
		dev.States["model"] = UnknownState("node_registration", "device", "unverified")
	}
	if !registered.Known["mode"] {
		dev.Mode = ""
		dev.States["mode"] = UnknownState("node_registration", "device", "unverified")
		dev.States["mode"].Params = map[string]string{"raw": registered.RawMode}
	}
	if registered.Known["health"] {
		healthy := device.Health
		dev.Healthy = &healthy
	} else {
		dev.States["health"] = UnknownState("node_registration", "device", "unverified")
	}
	if registered.Known["count"] {
		dev.SlotsCapacity = resourceview.Quantity{
			Value: resource.NewQuantity(int64(device.Count), resource.DecimalSI),
			Unit:  "slots",
			State: KnownState("node_registration", "device"),
		}
	} else {
		dev.SlotsCapacity = UnknownQuantity("slots", "node_registration", "device", "unverified")
	}
	if registered.Known["devmem"] {
		dev.MemoryCapacityBytes = resourceview.Quantity{
			Value: resource.NewQuantity(int64(device.Devmem)*1024*1024, resource.BinarySI),
			Unit:  "bytes",
			State: KnownState("node_registration", "device"),
		}
	} else {
		dev.MemoryCapacityBytes = UnknownQuantity("bytes", "node_registration", "device", "unverified")
	}
	if configKey != "" {
		ApplyNodeResourceMapping(&dev, node, catalog, configKey, fallbackConfigKeys...)
	}
	return dev
}

// DecodeAnnotationInventory decodes a single-annotation node registration into
// Devices and a hami_registration source state using shared error handling.
func DecodeAnnotationInventory(
	node *corev1.Node,
	selected bool,
	providerName string,
	annoKey string,
	decode func(raw string) ([]util.RegistrationDevice, error),
	build func(reg util.RegistrationDevice) resourceview.Device,
) NodeInventory {
	if !selected || node == nil {
		return NodeInventory{}
	}
	raw, ok := node.Annotations[annoKey]
	if !ok {
		return NodeInventory{}
	}
	source := &viewv1.SourceState{Source: "hami_registration:" + providerName, State: resourceview.Known}
	regs, err := decode(raw)
	if err != nil {
		source.State, source.Reason = resourceview.Unknown, "unverified"
	}
	devices := make([]resourceview.Device, 0, len(regs))
	for _, reg := range regs {
		devices = append(devices, build(reg))
	}
	return NodeInventory{
		Devices: devices,
		Sources: []*viewv1.SourceState{source},
	}
}
