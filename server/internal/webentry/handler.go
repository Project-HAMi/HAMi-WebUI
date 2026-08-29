// Package webentry provides the public HTTP entry point for HAMi-WebUI.
//
// It deliberately depends only on the Go standard library. The package owns
// Web concerns (static files, SPA fallback and the same-origin API prefix), but
// it does not initialize Kubernetes, Prometheus or the HAMi application.
package webentry

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	apiPrefix       = "/api/vgpu"
	indexFile       = "index.html"
	noCache         = "no-cache"
	immutableCache  = "public, max-age=31536000, immutable"
	healthCheckBody = "OK\n"
)

var (
	viteHashPattern    = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[^.]+$`)
	webpackHashPattern = regexp.MustCompile(`\.[0-9a-fA-F]{8,}\.[^.]+$`)
	backendOnlyPaths   = []string{"/metrics", "/readyz", "/q"}
)

// HandlerConfig contains the dependencies of the Web entry HTTP handler.
// StaticFS must contain index.html at its root. APIHandler receives requests
// after the /api/vgpu prefix has been removed.
type HandlerConfig struct {
	StaticFS   fs.FS
	APIHandler http.Handler
}

// NewHandler constructs the Web entry handler and verifies that the SPA shell
// exists. APIHandler may be nil; in that case API requests return 404 rather
// than falling through to the SPA.
func NewHandler(config HandlerConfig) (http.Handler, error) {
	if config.StaticFS == nil {
		return nil, errors.New("static filesystem is required")
	}
	info, err := fs.Stat(config.StaticFS, indexFile)
	if err != nil {
		return nil, fmt.Errorf("open SPA index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("SPA index is not a regular file")
	}

	apiHandler := config.APIHandler
	if apiHandler == nil {
		apiHandler = http.HandlerFunc(writeNotFound)
	}

	return &entryHandler{
		staticFS: config.StaticFS,
		api:      http.StripPrefix(apiPrefix, apiHandler),
	}, nil
}

type entryHandler struct {
	staticFS fs.FS
	api      http.Handler
}

func (h *entryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health_check":
		h.serveHealth(w, r)
	case hasPathPrefix(r.URL.Path, "/health_check"):
		writeNotFound(w, r)
	case hasPathPrefix(r.URL.Path, apiPrefix):
		if hasInvalidPathSegment(r.URL.Path) {
			writeNotFound(w, r)
			return
		}
		backendPath := path.Clean(strings.TrimPrefix(r.URL.Path, apiPrefix))
		if !hasPathPrefix(backendPath, "/v1") {
			writeNotFound(w, r)
			return
		}
		h.api.ServeHTTP(w, r)
	case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
		writeNotFound(w, r)
	case hasAnyPathPrefix(r.URL.Path, backendOnlyPaths):
		writeNotFound(w, r)
	default:
		h.serveFrontend(w, r)
	}
}

func (h *entryHandler) serveHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(healthCheckBody)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = io.WriteString(w, healthCheckBody)
	}
}

func (h *entryHandler) serveFrontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeNotFound(w, r)
		return
	}
	if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
		name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if !fs.ValidPath(name) || h.isStaticRequest(name) {
			writeNotFound(w, r)
			return
		}
		h.serveFile(w, r, indexFile, true)
		return
	}

	name, ok := requestFileName(r.URL.Path)
	if !ok {
		writeNotFound(w, r)
		return
	}
	if name == "." {
		h.serveFile(w, r, indexFile, true)
		return
	}

	if info, err := fs.Stat(h.staticFS, name); err == nil {
		if !info.Mode().IsRegular() {
			writeNotFound(w, r)
			return
		}
		h.serveFile(w, r, name, false)
		return
	}

	if h.isStaticRequest(name) {
		writeNotFound(w, r)
		return
	}
	h.serveFile(w, r, indexFile, true)
}

func (h *entryHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, index bool) {
	servedName := name
	if info, err := fs.Stat(h.staticFS, name+".gz"); err == nil && info.Mode().IsRegular() {
		w.Header().Add("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			servedName = name + ".gz"
			w.Header().Set("Content-Encoding", "gzip")
		}
	}

	file, err := h.staticFS.Open(servedName)
	if err != nil {
		writeNotFound(w, r)
		return
	}
	defer func() {
		_ = file.Close()
	}()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeNotFound(w, r)
		return
	}

	if index || !isHashedAsset(name) {
		w.Header().Set("Cache-Control", noCache)
	} else {
		w.Header().Set("Cache-Control", immutableCache)
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	reader, err := seekableFile(file)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), reader)
}

func (h *entryHandler) isStaticRequest(name string) bool {
	if !strings.Contains(name, "/") && path.Ext(name) != "" {
		return true
	}
	first, _, _ := strings.Cut(name, "/")
	info, err := fs.Stat(h.staticFS, first)
	return err == nil && info.IsDir()
}

func requestFileName(urlPath string) (string, bool) {
	if urlPath == "/" {
		return ".", true
	}
	if !strings.HasPrefix(urlPath, "/") || strings.HasSuffix(urlPath, "/") {
		return "", false
	}
	name := strings.TrimPrefix(urlPath, "/")
	if !fs.ValidPath(name) {
		return "", false
	}
	return name, true
}

func hasPathPrefix(requestPath, prefix string) bool {
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.NotFound(w, r)
}

func hasAnyPathPrefix(requestPath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if hasPathPrefix(requestPath, prefix) {
			return true
		}
	}
	return false
}

func hasInvalidPathSegment(requestPath string) bool {
	for _, segment := range strings.Split(requestPath, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func isHashedAsset(name string) bool {
	base := path.Base(name)
	return viteHashPattern.MatchString(base) || webpackHashPattern.MatchString(base)
}

func acceptsGzip(header string) bool {
	gzipSpecified := false
	gzipAccepted := false
	wildcardAccepted := false
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		encoding := strings.TrimSpace(strings.ToLower(parts[0]))
		if encoding != "gzip" && encoding != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			key, rawValue, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || !strings.EqualFold(key, "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(rawValue), 64)
			if err != nil {
				quality = 0
			} else {
				quality = parsed
			}
		}
		accepted := quality > 0 && quality <= 1
		if encoding == "gzip" {
			gzipSpecified = true
			gzipAccepted = accepted
		} else {
			wildcardAccepted = accepted
		}
	}
	if gzipSpecified {
		return gzipAccepted
	}
	return wildcardAccepted
}

func seekableFile(file fs.File) (io.ReadSeeker, error) {
	if reader, ok := file.(io.ReadSeeker); ok {
		return reader, nil
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}
