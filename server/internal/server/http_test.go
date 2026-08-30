package server

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vgpu/internal/conf"
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
			name:       "known route with unsupported method",
			method:     nethttp.MethodGet,
			path:       "/v1/nodes",
			wantStatus: nethttp.StatusMethodNotAllowed,
			wantReason: httpReasonMethodNotAllowed,
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
	request := httptest.NewRequest(nethttp.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	newTestHTTPHandler().ServeHTTP(response, request)

	if response.Code != nethttp.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("readyz response = {%d %q}, want {200 %q}", response.Code, response.Body.String(), "ok")
	}
}

func newTestHTTPHandler() nethttp.Handler {
	srv := NewHTTPServer(
		&conf.Bootstrap{Server: &conf.Server{Http: &conf.Server_HTTP{}}},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return srv.Handler
}
