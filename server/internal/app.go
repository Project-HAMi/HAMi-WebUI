package internal

import (
	"os"
	"time"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(NewConfig, NewLogger, NewPromClient)

func NewConfig(path string) (*conf.Bootstrap, error) {
	c := config.New(
		config.WithSource(
			file.NewSource(path),
		),
	)
	defer c.Close()

	if err := c.Load(); err != nil {
		return nil, err
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		return nil, err
	}
	return &bc, nil
}

func NewLogger() log.Logger {
	return log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
	)
}

func NewPromClient(c *conf.Bootstrap) *prom.Client {
	timeout, err := time.ParseDuration(c.Prometheus.Timeout)
	if err != nil {
		panic(err)
	}
	client, err := prom.NewClient(c.Prometheus.Address, timeout, c.Prometheus.Auth)
	if err != nil {
		panic(err)
	}
	return client
}
