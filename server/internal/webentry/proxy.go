package webentry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// ReverseProxyConfig defines an internal backend proxy. ReportError is optional
// and is called for transport failures without receiving the browser request,
// preventing request URLs, queries and bodies from entering logs by accident.
type ReverseProxyConfig struct {
	Target      *url.URL
	Timeout     time.Duration
	ReportError func(ProxyFailure)
}

// ProxyFailure is the bounded diagnostic passed to ReportError.
type ProxyFailure struct {
	Status int
	Class  string
	Err    error
}

// NewReverseProxy creates a reverse proxy with a request-wide upstream
// deadline. Upstream HTTP status codes and response bodies pass through
// unchanged; transport failures are represented as 502 or 504 JSON responses.
func NewReverseProxy(config ReverseProxyConfig) (http.Handler, error) {
	target := config.Target
	timeout := config.Timeout
	if target == nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("backend URL must be an absolute HTTP URL")
	}
	if target.User != nil {
		return nil, errors.New("backend URL must not contain credentials")
	}
	if target.Fragment != "" {
		return nil, errors.New("backend URL must not contain a fragment")
	}
	if timeout <= 0 {
		return nil, errors.New("proxy timeout must be positive")
	}

	target = cloneURL(target)
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(request *http.Request) {
		director(request)
		request.Host = target.Host
	}
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		if isTimeoutError(err) {
			reportProxyFailure(config.ReportError, http.StatusGatewayTimeout, "timeout", err)
			writeProxyError(w, http.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "backend request timed out")
			return
		}
		reportProxyFailure(config.ReportError, http.StatusBadGateway, "transport", err)
		writeProxyError(w, http.StatusBadGateway, "BAD_GATEWAY", "backend unavailable")
	}
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = transport.Clone()
		// The backend is an explicitly configured internal endpoint. Do not let
		// ambient HTTP_PROXY settings reroute same-Pod API traffic.
		transport.Proxy = nil
		transport.DialContext = (&net.Dialer{
			Timeout:   minDuration(timeout, 10*time.Second),
			KeepAlive: 30 * time.Second,
		}).DialContext
		transport.ResponseHeaderTimeout = timeout
		transport.TLSHandshakeTimeout = minDuration(timeout, 10*time.Second)
		transport.MaxIdleConnsPerHost = 32
		proxy.Transport = transport
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(ctx))
	}), nil
}

func reportProxyFailure(report func(ProxyFailure), status int, class string, err error) {
	if report != nil {
		report(ProxyFailure{Status: status, Class: class, Err: err})
	}
}

type proxyError struct {
	Code    int    `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func writeProxyError(w http.ResponseWriter, status int, reason, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(proxyError{Code: status, Reason: reason, Message: message})
}

func isTimeoutError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netError net.Error
	return errors.As(err, &netError) && netError.Timeout()
}

func cloneURL(target *url.URL) *url.URL {
	clone := *target
	return &clone
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
