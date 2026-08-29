package webentry

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const indexFixture = `<html><head><base data-hami-webui-base href="/"><title>HAMi</title></head><body><main>HAMi WebUI</main></body></html>`

func TestHandlerRoutes(t *testing.T) {
	t.Parallel()

	staticDir := newStaticDir(t)
	var receivedPath string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("X-Upstream", "preserved")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, `{"error":"upstream"}`)
	})
	handler := mustHandler(t, HandlerConfig{
		StaticFS:   os.DirFS(staticDir),
		APIHandler: api,
	})

	tests := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantBody    string
		wantAPIPath string
	}{
		{
			name:       "root serves index",
			method:     http.MethodGet,
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   indexFixture,
		},
		{
			name:       "deep route serves index",
			method:     http.MethodGet,
			path:       "/admin/vgpu/monitor/overview",
			wantStatus: http.StatusOK,
			wantBody:   indexFixture,
		},
		{
			name:       "deep route with trailing slash serves index",
			method:     http.MethodGet,
			path:       "/admin/vgpu/monitor/overview/",
			wantStatus: http.StatusOK,
			wantBody:   indexFixture,
		},
		{
			name:       "deep route with dotted parameter serves index",
			method:     http.MethodGet,
			path:       "/redirect/example.com",
			wantStatus: http.StatusOK,
			wantBody:   indexFixture,
		},
		{
			name:       "existing asset is served",
			method:     http.MethodGet,
			path:       "/static/app.js",
			wantStatus: http.StatusOK,
			wantBody:   "console.log('app')",
		},
		{
			name:        "api prefix is stripped",
			method:      http.MethodPost,
			path:        "/api/vgpu/v1/nodes",
			wantStatus:  http.StatusTeapot,
			wantBody:    `{"error":"upstream"}`,
			wantAPIPath: "/v1/nodes",
		},
		{
			name:       "unknown api is not a frontend route",
			method:     http.MethodGet,
			path:       "/api/unknown",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "similar api prefix is not proxied",
			method:     http.MethodGet,
			path:       "/api/vgpu-extra/v1/nodes",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "api dot segments cannot reach backend-only paths",
			method:     http.MethodGet,
			path:       "/api/vgpu/../metrics",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "api prefix cannot expose backend metrics",
			method:     http.MethodGet,
			path:       "/api/vgpu/metrics",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "api prefix cannot expose backend readiness",
			method:     http.MethodGet,
			path:       "/api/vgpu/readyz",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "api prefix cannot expose backend documentation",
			method:     http.MethodGet,
			path:       "/api/vgpu/q/openapi.yaml",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "unsupported api version is not proxied",
			method:     http.MethodGet,
			path:       "/api/vgpu/v2/private",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "missing asset with extension is not index",
			method:     http.MethodGet,
			path:       "/static/missing.js",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "missing file below asset directory is not index",
			method:     http.MethodGet,
			path:       "/static/missing",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "asset directory is not index",
			method:     http.MethodGet,
			path:       "/static/",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
		{
			name:       "post to frontend route is not index",
			method:     http.MethodPost,
			path:       "/admin/vgpu/monitor/overview",
			wantStatus: http.StatusNotFound,
			wantBody:   "404 page not found\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedPath = ""
			req := httptest.NewRequest(tt.method, tt.path, nil)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, req)

			response := recorder.Result()
			defer closeResponseBody(t, response.Body)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.wantStatus)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if receivedPath != tt.wantAPIPath {
				t.Errorf("API path = %q, want %q", receivedPath, tt.wantAPIPath)
			}
			if tt.wantAPIPath != "" && response.Header.Get("X-Upstream") != "preserved" {
				t.Errorf("upstream response header was not preserved")
			}
			if tt.wantStatus == http.StatusNotFound && response.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("generated 404 Cache-Control = %q, want no-store", response.Header.Get("Cache-Control"))
			}
		})
	}
}

