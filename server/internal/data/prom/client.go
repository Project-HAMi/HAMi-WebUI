package prom

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Client struct {
	client  api.Client
	timeout time.Duration
}

type CustomTransport struct {
	auth string
	*http.Transport
}

func (t *CustomTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", t.auth)
	return t.Transport.RoundTrip(req)
}

func NewClient(address string, timeout time.Duration, auth string) (*Client, error) {
	client, err := api.NewClient(api.Config{
		Address: address,
		RoundTripper: &CustomTransport{
			auth: auth,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // 忽略 SSL 证书验证
				},
			},
		},
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		return nil, fmt.Errorf("error creating client: %v", err)
	}
	return &Client{
		client,
		timeout,
	}, nil
}

func (c *Client) Conn() (api.Client, error) {
	return c.client, nil
}

// Query 查询单点时刻指标值
func (c *Client) Query(ctx context.Context, query string) (model.Value, error) {
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.Query(ctx, query, time.Now(), v1.WithTimeout(c.timeout))
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		return result, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
		return result, fmt.Errorf("warnings: %v", warnings)
	}
	return result, nil
}

// QueryRange 查询时间范围内指标变化趋势数据
func (c *Client) QueryRange(ctx context.Context, query string, r v1.Range) (model.Value, error) {
	v1api := v1.NewAPI(c.client)
	result, warnings, err := v1api.QueryRange(ctx, query, r, v1.WithTimeout(c.timeout))
	if err != nil {
		fmt.Printf("Error querying Prometheus: %v\n", err)
		return result, fmt.Errorf("error querying Prometheus: %v", err)
	}
	if len(warnings) > 0 {
		fmt.Printf("Warnings: %v\n", warnings)
		return result, fmt.Errorf("warnings: %v", warnings)
	}
	return result, nil
}
