package server

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	obsv1 "vgpu/api/observability/v1"
	"vgpu/internal/conf"
	"vgpu/internal/resourceview"
	"vgpu/internal/service"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestHTTPFallbackResponses(t *testing.T) {
	defaultServeMux := nethttp.DefaultServeMux
	nethttp.DefaultServeMux = nethttp.NewServeMux()
	nethttp.DefaultServeMux.HandleFunc("/default-mux-probe", func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusTeapot)
	})
	t.Cleanup(func() {
		nethttp.DefaultServeMux = defaultServeMux
	})

	handler := newTestHTTPHandler()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantReason string
	}{
		{
			name:       "global default mux endpoint is hidden",
			method:     nethttp.MethodGet,
			path:       "/default-mux-probe",
			wantStatus: nethttp.StatusNotFound,
			wantReason: httpReasonRouteNotFound,
		},
		{
			name:       "unknown route",
			method:     nethttp.MethodGet,
			path:       "/not-a-route",
			wantStatus: nethttp.StatusNotFound,
			wantReason: httpReasonRouteNotFound,
		},
		{
			name:       "business API is not exposed on metrics listener",
			method:     nethttp.MethodGet,
			path:       "/api/v1/overview/inventory",
			wantStatus: nethttp.StatusNotFound,
			wantReason: httpReasonRouteNotFound,
		},
		{
			name:       "legacy v1 route is removed",
			method:     nethttp.MethodPost,
			path:       "/v1/nodes",
			wantStatus: nethttp.StatusNotFound,
			wantReason: httpReasonRouteNotFound,
		},
		{
			name:       "openapi docs route is removed",
			method:     nethttp.MethodGet,
			path:       "/q/openapi.yaml",
			wantStatus: nethttp.StatusNotFound,
			wantReason: httpReasonRouteNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}

			var body struct {
				Code   int32  `json:"code"`
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v; body = %s", err, response.Body.String())
			}
			if body.Code != int32(test.wantStatus) || body.Reason != test.wantReason {
				t.Fatalf("error = {%d %q}, want {%d %q}", body.Code, body.Reason, test.wantStatus, test.wantReason)
			}
		})
	}
}

func TestRegisteredHTTPHandlerTakesPrecedenceOverFallback(t *testing.T) {
	handler := newTestHTTPHandler()
	for _, route := range []string{"/readyz", "/metrics"} {
		request := httptest.NewRequest(nethttp.MethodGet, route, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != nethttp.StatusOK {
			t.Fatalf("%s status = %d, want 200", route, response.Code)
		}
		if route == "/readyz" && response.Body.String() != "ok" {
			t.Fatalf("readyz response = {%d %q}, want {200 %q}", response.Code, response.Body.String(), "ok")
		}
	}
}

func newTestHTTPHandler() nethttp.Handler {
	srv := NewHTTPServer(
		&conf.Bootstrap{Server: &conf.Server{Http: &conf.Server_HTTP{}}},
		prometheus.NewRegistry(),
		nil,
	)
	return srv.Handler
}

type deadlineCapturingSource struct {
	remaining time.Duration
	ok        bool
}

func (s *deadlineCapturingSource) Snapshot(ctx context.Context) (resourceview.Snapshot, error) {
	if deadline, ok := ctx.Deadline(); ok {
		s.ok = true
		s.remaining = time.Until(deadline)
	}
	return resourceview.Snapshot{
		NodeCoverage:   &obsv1.Coverage{},
		DeviceCoverage: &obsv1.Coverage{},
	}, nil
}

func TestAPIHandlerHonorsConfiguredTimeout(t *testing.T) {
	src := &deadlineCapturingSource{}
	overview := service.NewOverviewService(resourceview.WithObservations(src, nil))
	handler := NewAPIHandler(
		&conf.Bootstrap{Server: &conf.Server{Http: &conf.Server_HTTP{Timeout: durationpb.New(5 * time.Second)}}},
		nil, nil, nil, nil, nil, nil, overview, nil, nil,
	)
	req := httptest.NewRequest(nethttp.MethodGet, "/api/v1/overview/inventory", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != nethttp.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !src.ok || src.remaining < 3*time.Second || src.remaining > 6*time.Second {
		t.Fatalf("configured deadline missing or wrong: ok=%t remaining=%s (want ~5s, not Kratos 1s default)", src.ok, src.remaining)
	}
}
