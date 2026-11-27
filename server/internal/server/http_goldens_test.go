package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	obsv1 "vgpu/api/observability/v1"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"
	"vgpu/internal/resourceview"
	"vgpu/internal/service"
	"vgpu/internal/telemetry"
)

type goldenSnapshotSource struct{ snapshot resourceview.Snapshot }

func (s goldenSnapshotSource) Snapshot(context.Context) (resourceview.Snapshot, error) {
	return s.snapshot, nil
}

type goldenObservations struct {
	values resourceview.DeviceObservations
}

func (o goldenObservations) DeviceObservations() resourceview.DeviceObservations {
	return o.values
}

type goldenNodeRepo struct {
	nodes   []*biz.Node
	devices []*biz.DeviceInfo
}

func (r *goldenNodeRepo) ListAll(context.Context) ([]*biz.Node, error) { return r.nodes, nil }
func (r *goldenNodeRepo) GetNode(context.Context, string) (*biz.Node, error) {
	if len(r.nodes) == 0 {
		return nil, os.ErrNotExist
	}
	return r.nodes[0], nil
}
func (r *goldenNodeRepo) ListAllDevices(context.Context) ([]*biz.DeviceInfo, error) {
	return r.devices, nil
}
func (r *goldenNodeRepo) FindDeviceByAliasId(aliasID string) (*biz.DeviceInfo, error) {
	for _, d := range r.devices {
		if d.AliasId == aliasID {
			return d, nil
		}
	}
	return nil, os.ErrNotExist
}

type goldenPodRepo struct{ containers []*biz.Container }

func (r *goldenPodRepo) NodeHolders(context.Context, string, string) (biz.NodeHolders, error) {
	return biz.NodeHolders{}, nil
}
func (r *goldenPodRepo) ListAll(context.Context) ([]*biz.Container, error) {
	return r.containers, nil
}
func (r *goldenPodRepo) FindOne(_ context.Context, podUID, name string) (*biz.Container, error) {
	for _, c := range r.containers {
		if c.PodUID == podUID && c.Name == name {
			return c, nil
		}
	}
	return nil, os.ErrNotExist
}

type goldenSchedulingRepo struct{}

func (goldenSchedulingRepo) ListSchedulingPods(context.Context) ([]*biz.SchedulingPod, error) {
	return nil, nil
}
func (goldenSchedulingRepo) GetSchedulingPod(context.Context, string, string, string) (*biz.SchedulingDetail, error) {
	return nil, os.ErrNotExist
}

func newGoldenQuantity(dec string, unit, source string) resourceview.Quantity {
	q := resource.MustParse(dec)
	return resourceview.Quantity{
		Value: &q,
		Unit:  unit,
		State: &obsv1.FieldState{
			State:      resourceview.Known,
			Source:     source,
			Scope:      "device",
			ObservedAt: timestamppb.New(time.Unix(1700000000, 0).UTC()),
		},
	}
}

type goldenRangeQuerier struct {
	at          time.Time
	circuitOpen bool
}

func (q goldenRangeQuerier) QueryRange(_ context.Context, _ string, _ promv1.Range) (model.Value, error) {
	if q.circuitOpen {
		return nil, prom.ErrCircuitOpen
	}
	end := q.at.Truncate(time.Minute)
	return model.Matrix{
		&model.SampleStream{
			Values: []model.SamplePair{
				{Timestamp: model.Time(end.Add(-time.Minute).UnixMilli()), Value: 42.5},
				{Timestamp: model.Time(end.UnixMilli()), Value: 50},
			},
		},
	}, nil
}

