package server

import (
	v1 "vgpu/api/v1"
	"vgpu/internal/conf"
	"vgpu/internal/exporter"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/metrics"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"
	"github.com/go-kratos/swagger-api/openapiv2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Bootstrap,
	node *service.NodeService,
	card *service.CardService,
	ctr *service.ContainerService,
	monitor *service.MonitorService,
	exporter *exporter.MetricsGenerator,
	logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			metrics.Server(),
		),
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
	v1.RegisterNodeHTTPServer(srv, node)
	v1.RegisterCardHTTPServer(srv, card)
	v1.RegisterContainerHTTPServer(srv, ctr)
	v1.RegisterMonitorHTTPServer(srv, monitor)
	srv.HandlePrefix("/q/", openapiv2.NewHandler())
	srv.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		exporter.GenerateMetrics(r.Context())
		//mock.MockMetrics(r.Context())
		promhttp.Handler().ServeHTTP(w, r)
	})
	return srv
}
