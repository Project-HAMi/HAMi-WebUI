package resourceview

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
)

// DeviceIdentity is the node-scoped provider identity. NodeUID guards cache
// associations; the public stable ID uses Provider/Node/DeviceID. DRA must use
// its own driver/pool/device identity and must not fabricate a Node here.
type DeviceIdentity struct {
	Provider, Node, DeviceID string
	NodeUID                  types.UID
}

type Device struct {
	Identity                                 DeviceIdentity
	Kind                                     string // physical, instance or logical; never derived from a slot count
	ParentIdentity                           *DeviceIdentity
	UUID                                     *string // populated only with proven physical UUID evidence
	Vendor, Model, Mode                      string
	Configured, Healthy                      *bool
	Resource                                 string
	SlotsCapacity                            Quantity
	ComputeCapacityShare                     Quantity
	MemoryCapacityBytes, MemoryPhysicalBytes Quantity
	// Telemetry is separate from registration capacity. Missing never becomes 0.
	MemoryUsedBytes, UtilizationRatio Quantity
	EffectiveOccupancies              []EffectiveOccupancy
	States                            map[string]*viewv1.FieldState
	Sources                           []*viewv1.SourceState
	Coverage                          *viewv1.Coverage
	AllocationCoverage                *viewv1.Coverage // effective physical occupancy scope, independent of device inventory
}

// EffectiveOccupancy is the source-specific physical-accounting input. HAMi
// records use the Pod lifetime peak; persistent claims require another policy.
type EffectiveOccupancy struct {
	PodUID                                                      types.UID
	Source                                                      string
	Device                                                      DeviceIdentity
	Resource                                                    string
	SlotsAllocated, ComputeAllocatedShare, MemoryAllocatedBytes Quantity
	Complete                                                    bool
}

type Node struct {
	InternalIP         string
	SystemInfo         corev1.NodeSystemInfo
	UID                types.UID
	Name               string
	CreatedAt          *time.Time
	Schedulable, Ready *bool
	Resources          []NodeResource
	States             map[string]*viewv1.FieldState
	Sources            []*viewv1.SourceState
	DeviceCoverage     *viewv1.Coverage
}

// Native extended resources without physical identity stay on the Node as
// resource_units. They do not create synthetic Device rows or physical totals.
type NodeResource struct {
	Resource              string
	Capacity, Allocatable Quantity
}

type Snapshot struct {
	At                           time.Time // assembly time, never substituted for field observation time
	Nodes                        []Node
	Devices                      []Device
	NodeCoverage, DeviceCoverage *viewv1.Coverage
}

// SnapshotSource returns detached facts. Implementations use local evidence;
// an API list request must not start network inventory discovery.
type SnapshotSource interface {
	Snapshot(context.Context) (Snapshot, error)
}
