package prom

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

type Client struct {
	client  api.Client
	timeout time.Duration
	log     *log.Helper
}

type TLSConfig struct {
	CAFile             string
	CertFile           string
	KeyFile            string
	ServerName         string
	InsecureSkipVerify bool
}

type authorizationRoundTripper struct {
	auth string
	next http.RoundTripper
}

func (t *authorizationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", t.auth)
	return t.next.RoundTrip(clone)
}

func NewClient(address string, timeout time.Duration, auth string, tlsConfig TLSConfig, logger log.Logger) (*Client, error) {
	httpConfig := promconfig.DefaultHTTPClientConfig
	httpConfig.TLSConfig = promconfig.TLSConfig{
		CAFile:             tlsConfig.CAFile,
		CertFile:           tlsConfig.CertFile,
		KeyFile:            tlsConfig.KeyFile,
		ServerName:         tlsConfig.ServerName,
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify,
	}
	roundTripper, err := promconfig.NewRoundTripperFromConfig(httpConfig, "hami-webui-prometheus")
	if err != nil {
		return nil, fmt.Errorf("create Prometheus HTTP transport: %w", err)
	}
	if auth != "" {
		roundTripper = &authorizationRoundTripper{auth: auth, next: roundTripper}
	}

	client, err := api.NewClient(api.Config{
		Address:      address,
		RoundTripper: roundTripper,
	})
	if err != nil {
		return nil, fmt.Errorf("create Prometheus client: %w", err)
	}
	return &Client{
		client:  client,
		timeout: timeout,
		log:     log.NewHelper(log.With(logger, "module", "prometheus-client")),
	}, nil
}

func (c *Client) Conn() (api.Client, error) {
	return c.client, nil
}

// Query evaluates query at the current time.
func (c *Client) Query(ctx context.Context, query string) (model.Value, error) {
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.Query(ctx, query, time.Now(), v1.WithTimeout(c.timeout))
	if err != nil {
		return result, fmt.Errorf("query Prometheus: %w", err)
	}
	if len(warnings) > 0 {
		c.log.WithContext(ctx).Warnw(
			"msg", "Prometheus query completed with warnings",
			"operation", "instant",
			"warning_count", len(warnings),
			"warnings", []string(warnings),
		)
	}
	return result, nil
}

// QueryRange evaluates query over the supplied time range.
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, error) {
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.QueryRange(ctx, query, r, v1.WithTimeout(c.timeout))
	if err != nil {
		return result, fmt.Errorf("query Prometheus range: %w", err)
	}
	if len(warnings) > 0 {
		c.log.WithContext(ctx).Warnw(
			"msg", "Prometheus query completed with warnings",
			"operation", "range",
			"warning_count", len(warnings),
			"warnings", []string(warnings),
		)
	}
	return result, nil
}
