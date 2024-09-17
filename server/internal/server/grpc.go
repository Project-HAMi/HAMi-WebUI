package server

import (
	v1 "vgpu/api/v1"
	"vgpu/internal/conf"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/grpc"
)

// NewGRPCServer new a gRPC server.
func NewGRPCServer(c *conf.Bootstrap,
	node *service.NodeService,
	card *service.CardService,
	ctr *service.ContainerService,
	monitor *service.MonitorService,
	logger log.Logger) *grpc.Server {
	var opts = []grpc.ServerOption{
		grpc.Middleware(
			recovery.Recovery(),
		),
	}
	if c.Server.Grpc.Network != "" {
		opts = append(opts, grpc.Network(c.Server.Grpc.Network))
	}
	if c.Server.Grpc.Addr != "" {
		opts = append(opts, grpc.Address(c.Server.Grpc.Addr))
	}
	if c.Server.Grpc.Timeout != nil {
		opts = append(opts, grpc.Timeout(c.Server.Grpc.Timeout.AsDuration()))
	}
	srv := grpc.NewServer(opts...)
	v1.RegisterNodeServer(srv, node)
	v1.RegisterCardServer(srv, card)
	v1.RegisterContainerServer(srv, ctr)
	v1.RegisterMonitorServer(srv, monitor)
	return srv
}