func TestHandlerDoesNotExposeBackendOnlyPaths(t *testing.T) {
	t.Parallel()

	handler := mustHandler(t, HandlerConfig{StaticFS: os.DirFS(newStaticDir(t))})
	for _, requestPath := range []string{"/metrics", "/readyz", "/q", "/q/openapi.yaml"} {
		t.Run(requestPath, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestHandlerHealthCheck(t *testing.T) {
	t.Parallel()

	handler := mustHandler(t, HandlerConfig{StaticFS: os.DirFS(newStaticDir(t))})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(method, "/health_check", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			if method == http.MethodGet && recorder.Body.String() != "OK\n" {
				t.Errorf("body = %q, want %q", recorder.Body.String(), "OK\\n")
			}
		})
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/health_check", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health_check/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("trailing-slash status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHandlerCachingAndPrecompressedAssets(t *testing.T) {
	t.Parallel()

	staticDir := newStaticDir(t)
	writeFile(t, filepath.Join(staticDir, "static", "app-a1B2c3D4.js"), []byte("plain"))
	writeFile(t, filepath.Join(staticDir, "static", "app-a1B2c3D4.js.gz"), []byte("compressed"))
	handler := mustHandler(t, HandlerConfig{StaticFS: os.DirFS(staticDir)})

	tests := []struct {
		name             string
		path             string
		acceptEncoding   string
		wantBody         string
		wantCacheControl string
		wantEncoding     string
	}{
		{
			name:             "index is revalidated",
			path:             "/",
			wantBody:         indexFixture,
			wantCacheControl: "no-cache",
		},
		{
			name:             "hashed asset uses gzip and immutable cache",
			path:             "/static/app-a1B2c3D4.js",
			acceptEncoding:   "br, gzip",
			wantBody:         "compressed",
			wantCacheControl: "public, max-age=31536000, immutable",
			wantEncoding:     "gzip",
		},
		{
			name:             "gzip q zero serves the original",
			path:             "/static/app-a1B2c3D4.js",
			acceptEncoding:   "gzip;q=0, br",
			wantBody:         "plain",
			wantCacheControl: "public, max-age=31536000, immutable",
		},
		{
			name:             "explicit gzip rejection overrides wildcard",
			path:             "/static/app-a1B2c3D4.js",
			acceptEncoding:   "*;q=1, gzip;q=0",
			wantBody:         "plain",
			wantCacheControl: "public, max-age=31536000, immutable",
		},
		{
			name:             "unhashed asset is revalidated",
			path:             "/static/app.js",
			wantBody:         "console.log('app')",
			wantCacheControl: "no-cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			response := recorder.Result()
			defer closeResponseBody(t, response.Body)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if got := response.Header.Get("Cache-Control"); got != tt.wantCacheControl {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCacheControl)
			}
			if got := response.Header.Get("Content-Encoding"); got != tt.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tt.wantEncoding)
			}
			if tt.wantEncoding != "" && !strings.Contains(response.Header.Get("Vary"), "Accept-Encoding") {
				t.Errorf("Vary = %q, want Accept-Encoding", response.Header.Get("Vary"))
			}
			if strings.HasSuffix(tt.path, ".js") && !strings.Contains(response.Header.Get("Content-Type"), "javascript") {
				t.Errorf("Content-Type = %q, want JavaScript", response.Header.Get("Content-Type"))
			}
		})
	}
}

