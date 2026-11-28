package server

import (
	nethttp "net/http"

	pb "vgpu/api/observability/v1"
	v1 "vgpu/api/v1"
	"vgpu/internal/conf"
	"vgpu/internal/service"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	httpReasonRouteNotFound    = "ROUTE_NOT_FOUND"
	httpReasonMethodNotAllowed = "METHOD_NOT_ALLOWED"
)

// APIHandler serves the public /api/v1/* endpoints in-process behind the
// unified Web entry on port 3000 without binding an external listener.
type APIHandler struct {
	nethttp.Handler
}

// NewAPIHandler builds the in-process HTTP handler serving canonical /api/v1/*
// resources, cluster overview, and bounded telemetry endpoints.
func NewAPIHandler(
	node *service.NodeService,
	card *service.CardService,
	ctr *service.ContainerService,
	scheduling *service.SchedulingService,
	workload *service.WorkloadService,
	deviceConfig *service.DeviceConfigService,
	overview *service.OverviewService,
	telemetrySvc *service.TelemetryService,
	about *service.AboutService,
	_ log.Logger,
) APIHandler {
	srv := http.NewServer(
		http.Middleware(
			recovery.Recovery(),
			metrics.Server(),
		),
		http.RequestQueryDecoder(overviewQuery),
		http.NotFoundHandler(httpErrorHandler(nethttp.StatusNotFound, httpReasonRouteNotFound)),
		http.MethodNotAllowedHandler(httpErrorHandler(nethttp.StatusMethodNotAllowed, httpReasonMethodNotAllowed)),
	)
	v1.RegisterResourcesHTTPServer(srv, service.NewResourcesService(node, card, ctr, workload, scheduling, deviceConfig, about))
	pb.RegisterOverviewHTTPServer(srv, overview)
	pb.RegisterTelemetryHTTPServer(srv, telemetrySvc)
	return APIHandler{Handler: srv}
}

// NewHTTPServer builds the internal :8000 HTTP server exposing /metrics and /readyz only.
//
// The /metrics endpoint here only serializes the in-memory Prometheus registry.
// The registry is refreshed in the background by exporter.MetricsGenerator, which
// is registered as its own kratos transport.Server in main.go. This is what keeps
// scrape latency O(1) and decouples /metrics from server.http.timeout — at customer
// scale a single refresh can fan out ~1800 PromQL queries, which previously raced
// the 1s request deadline and starved vmselect.
//
// /readyz answers 200 once this HTTP server is accepting requests. The backend only
// starts listening after the k8s informer caches have synced (see data.NewNodeRepo /
// NewPodRepo), so a readiness probe on /readyz keeps the pod out of Service endpoints
// until the backend can actually serve — this is what prevents the Web entry from
// getting "connection refused" against the not-yet-listening backend at startup.
func NewHTTPServer(c *conf.Bootstrap,
	gatherer prometheus.Gatherer,
	logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			metrics.Server(),
		),
		http.NotFoundHandler(httpErrorHandler(nethttp.StatusNotFound, httpReasonRouteNotFound)),
		http.MethodNotAllowedHandler(httpErrorHandler(nethttp.StatusMethodNotAllowed, httpReasonMethodNotAllowed)),
	}
	if c.Server.Http.Network != "" {
		opts = append(opts, http.Network(c.Server.Http.Network))
	}
	if c.Server.Http.Addr != "" {
		opts = append(opts, http.Address(c.Server.Http.Addr))
	}
	if c.Server.Http.Timeout != nil {
		opts = append(opts, http.Timeout(c.Server.Http.Timeout.AsDuration()))
	}
	srv := http.NewServer(opts...)
	srv.Handle("/metrics", promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})))
	srv.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Reaching this handler means the server is listening and the informer
		// caches have already synced (the backend only starts serving after that).
		w.WriteHeader(nethttp.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return srv
}

func httpErrorHandler(status int, reason string) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		http.DefaultErrorEncoder(w, r, kratoserrors.New(status, reason, nethttp.StatusText(status)))
	})
}
