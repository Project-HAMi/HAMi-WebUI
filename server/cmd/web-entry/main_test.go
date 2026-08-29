package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOptionsFlagsOverrideEnvironment(t *testing.T) {
	t.Parallel()

	environment := map[string]string{
		"HAMI_WEBUI_LISTEN_ADDRESS":  ":3100",
		"HAMI_WEBUI_BACKEND_URL":     "http://backend-from-env:8000",
		"HAMI_WEBUI_STATIC_DIR":      "/env/public",
		"HAMI_WEBUI_PROXY_TIMEOUT":   "70s",
		"HAMI_WEBUI_HEALTHCHECK_URL": "http://health-from-env/health_check",
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}

	config, err := parseOptions([]string{
		"--listen-address=:3200",
		"--backend-url=http://backend-from-flag:8000",
		"--static-dir=/flag/public",
		"--proxy-timeout=80s",
		"--healthcheck-url=http://health-from-flag/health_check",
	}, lookup)
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}

	if config.listenAddress != ":3200" ||
		config.backendURL != "http://backend-from-flag:8000" ||
		config.staticDir != "/flag/public" ||
		config.proxyTimeout != "80s" ||
		config.healthcheckURL != "http://health-from-flag/health_check" {
		t.Fatalf("flags did not override environment: %+v", config)
	}
}

func TestParseOptionsUsesDocumentedDefaults(t *testing.T) {
	t.Parallel()

	config, err := parseOptions(nil, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	if config.listenAddress != defaultListenAddress ||
		config.backendURL != defaultBackendURL ||
		config.staticDir != defaultStaticDir ||
		config.proxyTimeout != defaultProxyTimeout ||
		config.healthcheckURL != defaultHealthcheckURL {
		t.Fatalf("defaults = %+v", config)
	}
}

func TestPerformHealthcheckRequiresTwoHundredStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{name: "success", status: http.StatusNoContent},
		{name: "redirect", status: http.StatusFound, wantErr: true},
		{name: "server error", status: http.StatusServiceUnavailable, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			err := performHealthcheck(server.URL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("performHealthcheck error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHealthcheckModeDoesNotInitializeServeConfiguration(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	environment := map[string]string{
		"HAMI_WEBUI_BACKEND_URL":     "://invalid",
		"HAMI_WEBUI_STATIC_DIR":      "/does/not/exist",
		"HAMI_WEBUI_PROXY_TIMEOUT":   "not-a-duration",
		"HAMI_WEBUI_HEALTHCHECK_URL": server.URL,
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}

	if err := run([]string{"--healthcheck"}, lookup); err != nil {
		t.Fatalf("healthcheck initialized unrelated serve configuration: %v", err)
	}
}

func TestParseHTTPURLDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()

	const secret = "do-not-print-this"
	_, err := parseHTTPURL("http://user:" + secret + "@backend:8000")
	if err == nil {
		t.Fatal("parseHTTPURL accepted credentials")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("configuration error exposed URL credentials")
	}
}

func TestRunDoesNotExposeBackendCredentials(t *testing.T) {
	t.Parallel()

	staticDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticDir, "index.html"), []byte("<main></main>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	const secret = "do-not-print-this"
	environment := map[string]string{
		"HAMI_WEBUI_BACKEND_URL": "http://user:" + secret + "@backend:8000",
		"HAMI_WEBUI_STATIC_DIR":  staticDir,
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}

	err := run(nil, lookup)
	if err == nil {
		t.Fatal("run accepted backend credentials")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("startup error exposed URL credentials")
	}
}