func TestHandlerHeadAssetHasHeadersAndNoBody(t *testing.T) {
	t.Parallel()

	staticDir := newStaticDir(t)
	writeFile(t, filepath.Join(staticDir, "static", "app-a1B2c3D4.js"), []byte("plain"))
	handler := mustHandler(t, HandlerConfig{StaticFS: os.DirFS(staticDir)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/static/app-a1B2c3D4.js", nil))

	response := recorder.Result()
	defer closeResponseBody(t, response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if len(body) != 0 {
		t.Errorf("HEAD body = %q, want empty", body)
	}
	if got := response.Header.Get("Cache-Control"); got != immutableCache {
		t.Errorf("Cache-Control = %q, want %q", got, immutableCache)
	}
	if response.Header.Get("Content-Length") == "" {
		t.Error("HEAD response is missing Content-Length")
	}
}

func closeResponseBody(t *testing.T, body io.Closer) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Errorf("close response body: %v", err)
	}
}

func TestNewHandlerRequiresIndex(t *testing.T) {
	t.Parallel()

	if _, err := NewHandler(HandlerConfig{StaticFS: os.DirFS(t.TempDir())}); err == nil {
		t.Fatal("NewHandler succeeded without index.html")
	}
}

func TestHandlerConfiguredBasePath(t *testing.T) {
	t.Parallel()

	var receivedPath string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mustHandler(t, HandlerConfig{
		StaticFS:   os.DirFS(newStaticDir(t)),
		APIHandler: api,
		BasePath:   "gpu-ui",
	})

	tests := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantAPIPath string
		wantIndex   bool
	}{
		{name: "base root", method: http.MethodGet, path: "/gpu-ui", wantStatus: http.StatusOK, wantIndex: true},
		{name: "base root slash", method: http.MethodGet, path: "/gpu-ui/", wantStatus: http.StatusOK, wantIndex: true},
		{name: "deep link", method: http.MethodGet, path: "/gpu-ui/admin/vgpu/monitor/overview", wantStatus: http.StatusOK, wantIndex: true},
		{name: "static asset", method: http.MethodGet, path: "/gpu-ui/static/app.js", wantStatus: http.StatusOK},
		{name: "API", method: http.MethodPost, path: "/gpu-ui/api/vgpu/v1/nodes?limit=10", wantStatus: http.StatusNoContent, wantAPIPath: "/v1/nodes?limit=10"},
		{name: "unprefixed root", method: http.MethodGet, path: "/", wantStatus: http.StatusNotFound},
		{name: "unprefixed deep link", method: http.MethodGet, path: "/admin/vgpu/monitor/overview", wantStatus: http.StatusNotFound},
		{name: "unprefixed API", method: http.MethodPost, path: "/api/vgpu/v1/nodes", wantStatus: http.StatusNotFound},
		{name: "similar prefix", method: http.MethodGet, path: "/gpu-ui-extra/admin", wantStatus: http.StatusNotFound},
		{name: "prefixed health is private", method: http.MethodGet, path: "/gpu-ui/health_check", wantStatus: http.StatusNotFound},
		{name: "root health remains available", method: http.MethodGet, path: "/health_check", wantStatus: http.StatusOK},
		{name: "dot segment cannot escape API", method: http.MethodGet, path: "/gpu-ui/api/vgpu/../metrics", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedPath = ""
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))
			response := recorder.Result()
			defer closeResponseBody(t, response.Body)
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			if response.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, tt.wantStatus)
			}
			if tt.wantIndex {
				if !strings.Contains(string(body), `<base data-hami-webui-base href="/gpu-ui/">`) {
					t.Errorf("index base was not rewritten: %q", body)
				}
				if strings.Contains(string(body), `<base data-hami-webui-base href="/">`) {
					t.Errorf("index retained root base: %q", body)
				}
			}
			if receivedPath != tt.wantAPIPath {
				t.Errorf("API request URI = %q, want %q", receivedPath, tt.wantAPIPath)
			}
		})
	}
}

func TestHandlerRecompressesRenderedIndex(t *testing.T) {
	t.Parallel()

	staticDir := newStaticDir(t)
	writeFile(t, filepath.Join(staticDir, "index.html.gz"), []byte("stale precompressed index"))
	handler := mustHandler(t, HandlerConfig{
		StaticFS: os.DirFS(staticDir),
		BasePath: "/gpu-ui/",
	})

	request := httptest.NewRequest(http.MethodGet, "/gpu-ui/", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	response := recorder.Result()
	defer closeResponseBody(t, response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", response.Header.Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("open gzip response: %v", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close gzip reader: %v", err)
		}
	}()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip response: %v", err)
	}
	if !strings.Contains(string(body), `<base data-hami-webui-base href="/gpu-ui/">`) {
		t.Errorf("compressed index base was not rewritten: %q", body)
	}
	if strings.Contains(string(body), "stale precompressed index") {
		t.Errorf("served stale index.html.gz: %q", body)
	}

	direct := httptest.NewRecorder()
	handler.ServeHTTP(direct, httptest.NewRequest(http.MethodGet, "/gpu-ui/index.html.gz", nil))
	if direct.Code != http.StatusNotFound {
		t.Errorf("direct index.html.gz status = %d, want %d", direct.Code, http.StatusNotFound)
	}
}

func TestRenderedIndexDoesNotUseStaticFileValidator(t *testing.T) {
	t.Parallel()

	handler := mustHandler(t, HandlerConfig{
		StaticFS: os.DirFS(newStaticDir(t)),
		BasePath: "/gpu-ui/",
	})
	request := httptest.NewRequest(http.MethodGet, "/gpu-ui/", nil)
	request.Header.Set("If-Modified-Since", "Wed, 31 Dec 2099 23:59:59 GMT")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Last-Modified"); got != "" {
		t.Errorf("Last-Modified = %q, want empty for runtime-rendered index", got)
	}
	if !strings.Contains(recorder.Body.String(), `<base data-hami-webui-base href="/gpu-ui/">`) {
		t.Errorf("body did not contain current runtime base: %q", recorder.Body.String())
	}
}

func TestHandlerFrameAncestorsPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sources []string
		want    string
	}{
		{name: "unset omits header", sources: nil, want: ""},
		{name: "empty denies all", sources: []string{}, want: "frame-ancestors 'none'"},
		{name: "explicit sources", sources: []string{"'self'", "https://portal.example", "http://localhost:8080"}, want: "frame-ancestors 'self' https://portal.example http://localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := mustHandler(t, HandlerConfig{
				StaticFS:       os.DirFS(newStaticDir(t)),
				FrameAncestors: tt.sources,
			})

			for _, requestPath := range []string{"/", "/admin/vgpu/monitor/overview", "/index.html"} {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
				if recorder.Code != http.StatusOK {
					t.Fatalf("%s status = %d, want %d", requestPath, recorder.Code, http.StatusOK)
				}
				if got := recorder.Header().Get("Content-Security-Policy"); got != tt.want {
					t.Errorf("%s CSP = %q, want %q", requestPath, got, tt.want)
				}
			}

			for _, requestPath := range []string{"/health_check", "/static/app.js", "/api/unknown"} {
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
				if got := recorder.Header().Get("Content-Security-Policy"); got != "" {
					t.Errorf("%s CSP = %q, want empty", requestPath, got)
				}
			}
		})
	}
}

func TestNewHandlerRejectsInvalidRuntimeConfiguration(t *testing.T) {
	t.Parallel()

	staticFS := os.DirFS(newStaticDir(t))
	invalidBasePaths := []string{
		" /gpu-ui", "/gpu ui", "/gpu-ui//nested", "/./gpu-ui", "/gpu-ui?tenant=x", "/gpu%2Dui", `/gpu\\ui`, "/health_check", "/health_check/nested",
	}
	for _, basePath := range invalidBasePaths {
		t.Run("base "+basePath, func(t *testing.T) {
			if _, err := NewHandler(HandlerConfig{StaticFS: staticFS, BasePath: basePath}); err == nil {
				t.Fatalf("NewHandler accepted base path %q", basePath)
			}
		})
	}

	invalidFrameAncestors := [][]string{
		{"self"}, {"'none'"}, {"*"}, {"https:"}, {"ftp://portal.example"}, {"https://user@portal.example"}, {"https://portal.example/"}, {"https://portal.example/path"}, {"https://portal.example?tenant=x"}, {"https://portal.example#"}, {"https://*.example"}, {"https://portal.example:"}, {"https://portal.example:99999"}, {"https://-portal.example"}, {"https://portal..example"}, {"https://[::::]"}, {"https://例子.测试"},
	}
	for _, sources := range invalidFrameAncestors {
		t.Run("frame "+strings.Join(sources, "_"), func(t *testing.T) {
			if _, err := NewHandler(HandlerConfig{StaticFS: staticFS, FrameAncestors: sources}); err == nil {
				t.Fatalf("NewHandler accepted frame ancestors %#v", sources)
			}
		})
	}
}

func TestNewHandlerRequiresSingleRuntimeBaseMarker(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		"<html><head></head></html>",
		indexFixture + indexFixture,
	} {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, indexFile), []byte(body))
		if _, err := NewHandler(HandlerConfig{StaticFS: os.DirFS(dir)}); err == nil {
			t.Fatalf("NewHandler accepted index %q", body)
		}
	}
}

func TestHandlerForwardsRequestBodyAndHeaders(t *testing.T) {
	t.Parallel()

	requestBody := []byte(`{"filters":{"name":"worker"}}`)
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read API body: %v", err)
		}
		if !bytes.Equal(body, requestBody) {
			t.Errorf("body = %q, want %q", body, requestBody)
		}
		if got := r.Header.Get("X-Hami-Probe"); got != "preserved" {
			t.Errorf("X-Hami-Probe = %q, want preserved", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mustHandler(t, HandlerConfig{
		StaticFS:   os.DirFS(newStaticDir(t)),
		APIHandler: api,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/vgpu/v1/nodes?limit=10", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hami-Probe", "preserved")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func mustHandler(t *testing.T, config HandlerConfig) http.Handler {
	t.Helper()
	handler, err := NewHandler(config)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return handler
}

func newStaticDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), []byte(indexFixture))
	writeFile(t, filepath.Join(dir, "static", "app.js"), []byte("console.log('app')"))
	return dir
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
