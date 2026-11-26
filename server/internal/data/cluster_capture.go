package data

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

// ClusterCapture is an immutable, generation-stamped snapshot of cluster facts
// with prebuilt primary-key lookup indexes. Callers obtain O(1) lookup by
// canonical DeviceID, NodeUID, NodeName, PodUID, or ContainerKey; filtered list
// scans, sorting, and TopN aggregations continue to enforce explicit budgets.
type ClusterCapture struct {
	Generation uint64
	CapturedAt time.Time
	CatalogRV  string
	Resources  resourceview.Snapshot
	Pods       podfacts.Inventory

	ByDeviceID  map[string]*resourceview.Device
	ByNodeUID   map[types.UID]*resourceview.Node
	ByNodeName  map[string]*resourceview.Node
	ByPodUID    map[types.UID]*podfacts.Entry
	ByContainer map[podfacts.ContainerKey]*podfacts.Container
}

// BuildClusterCapture constructs an immutable ClusterCapture and populates its
// primary-key indexes in a single pass.
func BuildClusterCapture(
	generation uint64,
	catalogRV string,
	resources resourceview.Snapshot,
	pods podfacts.Inventory,
	capturedAt time.Time,
) *ClusterCapture {
	resources.Nodes = slices.Clone(resources.Nodes)
	resources.Devices = slices.Clone(resources.Devices)
	pods.Entries = slices.Clone(pods.Entries)
	for i := range pods.Entries {
		if pods.Entries[i].Facts == nil {
			continue
		}
		factsCopy := *pods.Entries[i].Facts
		factsCopy.Containers = slices.Clone(factsCopy.Containers)
		pods.Entries[i].Facts = &factsCopy
	}
	capture := &ClusterCapture{
		Generation:  generation,
		CapturedAt:  capturedAt,
		CatalogRV:   catalogRV,
		Resources:   resources,
		Pods:        pods,
		ByDeviceID:  make(map[string]*resourceview.Device, len(resources.Devices)),
		ByNodeUID:   make(map[types.UID]*resourceview.Node, len(resources.Nodes)),
		ByNodeName:  make(map[string]*resourceview.Node, len(resources.Nodes)),
		ByPodUID:    make(map[types.UID]*podfacts.Entry, len(pods.Entries)),
		ByContainer: make(map[podfacts.ContainerKey]*podfacts.Container, len(pods.Entries)*2),
	}
	for i := range capture.Resources.Nodes {
		node := &capture.Resources.Nodes[i]
		if node.UID != "" {
			if _, exists := capture.ByNodeUID[node.UID]; !exists {
				capture.ByNodeUID[node.UID] = node
			}
		}
		if node.Name != "" {
			if _, exists := capture.ByNodeName[node.Name]; !exists {
				capture.ByNodeName[node.Name] = node
			}
		}
	}
	for i := range capture.Resources.Devices {
		device := &capture.Resources.Devices[i]
		id, err := resourceview.DeviceID(device.Identity.Provider, device.Identity.Node, device.Identity.DeviceID)
		if err != nil || id == "" {
			continue
		}
		if _, exists := capture.ByDeviceID[id]; !exists {
			capture.ByDeviceID[id] = device
		}
	}
	for i := range capture.Pods.Entries {
		entry := &capture.Pods.Entries[i]
		if entry.Facts == nil {
			continue
		}
		uid := entry.Facts.Metadata.UID
		if uid != "" {
			if _, exists := capture.ByPodUID[uid]; !exists {
				capture.ByPodUID[uid] = entry
			}
		}
		for j := range entry.Facts.Containers {
			ctr := &entry.Facts.Containers[j]
			if ctr.Key.PodUID == "" || ctr.Key.Name == "" {
				continue
			}
			if _, exists := capture.ByContainer[ctr.Key]; !exists {
				capture.ByContainer[ctr.Key] = ctr
			}
		}
	}
	return capture
}

func (c *ClusterCapture) LookupDevice(opaqueDeviceID string) (*resourceview.Device, bool) {
	if c == nil || opaqueDeviceID == "" {
		return nil, false
	}
	d, ok := c.ByDeviceID[opaqueDeviceID]
	return d, ok
}

func (c *ClusterCapture) LookupNode(uid types.UID) (*resourceview.Node, bool) {
	if c == nil || uid == "" {
		return nil, false
	}
	n, ok := c.ByNodeUID[uid]
	return n, ok
}

func (c *ClusterCapture) LookupNodeByName(name string) (*resourceview.Node, bool) {
	if c == nil || name == "" {
		return nil, false
	}
	n, ok := c.ByNodeName[name]
	return n, ok
}

func (c *ClusterCapture) LookupPod(uid types.UID) (*podfacts.Entry, bool) {
	if c == nil || uid == "" {
		return nil, false
	}
	p, ok := c.ByPodUID[uid]
	return p, ok
}

func (c *ClusterCapture) LookupContainer(key podfacts.ContainerKey) (*podfacts.Container, bool) {
	if c == nil || key.PodUID == "" || key.Name == "" {
		return nil, false
	}
	ctr, ok := c.ByContainer[key]
	return ctr, ok
}

// CaptureStore publishes immutable ClusterCapture generations in strict
// monotonic order while keeping Load lock-free.
type CaptureStore struct {
	mu      sync.Mutex
	seq     uint64
	current atomic.Pointer[ClusterCapture]
}

func NewCaptureStore() *CaptureStore {
	return &CaptureStore{}
}

func (s *CaptureStore) Publish(
	catalogRV string,
	resources resourceview.Snapshot,
	pods podfacts.Inventory,
	capturedAt time.Time,
) *ClusterCapture {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	capture := BuildClusterCapture(s.seq, catalogRV, resources, pods, capturedAt)
	s.current.Store(capture)
	return capture
}

func (s *CaptureStore) Load() *ClusterCapture {
	return s.current.Load()
}
