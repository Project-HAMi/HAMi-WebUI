package exporter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/data/prom"
	"vgpu/internal/resourceview"
)

func TestSelfObservabilityCollectorEmitsBoundedClientSplitsAndObservationStates(t *testing.T) {
	var failMode atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if failMode.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "unavailable")
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	t.Cleanup(server.Close)

	client, err := prom.NewClient(server.URL, time.Second, prom.HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create prom client: %v", err)
	}
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("successful query failed: %v", err)
	}
	failMode.Store(true)
	for i := 0; i < 4; i++ {
		_, _ = client.Query(context.Background(), "up")
	}

	at := time.Unix(1760000000, 0).UTC()
	now := at
	idKnown := resourceview.DeviceIdentity{Provider: "NVIDIA", Node: "n1", NodeUID: types.UID("u1"), DeviceID: "GPU-1"}
	idConflict := resourceview.DeviceIdentity{Provider: "NVIDIA", Node: "n1", NodeUID: types.UID("u1"), DeviceID: "GPU-2"}
	g := &MetricsGenerator{
		promClient: client,
		now:        func() time.Time { return now },
		observations: resourceview.DeviceObservations{
			idKnown: {
				UtilizationRatio:    knownResource("0.5", "ratio"),
				MemoryUsedBytes:     knownResource("1Gi", "bytes"),
				MemoryPhysicalBytes: resourceview.Quantity{Unit: "bytes", State: &viewv1.FieldState{State: resourceview.Unknown, Reason: "sample_missing"}},
			},
			idConflict: {
				UtilizationRatio: resourceview.Quantity{Unit: "ratio", State: &viewv1.FieldState{State: resourceview.Conflict, Reason: "ambiguous_source_or_identity"}},
			},
		},
	}
	// Stamp known observations: UtilizationRatio is 2m old, MemoryUsedBytes is fresh at `at`.
	obsKnown := g.observations[idKnown]
	obsKnown.UtilizationRatio.State.ObservedAt = timestamppb.New(at.Add(-2 * time.Minute))
	obsKnown.MemoryUsedBytes.State.ObservedAt = timestamppb.New(at)
	g.observations[idKnown] = obsKnown

	g.splitCounts[splitReasonOverflow].Add(2)
	g.splitCounts[splitReasonSeriesLimit].Add(1)
	g.splitCounts[splitReasonTimeout].Add(3)

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(newSelfObservabilityCollector(g))

	want := `
# HELP hami_webui_prom_circuit_breaker_open Whether the Prometheus query circuit breaker is currently open or half-open (1) vs closed (0).
# TYPE hami_webui_prom_circuit_breaker_open gauge
hami_webui_prom_circuit_breaker_open 1
# HELP hami_webui_prom_circuit_breaker_opens_total Total transitions of the Prometheus query circuit breaker to the open state.
# TYPE hami_webui_prom_circuit_breaker_opens_total counter
hami_webui_prom_circuit_breaker_opens_total 1
# HELP hami_webui_prom_queries_total Total bounded Prometheus queries grouped by outcome.
# TYPE hami_webui_prom_queries_total counter
hami_webui_prom_queries_total{outcome="canceled"} 0
hami_webui_prom_queries_total{outcome="circuit_open"} 1
hami_webui_prom_queries_total{outcome="error"} 3
hami_webui_prom_queries_total{outcome="ok"} 1
hami_webui_prom_queries_total{outcome="response_too_large"} 0
# HELP hami_webui_telemetry_batch_splits_total Total telemetry batches split into smaller fallback chunks, grouped by trigger reason.
# TYPE hami_webui_telemetry_batch_splits_total counter
hami_webui_telemetry_batch_splits_total{reason="response_too_large"} 2
hami_webui_telemetry_batch_splits_total{reason="series_limit"} 1
hami_webui_telemetry_batch_splits_total{reason="timeout"} 3
# HELP hami_webui_telemetry_observations Number of published physical-device observation fields by freshness/verification state.
# TYPE hami_webui_telemetry_observations gauge
hami_webui_telemetry_observations{state="conflict"} 1
hami_webui_telemetry_observations{state="known"} 2
hami_webui_telemetry_observations{state="stale"} 0
hami_webui_telemetry_observations{state="unknown"} 1
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(want)); err != nil {
		t.Fatalf("self observability mismatch:\n%v", err)
	}

	// Advancing by 4m pushes UtilizationRatio past TelemetryMaxSampleAge (6m > 5m) while MemoryUsedBytes stays fresh (4m <= 5m).
	now = at.Add(4 * time.Minute)
	wantAged := strings.ReplaceAll(want,
		`hami_webui_telemetry_observations{state="known"} 2
hami_webui_telemetry_observations{state="stale"} 0`,
		`hami_webui_telemetry_observations{state="known"} 1
hami_webui_telemetry_observations{state="stale"} 1`,
	)
	if err := testutil.GatherAndCompare(registry, strings.NewReader(wantAged)); err != nil {
		t.Fatalf("aged self observability mismatch:\n%v", err)
	}
}