func newGoldenHTTPHandler(empty bool) nethttp.Handler {
	logger := log.NewStdLogger(io.Discard)
	at := time.Unix(1700000000, 0).UTC()

	var snap resourceview.Snapshot
	var obs resourceview.ObservationReader
	nodeRepo := &goldenNodeRepo{}
	podRepo := &goldenPodRepo{}

	if empty {
		knownSrc := []*obsv1.SourceState{{Source: "shared_node_informer", State: resourceview.Known}}
		snap = resourceview.Snapshot{
			At:             at,
			Nodes:          []resourceview.Node{},
			Devices:        []resourceview.Device{},
			NodeCoverage:   &obsv1.Coverage{Entity: "node", Observed: 0, Total: proto.Int32(0), Complete: true, Sources: knownSrc},
			DeviceCoverage: &obsv1.Coverage{Entity: "device", Observed: 0, Total: proto.Int32(0), Complete: true, Sources: knownSrc},
		}
	} else {
		ready, schedulable, healthy, configured := true, true, true, true
		knownNodeField := &obsv1.FieldState{State: resourceview.Known, Source: "node_spec", Scope: "node"}
		knownDevField := &obsv1.FieldState{State: resourceview.Known, Source: "node_registration", Scope: "device"}
		knownSrc := []*obsv1.SourceState{{Source: "shared_node_informer", State: resourceview.Known}}
		uuid := "GPU-1"
		devID := resourceview.DeviceIdentity{Provider: "NVIDIA", Node: "node-1", NodeUID: types.UID("node-uid-1"), DeviceID: "GPU-1"}
		snap = resourceview.Snapshot{
			At: at,
			Nodes: []resourceview.Node{{
				UID:         types.UID("node-uid-1"),
				Name:        "node-1",
				Ready:       &ready,
				Schedulable: &schedulable,
				States: map[string]*obsv1.FieldState{
					"ready":       knownNodeField,
					"schedulable": knownNodeField,
				},
				Sources:        knownSrc,
				DeviceCoverage: &obsv1.Coverage{Entity: "device", Observed: 1, Total: proto.Int32(1), Complete: true, Sources: knownSrc},
			}},
			Devices: []resourceview.Device{{
				Identity:   devID,
				Kind:       "physical",
				UUID:       &uuid,
				Vendor:     "nvidia",
				Model:      "NVIDIA A100",
				Healthy:    &healthy,
				Configured: &configured,
				States: map[string]*obsv1.FieldState{
					"kind":   knownDevField,
					"model":  knownDevField,
					"health": knownDevField,
					"uuid":   knownDevField,
				},
				Sources:             knownSrc,
				Coverage:            &obsv1.Coverage{Entity: "device", Observed: 1, Total: proto.Int32(1), Complete: true, Sources: knownSrc},
				MemoryCapacityBytes: newGoldenQuantity("85899345920", "bytes", "node_registration"),
			}},
			NodeCoverage:   &obsv1.Coverage{Entity: "node", Observed: 1, Total: proto.Int32(1), Complete: true, Sources: knownSrc},
			DeviceCoverage: &obsv1.Coverage{Entity: "device", Observed: 1, Total: proto.Int32(1), Complete: true, Sources: knownSrc},
		}
		obs = goldenObservations{values: resourceview.DeviceObservations{
			devID: {
				UtilizationRatio:    newGoldenQuantity("0.42", "ratio", "prometheus"),
				MemoryUsedBytes:     newGoldenQuantity("21474836480", "bytes", "prometheus"),
				MemoryPhysicalBytes: newGoldenQuantity("85899345920", "bytes", "prometheus"),
			},
		}}

		dev := &biz.DeviceInfo{
			Id:       "GPU-1",
			AliasId:  "GPU-1",
			Count:    4,
			Devmem:   81920,
			Devcore:  100,
			Type:     "NVIDIA A100",
			Mode:     "hami-core",
			Health:   true,
			NodeName: "node-1",
			NodeUid:  "node-uid-1",
			Provider: biz.NvidiaGPUDevice,
		}
		nodeRepo.nodes = []*biz.Node{{Name: "node-1", Uid: "node-uid-1", IsReady: true, IsSchedulable: true, Devices: []*biz.DeviceInfo{dev}}}
		nodeRepo.devices = []*biz.DeviceInfo{dev}
		podRepo.containers = []*biz.Container{{
			Name:       "trainer",
			PodUID:     "pod-uid-1",
			PodName:    "train-job-0",
			Namespace:  "default",
			NodeName:   "node-1",
			NodeUID:    "node-uid-1",
			Status:     biz.ContainerStatusSuccess,
			CreateTime: at,
			ContainerDevices: biz.ContainerDevices{{
				UUID:      "GPU-1",
				Type:      "NVIDIA A100",
				Vendor:    biz.NvidiaGPUDevice,
				Usedmem:   20480,
				Usedcores: 50,
				Shape:     biz.SplitShapeSoft,
			}},
		}}
	}

	nodeUC := biz.NewNodeUsecase(nodeRepo, logger)
	podUC := biz.NewPodUseCase(podRepo, logger)
	schedUC := biz.NewSchedulingUsecase(goldenSchedulingRepo{})
	ctrSvc := service.NewContainerService(nodeUC, podUC)

	telemetrySvc := service.NewTelemetryServiceWithReader(
		telemetry.NewHistoricalObservationReaderWithQuerier(goldenRangeQuerier{at: at, circuitOpen: empty}),
	)

	return NewHTTPServer(
		&conf.Bootstrap{Server: &conf.Server{Http: &conf.Server_HTTP{}}},
		nil,
		service.NewCardService(nodeUC, podUC),
		ctrSvc,
		nil,
		nil,
		service.NewWorkloadService(ctrSvc, schedUC),
		nil,
		service.NewOverviewService(resourceview.WithObservations(goldenSnapshotSource{snapshot: snap}, obs)),
		telemetrySvc,
		nil,
		prometheus.NewRegistry(),
		logger,
	).Handler
}

