package data

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeInventoryReadsANodeWithoutStatus(t *testing.T) {
	repo := &nodeRepo{}

	node := repo.fetchNodeInfo(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "pending-node", UID: "node-1"},
	})
	if node.Name != "pending-node" || node.OperatingSystem != "" || node.Architecture != "" {
		t.Fatalf("a node without status: %+v", node)
	}

	ready := repo.fetchNodeInfo(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{
			OperatingSystem: "linux",
			Architecture:    "amd64",
		}},
	})
	if ready.OperatingSystem != "Linux" || ready.Architecture != "AMD64" {
		t.Fatalf("a registered node: %+v", ready)
	}
}
