// Package webentry provides the public HTTP entry point for HAMi-WebUI.
//
// It deliberately depends only on the Go standard library. The package owns
// Web concerns (static files, SPA fallback and the same-origin API prefix), but
// it does not initialize Kubernetes, Prometheus or the HAMi application.
package webentry

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	apiPrefix       = "/api/vgpu"
	indexFile       = "index.html"
	noCache         = "no-cache"
	immutableCache  = "public, max-age=31536000, immutable"
	healthCheckBody = "OK\n"
	rootBasePath    = "/"
)

var (
	viteHashPattern    = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[^.]+$`)
	webpackHashPattern = regexp.MustCompile(`\.[0-9a-fA-F]{8,}\.[^.]+$`)
	runtimeBaseElement = regexp.MustCompile(`<base[[:space:]]+data-hami-webui-base[[:space:]]+href="/"[[:space:]]*/?>`)
	basePathSegment    = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
	backendOnlyPaths   = []string{"/metrics", "/readyz", "/q"}
)

// HandlerConfig contains the dependencies of the Web entry HTTP handler.
// StaticFS must contain index.html at its root. APIHandler receives requests
// after the /api/vgpu prefix has been removed.
type HandlerConfig struct {
	StaticFS   fs.FS
	APIHandler http.Handler
	// BasePath is the operator-controlled external URL prefix. Empty means root.
	BasePath string
	// FrameAncestors is tri-state: nil omits CSP framing policy, an empty slice
	// denies all framing, and a non-empty slice is an explicit allowlist.
	FrameAncestors []string
}

// NewHandler constructs the Web entry handler and verifies that the SPA shell
// exists. APIHandler may be nil; in that case API requests return 404 rather
// than falling through to the SPA.
func NewHandler(config HandlerConfig) (http.Handler, error) {
	if config.StaticFS == nil {
		return nil, errors.New("static filesystem is required")
	}
	basePath, err := normalizeBasePath(config.BasePath)
	if err != nil {
		return nil, fmt.Errorf("base path: %w", err)
	}
	frameAncestors, err := frameAncestorsPolicy(config.FrameAncestors)
	if err != nil {
		return nil, fmt.Errorf("frame ancestors: %w", err)
	}
	info, err := fs.Stat(config.StaticFS, indexFile)
	if err != nil {
		return nil, fmt.Errorf("open SPA index: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("SPA index is not a regular file")
	}
	indexSource, err := fs.ReadFile(config.StaticFS, indexFile)
	if err != nil {
		return nil, fmt.Errorf("read SPA index: %w", err)
	}
	indexBody, err := renderIndexBase(indexSource, basePath)
	if err != nil {
		return nil, err
	}
	compressedIndex, err := gzipBytes(indexBody)
	if err != nil {
		return nil, fmt.Errorf("compress SPA index: %w", err)
	}

	apiHandler := config.APIHandler
	if apiHandler == nil {
		apiHandler = http.HandlerFunc(writeNotFound)
	}

	return &entryHandler{
		staticFS:        config.StaticFS,
		api:             http.StripPrefix(apiPrefix, apiHandler),
		basePath:        basePath,
		indexBody:       indexBody,
		compressedIndex: compressedIndex,
		frameAncestors:  frameAncestors,
	}, nil
}

type entryHandler struct {
	staticFS        fs.FS
	api             http.Handler
	basePath        string
	indexBody       []byte
	compressedIndex []byte
	frameAncestors  string
}

func (h *entryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health_check":
		h.serveHealth(w, r)
	case hasPathPrefix(r.URL.Path, "/health_check"):
		writeNotFound(w, r)
	default:
		applicationRequest, ok := h.applicationRequest(r)
		if !ok {
			writeNotFound(w, r)
			return
		}
		h.serveApplication(w, applicationRequest)
	}
}

func (h *entryHandler) applicationRequest(r *http.Request) (*http.Request, bool) {
	if h.basePath == rootBasePath {
		return r, true
	}

	basePrefix := strings.TrimSuffix(h.basePath, "/")
	if !hasPathPrefix(r.URL.Path, basePrefix) {
		return nil, false
	}
	requestPath := strings.TrimPrefix(r.URL.Path, basePrefix)
	if requestPath == "" {
		requestPath = "/"
	}
	clone := r.Clone(r.Context())
	clonedURL := *r.URL
	clonedURL.Path = requestPath
	// Routing uses URL.Path. Clear RawPath after removing the operator-controlled
	// prefix so an encoded spelling cannot retain an unstripped external path.
	clonedURL.RawPath = ""
	clone.URL = &clonedURL
	return clone, true
}

func (h *entryHandler) serveApplication(w http.ResponseWriter, r *http.Request) {
	switch {
	case hasPathPrefix(r.URL.Path, "/health_check"):
		// The liveness endpoint deliberately remains outside a configured base path.
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
		h.serveIndex(w, r)
		return
	}

	name, ok := requestFileName(r.URL.Path)
	if !ok {
		writeNotFound(w, r)
		return
	}
	if name == "." {
		h.serveIndex(w, r)
		return
	}
	if name == indexFile {
		h.serveIndex(w, r)
		return
	}
	if name == indexFile+".gz" {
		writeNotFound(w, r)
		return
	}

	if info, err := fs.Stat(h.staticFS, name); err == nil {
		if !info.Mode().IsRegular() {
			writeNotFound(w, r)
			return
		}
		h.serveFile(w, r, name)
		return
	}

	if h.isStaticRequest(name) {
		writeNotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func (h *entryHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	body := h.indexBody
	w.Header().Add("Vary", "Accept-Encoding")
	if acceptsGzip(r.Header.Get("Accept-Encoding")) {
		body = h.compressedIndex
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.Header().Set("Cache-Control", noCache)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if h.frameAncestors != "" {
		w.Header().Set("Content-Security-Policy", h.frameAncestors)
	}
	// Runtime configuration changes the representation without changing the
	// image file's mtime. A file-derived Last-Modified validator could therefore
	// produce a stale 304 after a base-path or framing-policy update.
	http.ServeContent(w, r, indexFile, time.Time{}, bytes.NewReader(body))
}

func (h *entryHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
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

	if !isHashedAsset(name) {
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

func normalizeBasePath(raw string) (string, error) {
	if raw == "" || raw == rootBasePath {
		return rootBasePath, nil
	}
	if strings.TrimSpace(raw) != raw {
		return "", errors.New("must not contain surrounding whitespace")
	}
	if strings.ContainsAny(raw, `?#%\\`) {
		return "", errors.New("must be an unescaped URL path without query or fragment")
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	if !strings.HasSuffix(raw, "/") {
		raw += "/"
	}
	for _, segment := range strings.Split(strings.Trim(raw, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." || !basePathSegment.MatchString(segment) {
			return "", errors.New("contains an invalid path segment")
		}
	}
	basePrefix := strings.TrimSuffix(raw, "/")
	if hasPathPrefix(basePrefix, "/health_check") {
		return "", errors.New("conflicts with the health-check endpoint")
	}
	return raw, nil
}

