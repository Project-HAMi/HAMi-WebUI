package data

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

const factsNodeIndex = "pod-facts-node"

// NewPodFactsLedger is the production read boundary for device allocation accounting. Construction is
// rooted in NewPodRepo, which registers the new handler on its existing informer.
func NewPodFactsLedger(repo *podRepo) *podfacts.Ledger { return repo.facts }
func (r *podRepo) initFacts(informer cache.SharedIndexInformer, catalog devicecatalog.Source) error {
	if r.facts == nil {
		return nil
	} // legacy unit fixtures may construct a repository directly
	if err := informer.AddIndexers(cache.Indexers{factsNodeIndex: func(obj interface{}) ([]string, error) {
		p, ok := obj.(*corev1.Pod)
		if !ok {
			return nil, nil
		}
		name := p.Spec.NodeName
		if name == "" {
			name = p.Annotations["hami.io/vgpu-node"]
		}
		if name == "" {
			return nil, nil
		}
		return []string{name}, nil
	}}); err != nil {
		return fmt.Errorf("index Pod facts by node: %w", err)
	}
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{AddFunc: r.onFactsPod, UpdateFunc: func(_, current interface{}) { r.onFactsPod(current) }, DeleteFunc: r.onFactsDelete}); err != nil {
		return err
	}
	catalog.Subscribe(func(_, _ *devicecatalog.Snapshot) { r.replayFacts() })
	// A node registration/configuration update can make a previously unknown
	// allocation mode/identity interpretable even if the Pod itself did not change.
	_, err := r.data.informers.Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{AddFunc: r.onFactsNode, UpdateFunc: func(_, current interface{}) { r.onFactsNode(current) }, DeleteFunc: r.onFactsNode})
	return err
}
func (r *podRepo) replayFacts() {
	pods, err := r.podLister.List(labels.Everything())
	if err != nil {
		r.log.Warnf("Pod fact replay: %v", err)
		return
	}
	for _, p := range pods {
		r.onFactsPod(p)
	}
}
func (r *podRepo) onFactsNode(obj interface{}) {
	defer r.recoverEvent("fact node", obj)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	node, ok := obj.(*corev1.Node)
	if !ok || node == nil {
		return
	}
	pods, err := r.podIndexer.ByIndex(factsNodeIndex, node.Name)
	if err != nil {
		return
	}
	for _, pod := range pods {
		r.onFactsPod(pod)
	}
}
func (r *podRepo) onFactsDelete(obj interface{}) {
	defer r.recoverEvent("fact delete", obj)
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	if pod, ok := obj.(*corev1.Pod); ok && pod != nil {
		r.facts.Delete(pod.UID)
	}
}
func (r *podRepo) onFactsPod(obj interface{}) {
	defer r.recoverEvent("fact update", obj)
	pod, ok := obj.(*corev1.Pod)
	if !ok || pod == nil || r.facts == nil {
		return
	}
	snapshot := r.factsCatalog.Snapshot()
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		nodeName = pod.Annotations["hami.io/vgpu-node"]
	}
	var node *corev1.Node
	if nodeName != "" {
		node, _ = r.nodeLister.Get(nodeName)
	}
	evidence, options := r.hamiFactEvidence(pod, node, snapshot)
	var nodeUID types.UID
	if node != nil {
		nodeUID = node.UID
		if original := r.facts.AssignmentNodeUID(pod.UID); original != "" && original != nodeUID {
			for i := range evidence.Allocations {
				evidence.Allocations[i].Source.State, evidence.Allocations[i].Source.Reason = resourceview.Unknown, "identity_conflict"
				evidence.Allocations[i].Source.Params = map[string]string{"identity": "node_uid_replaced"}
				evidence.Allocations[i].Complete = false
				evidence.Allocations[i].HasAllocationEvidence = evidence.Allocations[i].HasAllocationEvidence || len(evidence.Allocations[i].Records) > 0
			}
		}
	}
	facts, err := podfacts.Extract(pod, evidence, options)
	if err != nil {
		r.log.Warnf("Pod facts %s/%s: %v", pod.Namespace, pod.Name, err)
		return
	}
	r.facts.PutWithRevision(facts, func() bool {
		if !r.isCachedVersion(pod) || r.factsCatalog.Snapshot() != snapshot {
			return false
		}
		if nodeName != "" {
			current, err := r.nodeLister.Get(nodeName)
			if node == nil {
				return err != nil
			}
			if err != nil || current.UID != node.UID || current.ResourceVersion != node.ResourceVersion {
				return false
			}
		}
		return true
	}, factInputRevision(snapshot, node), nodeUID)
}

func factInputRevision(catalog *devicecatalog.Snapshot, node *corev1.Node) string {
	// Catalog snapshots are immutable. This process-local identity is never
	// exposed in public output; it detects same-RV configuration reinterpretation.
	if node == nil {
		return fmt.Sprintf("%p/missing-node", catalog)
	}
	return fmt.Sprintf("%p/%s/%s", catalog, node.UID, node.ResourceVersion)
}
