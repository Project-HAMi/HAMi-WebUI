package data

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	listers "k8s.io/client-go/listers/core/v1"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider/registry"
	"vgpu/internal/providerfacts"
	"vgpu/internal/resourceview"
)

type resourceSnapshotSource struct {
	continuity *sharedContinuity
	nodes      listers.NodeLister
	pods       *podfacts.Ledger
	catalog    devicecatalog.Source
	selectors  map[string]labels.Selector
	captures   *CaptureStore
	log        *log.Helper
}

// NewResourceSnapshotSource shares the Node informer already started by the
// Pod ledger dependency. Snapshot has no remote reads or per-request discovery.
func NewResourceSnapshotSource(data *Data, providers []nodeProvider, logger log.Logger, catalog devicecatalog.Source, pods *podfacts.Ledger) resourceview.SnapshotSource {
	selectors := make(map[string]labels.Selector, len(providers))
	for _, p := range providers {
		selectors[p.GetProvider()] = p.selector
	}
	return &resourceSnapshotSource{continuity: data.continuity, nodes: data.informers.Core().V1().Nodes().Lister(), pods: pods, catalog: catalog, selectors: selectors, captures: NewCaptureStore(), log: log.NewHelper(logger)}
}

func (r *resourceSnapshotSource) Snapshot(ctx context.Context) (resourceview.Snapshot, error) {
	capture, err := r.Capture(ctx)
	if err != nil {
		return resourceview.Snapshot{}, err
	}
	return cloneResourcesSnapshot(capture.Resources), nil
}

// Capture builds and publishes an immutable generation-stamped ClusterCapture
// while preserving Pod RV continuity lag protections (lagging Pods remain
// visible with degraded AllocationCoverage rather than disappearing).
func (r *resourceSnapshotSource) Capture(ctx context.Context) (*ClusterCapture, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var nodeTracker, podTracker *informerContinuity
	if r.continuity != nil {
		nodeTracker, podTracker = r.continuity.nodes, r.continuity.pods
	}
	nodeProof, podProof := nodeTracker.snapshot(), podTracker.snapshot()
	nodes, err := r.nodes.List(labels.Everything())
	if err != nil {
		return nil, err
	}
	catalog := r.catalog.Snapshot()
	pods := r.pods.AllocationSnapshot()
	nodeObjects := make([]metav1.Object, 0, len(nodes))
	nodeRevisions := make(map[string]string, len(nodes))
	missingNode := factInputRevision(catalog, nil)
	for _, node := range nodes {
		nodeObjects = append(nodeObjects, node)
		nodeRevisions[node.Name] = factInputRevision(catalog, node)
	}
	podObjects := make([]metav1.Object, 0, len(pods.Entries))
	// An informer gap has cluster scope. A Pod with stale derived facts only
	// affects its bound or preassigned nodes, unless neither can be identified.
	pendingNodes := make(map[string]bool)
	stalePods := make(map[types.UID]bool)
	for _, entry := range pods.Entries {
		f := entry.Facts
		podObjects = append(podObjects, &f.Metadata)
		if f.Terminal {
			continue
		}
		assigned := f.Metadata.Annotations["hami.io/vgpu-node"]
		name := f.NodeName
		if name == "" {
			name = assigned
		}
		revision, ok := nodeRevisions[name]
		if !ok {
			revision = missingNode
		}
		if entry.InputRevision != revision {
			stalePods[f.Metadata.UID] = true
			pendingNodes[name] = true // an empty name means the scope is unknown
			if assigned != "" {
				pendingNodes[assigned] = true
			}
		}
	}
	nodeCoverage := continuityCoverage("node", "shared_node_informer", nodeProof, nodeObjects, nodeTracker.unchanged(nodeProof))
	pods.Coverage = continuityCoverage("pod", "shared_pod_informer", podProof, podObjects, podTracker.unchanged(podProof))
	out := providerfacts.Build(nodes, catalog, pods, r.selectors, r.log, time.Now(), nodeCoverage, registry.DefaultInventoryDialects())
	if len(stalePods) > 0 {
		out.VerifiedBindings = slices.DeleteFunc(out.VerifiedBindings, func(b resourceview.VerifiedDeviceBinding) bool {
			return stalePods[b.PodUID]
		})
	}
	for i := range out.Devices {
		d := &out.Devices[i]
		if pendingNodes[""] || pendingNodes[d.Identity.Node] {
			c := d.AllocationCoverage
			c.Complete, c.Total = false, nil
			c.Sources = append(c.Sources, &viewv1.SourceState{Source: "pod_fact_replay", State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"continuity": "dependent_fact_replay_pending"}})
		}
	}
	// Physical identity and measurements do not depend on Pod events. A Pod
	// gap invalidates allocation totals only; the node catalog remains usable.
	inventoryChanged := !nodeTracker.unchanged(nodeProof) || r.catalog.Snapshot() != catalog
	if inventoryChanged {
		incompleteCoverage(out.NodeCoverage, "source_changed_during_snapshot")
		incompleteCoverage(out.DeviceCoverage, "source_changed_during_snapshot")
		for i := range out.Nodes {
			incompleteCoverage(out.Nodes[i].DeviceCoverage, "source_changed_during_snapshot")
		}
		for i := range out.Devices {
			incompleteCoverage(out.Devices[i].Coverage, "source_changed_during_snapshot")
		}
	}
	if inventoryChanged || !podTracker.unchanged(podProof) {
		for i := range out.Devices {
			incompleteCoverage(out.Devices[i].AllocationCoverage, "source_changed_during_snapshot")
		}
	}
	catalogRV := ""
	if catalog != nil {
		catalogRV = fmt.Sprintf("%p", catalog)
	}
	return r.captures.Publish(catalogRV, out, pods, out.At), nil
}
func incompleteCoverage(c *viewv1.Coverage, reason string) {
	if c == nil {
		return
	}
	c.Complete = false
	c.Total = nil
	for _, source := range c.Sources {
		if source != nil && source.State == resourceview.Known {
			source.State, source.Reason = resourceview.Unknown, "unverified"
			if source.Params == nil {
				source.Params = map[string]string{}
			}
			source.Params["continuity"] = reason
		}
	}
}