func renderIndexBase(source []byte, basePath string) ([]byte, error) {
	if matches := runtimeBaseElement.FindAllIndex(source, -1); len(matches) != 1 {
		return nil, errors.New("SPA index must contain exactly one runtime base marker")
	}
	replacement := []byte(`<base data-hami-webui-base href="` + basePath + `">`)
	return runtimeBaseElement.ReplaceAll(source, replacement), nil
}

func gzipBytes(source []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(source); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return compressed.Bytes(), nil
}

func frameAncestorsPolicy(sources []string) (string, error) {
	if sources == nil {
		return "", nil
	}
	if len(sources) == 0 {
		return "frame-ancestors 'none'", nil
	}
	for _, source := range sources {
		if source == "'self'" {
			continue
		}
		origin, err := url.Parse(source)
		if err != nil || origin.Scheme == "" || origin.Host == "" {
			return "", fmt.Errorf("%q is not an absolute HTTP origin", source)
		}
		if (origin.Scheme != "http" && origin.Scheme != "https") || origin.User != nil || origin.Path != "" || origin.RawPath != "" || origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.Opaque != "" {
			return "", fmt.Errorf("%q is not an exact HTTP origin", source)
		}
		if source != origin.Scheme+"://"+origin.Host || !validOriginHost(origin) {
			return "", fmt.Errorf("%q is not an exact HTTP origin", source)
		}
	}
	return "frame-ancestors " + strings.Join(sources, " "), nil
}

func validOriginHost(origin *url.URL) bool {
	host := origin.Hostname()
	if host == "" || strings.HasSuffix(origin.Host, ":") {
		return false
	}
	if port := origin.Port(); port != "" {
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value > 65535 {
			return false
		}
	}
	if strings.Contains(host, ":") {
		_, err := netip.ParseAddr(host)
		return err == nil
	}

	host = strings.TrimSuffix(host, ".")
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !isASCIILetterOrDigit(label[0]) || !isASCIILetterOrDigit(label[len(label)-1]) {
			return false
		}
		for index := 1; index < len(label)-1; index++ {
			if !isASCIILetterOrDigit(label[index]) && label[index] != '-' {
				return false
			}
		}
	}
	return true
}

func isASCIILetterOrDigit(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
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
