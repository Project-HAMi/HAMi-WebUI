package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
)

func TestAllocationListsKeepRegisteredMemoryCapacity(t *testing.T) {
	const registeredMemory = int32(65536)

	var (
		queriesMu sync.Mutex
		queries   []string
	)
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.FormValue("query")
		queriesMu.Lock()
		queries = append(queries, query)
		queriesMu.Unlock()

		label, value := "device_uuid", "100"
		if strings.Contains(query, "by (node)") {
			label = "node"
		}
		if strings.Contains(query, "hami_memory_size") {
			// A physical-memory metric deliberately differs from the registered
			// schedulable capacity. Allocation APIs must never use this value.
			value = "32768"
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"%s":"%s"},"value":[1788050000,"%s"]}]}}`, label, map[string]string{"node": "node-a", "device_uuid": "GPU-1"}[label], value)
	}))
	defer prometheus.Close()

	device := &biz.DeviceInfo{
		Id:       "GPU-1",
		AliasId:  "GPU-1",
		Count:    1,
		Devmem:   registeredMemory,
		Devcore:  200,
		NodeName: "node-a",
		NodeUid:  "node-uid-a",
	}
	nodeRepo := &capacityTestNodeRepo{
		nodes:   []*biz.Node{{Name: "node-a", Uid: "node-uid-a", Devices: []*biz.DeviceInfo{device}}},
		devices: []*biz.DeviceInfo{device},
	}
	podRepo := &capacityTestPodRepo{}
	nodeUsecase := biz.NewNodeUsecase(nodeRepo, log.DefaultLogger)
	podUsecase := biz.NewPodUseCase(podRepo, log.DefaultLogger)
	promClient, err := prom.NewClient(prometheus.URL, time.Second, prom.HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	monitor := NewMonitorService(promClient, nodeUsecase, podUsecase)

	nodes, err := NewNodeService(nodeUsecase, podUsecase, nil, monitor).GetAllNodes(
		context.Background(),
		&pb.GetAllNodesReq{Filters: &pb.GetAllNodesReq_Filters{}},
	)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if got := nodes.List[0].MemoryTotal; got != registeredMemory {
		t.Fatalf("node memory_total = %d, want registered capacity %d", got, registeredMemory)
	}
	if got := nodes.List[0].CoreTotal; got != 100 {
		t.Fatalf("node core_total = %d, want physical baseline 100", got)
	}

	cards, err := NewCardService(nodeUsecase, podUsecase, monitor).GetAllGPUs(
		context.Background(),
		&pb.GetAllGpusReq{Filters: &pb.GetAllGpusReq_Filters{}},
	)
	if err != nil {
		t.Fatalf("GetAllGPUs: %v", err)
	}
	if got := cards.List[0].MemoryTotal; got != registeredMemory {
		t.Fatalf("card memory_total = %d, want registered capacity %d", got, registeredMemory)
	}
	if got := cards.List[0].CoreTotal; got != 100 {
		t.Fatalf("card core_total = %d, want physical baseline 100", got)
	}

	queriesMu.Lock()
	defer queriesMu.Unlock()
	if len(queries) != 2 {
		t.Fatalf("Prometheus queries = %v, want one core query per list API", queries)
	}
	for _, query := range queries {
		if !strings.Contains(query, "hami_core_size") || strings.Contains(query, "memory_size") {
			t.Fatalf("unexpected allocation-list query %q", query)
		}
	}
}

type capacityTestNodeRepo struct {
	nodes   []*biz.Node
	devices []*biz.DeviceInfo
}

func (r *capacityTestNodeRepo) ListAll(context.Context) ([]*biz.Node, error) {
	return r.nodes, nil
}

func (r *capacityTestNodeRepo) GetNode(context.Context, string) (*biz.Node, error) {
	return r.nodes[0], nil
}

func (r *capacityTestNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return r.devices, nil
}

func (r *capacityTestNodeRepo) FindDeviceByAliasId(aliasID string) (*biz.DeviceInfo, error) {
	for _, device := range r.devices {
		if device.AliasId == aliasID {
			return device, nil
		}
	}
	return nil, nil
}

type capacityTestPodRepo struct{}

func (*capacityTestPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return nil, nil
}

func (*capacityTestPodRepo) FindOne(context.Context, string, string) (*biz.Container, error) {
	return nil, nil
}
