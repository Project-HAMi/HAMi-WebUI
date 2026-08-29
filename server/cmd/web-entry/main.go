// Command web-entry serves the HAMi-WebUI SPA and same-origin API endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vgpu/internal/webentry"
)

const (
	defaultListenAddress  = ":3000"
	defaultBackendURL     = "http://127.0.0.1:8000"
	defaultStaticDir      = "/apps/public"
	defaultProxyTimeout   = "65s"
	defaultHealthcheckURL = "http://127.0.0.1:3000/health_check"
	healthcheckTimeout    = 3 * time.Second
	shutdownTimeout       = 10 * time.Second
)

type options struct {
	listenAddress  string
	backendURL     string
	staticDir      string
	proxyTimeout   string
	healthcheck    bool
	healthcheckURL string
}

func main() {
	if err := run(os.Args[1:], os.LookupEnv); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, lookupEnv func(string) (string, bool)) error {
	config, err := parseOptions(args, lookupEnv)
	if err != nil {
		return errors.New("invalid command-line options")
	}
	if config.healthcheck {
		return performHealthcheck(config.healthcheckURL)
	}

	proxyTimeout, err := time.ParseDuration(config.proxyTimeout)
	if err != nil || proxyTimeout <= 0 {
		return errors.New("configuration error: proxy timeout must be a positive duration")
	}
	if config.listenAddress == "" {
		return errors.New("configuration error: listen address must not be empty")
	}
	if config.staticDir == "" {
		return errors.New("configuration error: static directory must not be empty")
	}
	staticInfo, err := os.Stat(config.staticDir)
	if err != nil || !staticInfo.IsDir() {
		return errors.New("configuration error: static directory is unavailable")
	}

	target, err := parseHTTPURL(config.backendURL)
	if err != nil {
		return errors.New("configuration error: backend URL must be an absolute HTTP URL without credentials")
	}
	logger := log.New(os.Stdout, "web-entry: ", log.LstdFlags|log.LUTC)
	proxy, err := webentry.NewReverseProxy(webentry.ReverseProxyConfig{
		Target:  target,
		Timeout: proxyTimeout,
		ReportError: func(failure webentry.ProxyFailure) {
			logger.Printf("backend proxy failure status=%d class=%s error=%v", failure.Status, failure.Class, failure.Err)
		},
	})
	if err != nil {
		return errors.New("configuration error: backend proxy is invalid")
	}
	handler, err := webentry.NewHandler(webentry.HandlerConfig{
		StaticFS:   os.DirFS(config.staticDir),
		APIHandler: proxy,
	})
	if err != nil {
		return errors.New("configuration error: static directory does not contain a usable SPA")
	}

	listener, err := net.Listen("tcp", config.listenAddress)
	if err != nil {
		return errors.New("cannot open Web-entry listener")
	}
	server, err := webentry.NewHTTPServer(webentry.HTTPServerConfig{
		Address:      config.listenAddress,
		Handler:      handler,
		ProxyTimeout: proxyTimeout,
	})
	if err != nil {
		_ = listener.Close()
		return errors.New("configuration error: HTTP server settings are invalid")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger.Printf("listening address=%s", listener.Addr())
	if err := webentry.Serve(ctx, server, listener, shutdownTimeout); err != nil {
		return fmt.Errorf("web entry stopped: %w", err)
	}
	return nil
}

func parseOptions(args []string, lookupEnv func(string) (string, bool)) (options, error) {
	config := options{}
	flags := flag.NewFlagSet("web-entry", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.listenAddress, "listen-address", envOrDefault(lookupEnv, "HAMI_WEBUI_LISTEN_ADDRESS", defaultListenAddress), "HTTP listen address")
	flags.StringVar(&config.backendURL, "backend-url", envOrDefault(lookupEnv, "HAMI_WEBUI_BACKEND_URL", defaultBackendURL), "backend base URL")
	flags.StringVar(&config.staticDir, "static-dir", envOrDefault(lookupEnv, "HAMI_WEBUI_STATIC_DIR", defaultStaticDir), "SPA static directory")
	flags.StringVar(&config.proxyTimeout, "proxy-timeout", envOrDefault(lookupEnv, "HAMI_WEBUI_PROXY_TIMEOUT", defaultProxyTimeout), "backend request timeout")
	flags.BoolVar(&config.healthcheck, "healthcheck", false, "check the running Web entry and exit")
	flags.StringVar(&config.healthcheckURL, "healthcheck-url", envOrDefault(lookupEnv, "HAMI_WEBUI_HEALTHCHECK_URL", defaultHealthcheckURL), "health-check URL")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	return config, nil
}

func envOrDefault(lookupEnv func(string) (string, bool), key, fallback string) string {
	if value, ok := lookupEnv(key); ok {
		return value
	}
	return fallback
}

func parseHTTPURL(raw string) (*url.URL, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.Fragment != "" {
		return nil, errors.New("invalid HTTP URL")
	}
	return target, nil
}

func performHealthcheck(rawURL string) error {
	target, err := parseHTTPURL(rawURL)
	if err != nil {
		return errors.New("health check configuration is invalid")
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.String(), nil)
	if err != nil {
		return errors.New("health check configuration is invalid")
	}
	client := &http.Client{
		Timeout: healthcheckTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("web-entry health check failed")
	}
	defer func() {
		_ = response.Body.Close()
	}()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return errors.New("web-entry health check returned a non-success status")
	}
	return nil
}