func TestHTTPJSONGoldens(t *testing.T) {
	goldensDir := filepath.Join("..", "..", "..", "test", "contracts", "http-goldens")
	update := os.Getenv("UPDATE_HTTP_GOLDENS") == "1"

	cases := []struct {
		name       string
		empty      bool
		method     string
		target     string
		body       string
		wantStatus int
	}{
		{name: "overview-inventory.json", empty: false, method: "GET", target: "/api/v1/overview/inventory", wantStatus: 200},
		{name: "overview-inventory-empty.json", empty: true, method: "GET", target: "/api/v1/overview/inventory", wantStatus: 200},
		{name: "gpu-detail-ok.json", empty: false, method: "GET", target: "/v1/gpu?uid=GPU-1", wantStatus: 200},
		{name: "gpu-detail-not-found.json", empty: false, method: "GET", target: "/v1/gpu?uid=GPU-MISSING", wantStatus: 404},
		{name: "workloads-page.json", empty: false, method: "POST", target: "/v1/workloads", body: `{"page":1,"pageSize":100,"filters":{"deviceId":"GPU-1"}}`, wantStatus: 200},
		{name: "workloads-empty.json", empty: true, method: "POST", target: "/v1/workloads", body: `{"page":1,"pageSize":100,"filters":{"deviceId":"GPU-1"}}`, wantStatus: 200},
		{name: "telemetry-trends-cluster.json", empty: false, method: "GET", target: "/api/v1/telemetry/trends?scope=cluster&start=2023-11-14T22:11:00Z&end=2023-11-14T22:13:00Z&step=60s", wantStatus: 200},
		{name: "telemetry-trends-circuit-open.json", empty: true, method: "GET", target: "/api/v1/telemetry/trends?scope=cluster&start=2023-11-14T22:11:00Z&end=2023-11-14T22:13:00Z&step=60s", wantStatus: 200},
		{name: "telemetry-trends-invalid.json", empty: false, method: "GET", target: "/api/v1/telemetry/trends?scope=invalid", wantStatus: 400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newGoldenHTTPHandler(tc.empty)
			var reqBody io.Reader
			if tc.body != "" {
				reqBody = bytes.NewBufferString(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.target, reqBody)
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("%s %s status=%d want=%d body=%s", tc.method, tc.target, rec.Code, tc.wantStatus, rec.Body.String())
			}

			var formatted bytes.Buffer
			if err := json.Indent(&formatted, rec.Body.Bytes(), "", "  "); err != nil {
				t.Fatalf("invalid JSON response %s: %v\nraw=%s", tc.name, err, rec.Body.String())
			}
			formatted.WriteByte('\n')

			goldenPath := filepath.Join(goldensDir, tc.name)
			if update {
				if err := os.MkdirAll(goldensDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(goldenPath, formatted.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_HTTP_GOLDENS=1 to generate)", goldenPath, err)
			}
			var gotObj, wantObj any
			if err := json.Unmarshal(rec.Body.Bytes(), &gotObj); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(want, &wantObj); err != nil {
				t.Fatal(err)
			}
			gotCanonical, _ := json.Marshal(gotObj)
			wantCanonical, _ := json.Marshal(wantObj)
			if !bytes.Equal(gotCanonical, wantCanonical) {
				t.Fatalf("HTTP JSON golden drift for %s:\ngot:  %s\nwant: %s", tc.name, rec.Body.String(), string(want))
			}
		})
	}
}
