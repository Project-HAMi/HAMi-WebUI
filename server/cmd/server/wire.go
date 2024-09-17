//go:build wireinject
// +build wireinject

// The build tag makes sure the stub is not built in the final build.

package main

import (
	"context"
	"vgpu/internal"
	"vgpu/internal/biz"
	"vgpu/internal/data"
	"vgpu/internal/exporter"
	"vgpu/internal/server"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"
)

func initApp(configPath string, ctx context.Context) (*kratos.App, func(), error) {
	panic(wire.Build(internal.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		server.ProviderSet,
		service.ProviderSet,
		exporter.ProviderSet,
		newApp,
	))
}
