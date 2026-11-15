package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kratosjson "github.com/go-kratos/kratos/v2/encoding/json"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

func fleetWithDevices(n int) overviewSource {
	src := overviewSource{At: time.Unix(600, 0), NodeCoverage: &pb.Coverage{Complete: true}, DeviceCoverage: &pb.Coverage{Complete: true}}
	for i := range n {
		if i%8 == 0 {
			src.Nodes = append(src.Nodes, testNode(fmt.Sprintf("gpu-%05d", i/8), proto.Bool(true)))
		}
		id := fmt.Sprintf("GPU-01234567-89ab-cdef-0123-%012d", i)
		d := testCard(src.Nodes[i/8], "NVIDIA", id, id, "NVIDIA-A100-SXM4-80GB")
		d.UtilizationRatio = testMetric("0", "ratio")
		d.MemoryUsedBytes = testMetric("20Gi", "bytes")
		d.MemoryPhysicalBytes = testMetric("80Gi", "bytes")
		src.Devices = append(src.Devices, d)
	}
	return src
}

type countedInventorySource struct {
	overviewSource
	reads int
}

func (s *countedInventorySource) Snapshot(ctx context.Context) (resourceview.Snapshot, error) {
	s.reads++
	return s.overviewSource.Snapshot(ctx)
}

func TestOverviewInventoryAcrossReplicas(t *testing.T) {
	replica := func(s *countedInventorySource) http.Handler {
		server := kratoshttp.NewServer()
		pb.RegisterOverviewHTTPServer(server, &OverviewService{source: s})
		return server.Handler
	}
	get := func(s http.Handler, path string, dst proto.Message) {
		t.Helper()
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: status %d", path, w.Code)
		} else if err := protojson.Unmarshal(w.Body.Bytes(), dst); err != nil {
			t.Fatal(err)
		}
	}
	a, b := &countedInventorySource{overviewSource: fleetWithDevices(501)}, &countedInventorySource{overviewSource: fleetWithDevices(501)}
	b.At = b.At.Add(time.Minute)
	b.Nodes[0].UID = "replacement"
	for i := range 8 {
		b.Devices[i].Identity.NodeUID = b.Nodes[0].UID
	}
	b.Devices[0].UtilizationRatio = testMetric("0.75", "ratio")
	firstReplica, secondReplica := replica(a), replica(b)

	for _, tc := range []struct {
		handler http.Handler
		source  *countedInventorySource
		compute float64
	}{{firstReplica, a, 0}, {secondReplica, b, 0.75}} {
		var inv pb.InventoryReply
		reads := tc.source.reads
		get(tc.handler, "/api/v1/overview/inventory", &inv)
		if tc.source.reads != reads+1 {
			t.Fatal("node and device inventories require exactly one source read")
		}
		if len(inv.Devices) != 501 || len(inv.Nodes) != 63 || inv.DeviceCoverage.GetObserved() != 501 ||
			inv.NodeCoverage.GetObserved() != 63 || inv.Nodes[0].Uid != string(tc.source.Nodes[0].UID) || !inv.SnapshotAt.AsTime().Equal(tc.source.At) {
			t.Fatal("mixed or incomplete inventory")
		}
		d := inv.Devices[0]
		if d.Node != inv.Nodes[0].Name || d.UtilizationRatio == nil || *d.UtilizationRatio != tc.compute || d.GetMemoryUsageRatio() != 0.25 {
			t.Fatalf("device readings: %v", d)
		}
	}
}

func TestCompactInventoryOmitsUnprovenFields(t *testing.T) {
	src := fleetWithDevices(5)
	src.DeviceCoverage.Complete = false
	src.Devices[0].MemoryUsedBytes = testMetric("81Gi", "bytes")
	src.Devices[1].MemoryUsedBytes = testMetric("0", "bytes")
	src.Devices[2].MemoryPhysicalBytes = testMetric("0", "bytes")
	src.Devices[3].MemoryUsedBytes.State.Source = "different"
	src.Devices[3].UtilizationRatio.State.State = resourceview.Stale
	src.Devices[3].Healthy = nil
	src.Devices[4].MemoryUsedBytes = testMetric("-1", "bytes")
	src.Nodes[0].Ready = nil
	out := makeInventory(resourceview.Snapshot(src)).compactInventory()
	if out.DeviceCoverage.Complete || !out.NodeCoverage.Complete || out.Nodes[0].Ready != nil || out.Devices[3].Healthy != nil || out.Devices[3].UtilizationRatio != nil {
		t.Fatal("partial evidence was promoted to known")
	}
	if out.Devices[1].MemoryUsageRatio == nil || *out.Devices[1].MemoryUsageRatio != 0 {
		t.Fatal("zero memory usage is a reading")
	}
	for _, i := range []int{0, 2, 3, 4} {
		if out.Devices[i].MemoryUsageRatio != nil {
			t.Errorf("card %d has unproven memory usage", i)
		}
	}
}

func TestCompactInventoryReconcilesAliasesAndOrphans(t *testing.T) {
	src := fleetWithDevices(2)
	first := src.Devices[0]
	alias := first
	alias.Identity.Provider = "other"
	alias.UtilizationRatio = testMetric("0.5", "ratio")
	orphan := src.Devices[1]
	orphan.Identity.NodeUID = "deleted-incarnation"
	src.Devices = append(src.Devices, alias, orphan)
	inv := makeInventory(resourceview.Snapshot(src))
	all := inv.compactInventory()
	if len(all.Devices) != 2 || len(all.Nodes) != 1 || all.Nodes[0].GetPhysicalDevices() != 2 || all.DeviceCoverage.Complete || all.DeviceCoverage.Observed != 2 {
		t.Fatalf("membership after adding an alias and an orphan: %v", all)
	}
	for i, card := range all.Devices {
		want := src.Devices[i].Identity
		if card.DeviceId != want.DeviceID || card.Node != want.Node || card.Provider != want.Provider {
			t.Fatalf("card %d: %v; expected %v", i, card, want)
		}
	}
	if all.Devices[0].UtilizationRatio != nil {
		t.Fatal("conflicting alias samples promoted to a reading")
	}
}

func jsonSize(b *testing.B, m proto.Message, err error) int {
	b.Helper()
	if err != nil {
		b.Fatal(err)
	}
	data, err := kratosjson.MarshalOptions.Marshal(m)
	if err != nil {
		b.Fatal(err)
	}
	return len(data)
}

// Includes service projection and uncompressed production JSON encoding. The
// fixture supplies a ready in-memory source, not a Kubernetes informer refresh.
func BenchmarkOverviewInventory(b *testing.B) {
	for _, n := range []int{501, 10000} {
		source := fleetWithDevices(n)
		b.Run(fmt.Sprintf("compact/%d", n), func(b *testing.B) {
			s := OverviewService{source: source}
			b.ReportAllocs()
			for b.Loop() {
				r, err := s.GetInventory(b.Context(), &pb.InventoryRequest{})
				b.ReportMetric(float64(jsonSize(b, r, err)), "json-bytes/op")
			}
		})
	}
}
