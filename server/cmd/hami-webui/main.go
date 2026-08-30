// Command hami-webui runs the HAMi-WebUI API, metrics collector and public Web
// entry in one process.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"vgpu/internal/conf"
	"vgpu/internal/exporter"
	"vgpu/internal/webentry"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"

	_ "go.uber.org/automaxprocs"
)

const (
	defaultConfigPath       = "../../config/config.yaml"
	defaultWebListenAddress = ":3000"
	defaultStaticDir        = "/apps/public"
	defaultBasePath         = "/"
	defaultFrameAncestors   = "null"
	defaultRequestTimeout   = 60 * time.Second
)

var (
	// Name is the name of the compiled software.
	Name string
	// Version is the version of the compiled software.
	Version string

	id, _ = os.Hostname()
)

type webConfig struct {
	listenAddress  string
	staticDir      string
	basePath       string
	frameAncestors []string
}

type options struct {
	configPath string
	web        webConfig
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
		return fmt.Errorf("invalid command-line options: %w", err)
	}
	ctx := context.Background()
	app, cleanup, err := initApp(config.configPath, config.web, ctx)
	if err != nil {
		return fmt.Errorf("initialize application: %w", err)
	}
	defer cleanup()
	if err := app.Run(); err != nil {
		return fmt.Errorf("run application: %w", err)
	}
	return nil
}

func parseOptions(args []string, lookupEnv func(string) (string, bool)) (options, error) {
	config := options{}
	var frameAncestorsJSON string
	flags := flag.NewFlagSet("hami-webui", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.configPath, "conf", defaultConfigPath, "backend config path")
	flags.StringVar(&config.web.listenAddress, "listen-address", envOrDefault(lookupEnv, "HAMI_WEBUI_LISTEN_ADDRESS", defaultWebListenAddress), "public HTTP listen address")
	flags.StringVar(&config.web.staticDir, "static-dir", envOrDefault(lookupEnv, "HAMI_WEBUI_STATIC_DIR", defaultStaticDir), "SPA static directory")
	flags.StringVar(&config.web.basePath, "base-path", envOrDefault(lookupEnv, "HAMI_WEBUI_BASE_PATH", defaultBasePath), "external URL base path")
	flags.StringVar(&frameAncestorsJSON, "frame-ancestors-json", envOrDefault(lookupEnv, "HAMI_WEBUI_FRAME_ANCESTORS_JSON", defaultFrameAncestors), "JSON array of allowed iframe ancestors, [] to deny all, or null to omit")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	if err := json.Unmarshal([]byte(frameAncestorsJSON), &config.web.frameAncestors); err != nil {
		return options{}, errors.New("frame ancestors must be null or a JSON array of allowed sources")
	}
	return config, nil
}

func envOrDefault(lookupEnv func(string) (string, bool), key, fallback string) string {
	if value, ok := lookupEnv(key); ok {
		return value
	}
	return fallback
}

func newWebServer(config webConfig, bootstrap *conf.Bootstrap, api *kratoshttp.Server) (*webentry.LifecycleServer, error) {
	handler, err := newWebHandler(config, api)
	if err != nil {
		return nil, err
	}
	requestTimeout := backendRequestTimeout(bootstrap)
	server, err := webentry.NewHTTPServer(webentry.HTTPServerConfig{
		Address:        config.listenAddress,
		Handler:        handler,
		RequestTimeout: requestTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("configure public HTTP server: %w", err)
	}
	managed, err := webentry.NewLifecycleServer(server)
	if err != nil {
		return nil, fmt.Errorf("configure public HTTP lifecycle: %w", err)
	}
	return managed, nil
}

func newWebHandler(config webConfig, api http.Handler) (http.Handler, error) {
	if config.staticDir == "" {
		return nil, errors.New("static directory must not be empty")
	}
	handler, err := webentry.NewHandler(webentry.HandlerConfig{
		StaticFS:       os.DirFS(config.staticDir),
		APIHandler:     api,
		BasePath:       config.basePath,
		FrameAncestors: config.frameAncestors,
	})
	if err != nil {
		return nil, fmt.Errorf("configure public Web handler: %w", err)
	}
	return handler, nil
}

func backendRequestTimeout(bootstrap *conf.Bootstrap) time.Duration {
	if bootstrap == nil || bootstrap.Server == nil || bootstrap.Server.Http == nil || bootstrap.Server.Http.Timeout == nil {
		return defaultRequestTimeout
	}
	if timeout := bootstrap.Server.Http.Timeout.AsDuration(); timeout != 0 {
		return timeout
	}
	return defaultRequestTimeout
}

// newApp registers both HTTP listeners in the same lifecycle. The public Web
// listener invokes the backend HTTP server directly as an http.Handler; it does
// not use a loopback proxy.
func newApp(
	ctx context.Context,
	logger log.Logger,
	gs *grpc.Server,
	hs *kratoshttp.Server,
	mc *exporter.MetricsGenerator,
	ws *webentry.LifecycleServer,
) *kratos.App {
	return kratos.New(
		kratos.Context(ctx),
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(gs, hs, mc, ws),
	)
}

func getNodeSelectors(c *conf.Bootstrap) map[string]string {
	return c.NodeSelectors
}
