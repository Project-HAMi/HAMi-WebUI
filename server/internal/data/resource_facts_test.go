package data

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"vgpu/internal/conf"
	"vgpu/internal/devicecatalog"
)

func TestResourceFactsProductionSourceUsesLocalSharedCaches(t *testing.T) {
	p := factTestPod()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: "node-uid", ResourceVersion: "1", Annotations: map[string]string{"hami.io/node-nvidia-register": "GPU-a,10,16384,100,T4,0,true:"}}}
	client := fake.NewSimpleClientset(p, node)
	stop := make(chan struct{})
	defer close(stop)
	data := &Data{k8sCl: client, informers: informers.NewSharedInformerFactory(client, 0), stopCh: stop}
	catalog := &mutableCatalog{current: loadedTestCatalog(t)}
	catalog.current.ResourceNames = map[string]devicecatalog.DeviceResourceNames{"NVIDIA": {Count: "nvidia.com/gpu"}}
	logger := log.NewStdLogger(io.Discard)
	repo, err := NewPodRepo(data, logger, &conf.Bootstrap{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewResourceSnapshotSource(data, map[string]string{"NVIDIA": ""}, logger, catalog, NewPodFactsLedger(repo))
	if err != nil {
		t.Fatal(err)
	}
	if err := wait.PollUntilContextTimeout(context.Background(), time.Millisecond*10, time.Second*3, true, func(context.Context) (bool, error) {
		snapshot, err := source.Snapshot(context.Background())
		return len(snapshot.Devices) == 1 && len(snapshot.Devices[0].EffectiveOccupancies) == 1, err
	}); err != nil {
		t.Fatal(err)
	}
	before := len(client.Actions())
	first, err := source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Devices[0].EffectiveOccupancies[0].ComputeAllocatedShare.Unit != "share" || first.Devices[0].ComputeCapacityShare.Unit != "share" {
		t.Fatal("share unit changed at provider boundary")
	}
	first.Devices[0].Identity.Node = "mutated"
	first.Nodes[0].Name = "mutated"
	second, err := source.Snapshot(context.Background())
	if err != nil || second.Devices[0].Identity.Node != "node-a" || second.Nodes[0].Name != "node-a" {
		t.Fatal("read mutated shared facts")
	}
	if len(client.Actions()) != before {
		t.Fatal("resource snapshot performed network reads")
	}
	card := second.Devices[0]
	if v, ok := card.MemoryCapacityBytes.Value.AsInt64(); !ok || v != 16384*1024*1024 || card.MemoryUsedBytes.Value != nil || second.DeviceCoverage.Complete {
		t.Fatal("snapshot changed capacity units or fabricated telemetry/completeness")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Snapshot(canceled); err != context.Canceled {
		t.Fatal("canceled snapshot ignored")
	}
}
