package webentry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReverseProxyPreservesUpstreamResponse(t *testing.T) {
	t.Parallel()

	var wantHost string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/v1/nodes")
		}
		if r.Host != wantHost {
			t.Errorf("Host = %q, want %q", r.Host, wantHost)
		}
		w.Header().Set("X-Upstream", "preserved")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid request"}`)
	}))
	t.Cleanup(backend.Close)
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}
	wantHost = backendURL.Host

	proxy := mustReverseProxy(t, backend.URL, time.Second, nil)
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/nodes", nil))

	response := recorder.Result()
	defer closeResponseBody(t, response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
	if response.Header.Get("X-Upstream") != "preserved" {
		t.Errorf("upstream response header was not preserved")
	}
	if string(body) != `{"error":"invalid request"}` {
		t.Errorf("body = %q", body)
	}
}

func TestReverseProxyMapsConnectionFailureToBadGateway(t *testing.T) {
	t.Parallel()

	// TCP port zero cannot have a listening service, making the connection
	// failure deterministic without a close-and-rebind race in this test.
	var failure ProxyFailure
	proxy := mustReverseProxy(t, "http://127.0.0.1:0", time.Second, func(got ProxyFailure) {
		failure = got
	})
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/nodes", nil))

	assertProxyError(t, recorder.Result(), http.StatusBadGateway, "BAD_GATEWAY")
	if failure.Status != http.StatusBadGateway || failure.Class != "transport" || failure.Err == nil {
		t.Errorf("reported failure = %+v", failure)
	}
}

func TestReverseProxyMapsUpstreamTimeoutToGatewayTimeout(t *testing.T) {
	t.Parallel()

	requestCancelled := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(requestCancelled)
	}))
	t.Cleanup(backend.Close)

	var failure ProxyFailure
	proxy := mustReverseProxy(t, backend.URL, 50*time.Millisecond, func(got ProxyFailure) {
		failure = got
	})
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/slow", nil))

	assertProxyError(t, recorder.Result(), http.StatusGatewayTimeout, "GATEWAY_TIMEOUT")
	if failure.Status != http.StatusGatewayTimeout || failure.Class != "timeout" || failure.Err == nil {
		t.Errorf("reported failure = %+v", failure)
	}
	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream request context was not cancelled")
	}
}

func TestNewReverseProxyRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  *url.URL
		timeout time.Duration
	}{
		{name: "nil target", timeout: time.Second},
		{name: "missing scheme", target: &url.URL{Host: "backend:8000"}, timeout: time.Second},
		{name: "unsupported scheme", target: &url.URL{Scheme: "file", Path: "/tmp/backend"}, timeout: time.Second},
		{name: "credentials", target: &url.URL{Scheme: "http", Host: "backend:8000", User: url.UserPassword("user", "secret")}, timeout: time.Second},
		{name: "zero timeout", target: &url.URL{Scheme: "http", Host: "backend:8000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewReverseProxy(ReverseProxyConfig{Target: tt.target, Timeout: tt.timeout}); err == nil {
				t.Fatal("NewReverseProxy succeeded, want error")
			}
		})
	}
}

func mustReverseProxy(t *testing.T, rawURL string, timeout time.Duration, report func(ProxyFailure)) http.Handler {
	t.Helper()
	target, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse backend URL: %v", err)
	}
	proxy, err := NewReverseProxy(ReverseProxyConfig{
		Target:      target,
		Timeout:     timeout,
		ReportError: report,
	})
	if err != nil {
		t.Fatalf("NewReverseProxy: %v", err)
	}
	return proxy
}

func assertProxyError(t *testing.T, response *http.Response, wantStatus int, wantReason string) {
	t.Helper()
	defer closeResponseBody(t, response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != wantStatus {
		t.Errorf("status = %d, want %d", response.StatusCode, wantStatus)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", contentType)
	}
	if !strings.Contains(string(body), `"reason":"`+wantReason+`"`) {
		t.Errorf("body = %q, want reason %q", body, wantReason)
	}
}

func TestTimeoutErrorClassification(t *testing.T) {
	t.Parallel()

	if !isTimeoutError(context.DeadlineExceeded) {
		t.Fatal("context deadline should be classified as a timeout")
	}
}
