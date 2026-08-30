package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vgpu/internal/conf"

	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestParseOptionsUsesDocumentedDefaults(t *testing.T) {
	t.Parallel()

	config, err := parseOptions(nil, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if config.configPath != defaultConfigPath ||
		config.web.listenAddress != defaultWebListenAddress ||
		config.web.staticDir != defaultStaticDir ||
		config.web.basePath != defaultBasePath ||
		config.web.frameAncestors != nil ||
		config.healthcheck ||
		config.healthcheckURL != defaultHealthcheckURL {
		t.Fatalf("defaults = %+v", config)
	}
}

func TestParseOptionsFlagsOverrideEnvironment(t *testing.T) {
	t.Parallel()

	environment := map[string]string{
		"HAMI_WEBUI_LISTEN_ADDRESS":       ":3100",
		"HAMI_WEBUI_STATIC_DIR":           "/env/public",
		"HAMI_WEBUI_BASE_PATH":            "/env-ui/",
		"HAMI_WEBUI_FRAME_ANCESTORS_JSON": `[]`,
		"HAMI_WEBUI_HEALTHCHECK_URL":      "http://health-from-env/health_check",
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}
	config, err := parseOptions([]string{
		"--conf=/flag/config.yaml",
		"--listen-address=:3200",
		"--static-dir=/flag/public",
		"--base-path=/flag-ui/",
		`--frame-ancestors-json=["'self'"]`,
		"--healthcheck",
		"--healthcheck-url=http://health-from-flag/health_check",
	}, lookup)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if config.configPath != "/flag/config.yaml" ||
		config.web.listenAddress != ":3200" ||
		config.web.staticDir != "/flag/public" ||
		config.web.basePath != "/flag-ui/" ||
		len(config.web.frameAncestors) != 1 || config.web.frameAncestors[0] != "'self'" {
		t.Fatalf("flags did not override environment: %+v", config)
	}
	if !config.healthcheck || config.healthcheckURL != "http://health-from-flag/health_check" {
		t.Fatalf("health-check flags did not override environment: %+v", config)
	}
}

func TestPerformHealthcheckRequiresSuccessStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "success", status: http.StatusNoContent},
		{name: "redirect", status: http.StatusFound, wantErr: true},
		{name: "client error", status: http.StatusNotFound, wantErr: true},
		{name: "server error", status: http.StatusServiceUnavailable, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			if err := performHealthcheck(server.URL); (err != nil) != tt.wantErr {
				t.Fatalf("performHealthcheck error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHealthcheckModeDoesNotInitializeApplication(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	lookup := func(key string) (string, bool) {
		if key == "HAMI_WEBUI_HEALTHCHECK_URL" {
			return server.URL, true
		}
		return "", false
	}
	if err := run([]string{"--healthcheck", "--conf=/does/not/exist"}, lookup); err != nil {
		t.Fatalf("health check initialized application dependencies: %v", err)
	}
}

func TestHealthcheckRejectsCredentialsWithoutExposingThem(t *testing.T) {
	t.Parallel()

	const secret = "do-not-print-this"
	err := performHealthcheck("http://user:" + secret + "@backend/health_check")
	if err == nil {
		t.Fatal("performHealthcheck accepted credentials")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("health-check error exposed URL credentials")
	}
}

func TestParseOptionsRejectsInvalidFrameAncestors(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "{}", `"'self'"`, `["'self'",1]`, "[] trailing"} {
		t.Run(raw, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				if key == "HAMI_WEBUI_FRAME_ANCESTORS_JSON" {
					return raw, true
				}
				return "", false
			}
			if _, err := parseOptions(nil, lookup); err == nil {
				t.Fatalf("parseOptions accepted frame ancestors %q", raw)
			}
		})
	}
}

func TestWebHandlerInvokesKratosAPIDirectly(t *testing.T) {
	t.Parallel()

	staticDir := newStaticDir(t)
	api := kratoshttp.NewServer()
	api.Handle("/v1/contract-probe", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := transport.FromServerContext(r.Context()); !ok {
			t.Error("request is missing Kratos server transport context")
		}
		if r.URL.RequestURI() != "/v1/contract-probe?limit=10" {
			t.Errorf("request URI = %q", r.URL.RequestURI())
		}
		if r.Header.Get("X-Hami-Probe") != "preserved" {
			t.Errorf("request header was not preserved")
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if string(body) != `{"name":"worker"}` {
			t.Errorf("request body = %q", body)
		}
		w.Header().Set("X-Hami-Response", "preserved")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))

	handler, err := newWebHandler(webConfig{
		staticDir:      staticDir,
		basePath:       "/gpu-ui/",
		frameAncestors: []string{"'self'"},
	}, api)
	if err != nil {
		t.Fatalf("newWebHandler: %v", err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/gpu-ui/api/vgpu/v1/contract-probe?limit=10",
		bytes.NewBufferString(`{"name":"worker"}`),
	)
	request.Header.Set("X-Hami-Probe", "preserved")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || response.Body.String() != `{"ok":true}` {
		t.Fatalf("response = {%d %q}, want {202 %q}", response.Code, response.Body.String(), `{"ok":true}`)
	}
	if response.Header().Get("X-Hami-Response") != "preserved" {
		t.Error("response header was not preserved")
	}
}

func TestWebHandlerKeepsBackendOnlyRoutesPrivate(t *testing.T) {
	t.Parallel()

	apiCalled := false
	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusOK)
	})
	handler, err := newWebHandler(webConfig{
		staticDir: newStaticDir(t),
		basePath:  "/gpu-ui/",
	}, api)
	if err != nil {
		t.Fatalf("newWebHandler: %v", err)
	}
	for _, requestPath := range []string{
		"/gpu-ui/metrics",
		"/gpu-ui/readyz",
		"/gpu-ui/q/openapi.yaml",
		"/gpu-ui/api/vgpu/v2/private",
		"/gpu-ui/api/vgpu/../metrics",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, requestPath, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", requestPath, response.Code)
		}
	}
	if apiCalled {
		t.Fatal("backend handler was called for a private or unsupported route")
	}
}

func TestBackendRequestTimeoutUsesConfigOrSafeDefault(t *testing.T) {
	t.Parallel()

	if got := backendRequestTimeout(nil); got != defaultRequestTimeout {
		t.Fatalf("nil config timeout = %s, want %s", got, defaultRequestTimeout)
	}
	configured := 17 * time.Second
	bootstrap := &conf.Bootstrap{
		Server: &conf.Server{
			Http: &conf.Server_HTTP{Timeout: durationpb.New(configured)},
		},
	}
	if got := backendRequestTimeout(bootstrap); got != configured {
		t.Fatalf("configured timeout = %s, want %s", got, configured)
	}
}

func newStaticDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	index := `<html><head><base data-hami-webui-base href="/"></head><body>HAMi</body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(index), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	return dir
}

func TestWebHandlerPreservesKratosJSONNotFound(t *testing.T) {
	t.Parallel()

	api := kratoshttp.NewServer(
		kratoshttp.NotFoundHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"reason":"ROUTE_NOT_FOUND"}`)
		})),
	)
	handler, err := newWebHandler(webConfig{staticDir: newStaticDir(t)}, api)
	if err != nil {
		t.Fatalf("newWebHandler: %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/vgpu/v1/missing", nil))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "ROUTE_NOT_FOUND") {
		t.Fatalf("response = {%d %q}, want Kratos JSON 404", response.Code, response.Body.String())
	}
}
