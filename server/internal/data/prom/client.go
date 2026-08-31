package prom

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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

type AuthorizationConfig struct {
	Type            string
	CredentialsFile string
}

type BasicAuthConfig struct {
	UsernameFile string
	PasswordFile string
}

type HTTPConfig struct {
	TLS                 TLSConfig
	Authorization       *AuthorizationConfig
	BasicAuth           *BasicAuthConfig
	LegacyAuthorization string
}

// authorizationRoundTripper preserves the pre-2.0 raw auth setting for direct
// configuration users. New configurations should use the file-backed standard
// authorization or basic_auth transport instead.
type authorizationRoundTripper struct {
	auth string
	next http.RoundTripper
}

// credentialOriginRoundTripper prevents inner authentication transports from
// attaching credentials to a redirect or request outside the configured
// Prometheus origin.
type credentialOriginRoundTripper struct {
	origin *url.URL
	next   http.RoundTripper
}

func (t *credentialOriginRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !sameOrigin(t.origin, req.URL) {
		return nil, errors.New("refusing to send Prometheus credentials to a different origin")
	}
	return t.next.RoundTrip(req)
}

func (t *credentialOriginRoundTripper) CloseIdleConnections() {
	if closeIdler, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closeIdler.CloseIdleConnections()
	}
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(address *url.URL) string {
	if port := address.Port(); port != "" {
		return port
	}
	switch strings.ToLower(address.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func (t *authorizationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", t.auth)
	return t.next.RoundTrip(clone)
}

func NewClient(address string, timeout time.Duration, config HTTPConfig, logger log.Logger) (*Client, error) {
	parsedAddress, err := url.Parse(address)
	if err != nil {
		return nil, errors.New("invalid Prometheus address")
	}
	if parsedAddress.User != nil {
		return nil, errors.New("prometheus address must not include user information; configure authentication separately")
	}
	if config.LegacyAuthorization != "" && (config.Authorization != nil || config.BasicAuth != nil) {
		return nil, errors.New("legacy Prometheus auth cannot be combined with authorization or basic_auth")
	}
	if config.Authorization != nil && config.BasicAuth != nil {
		return nil, errors.New("prometheus authorization and basic_auth are mutually exclusive")
	}
	if config.Authorization != nil && config.Authorization.CredentialsFile == "" {
		return nil, errors.New("prometheus authorization credentials file is required")
	}
	if config.BasicAuth != nil && (config.BasicAuth.UsernameFile == "" || config.BasicAuth.PasswordFile == "") {
		return nil, errors.New("prometheus basic_auth username and password files are required")
	}

	httpConfig := promconfig.DefaultHTTPClientConfig
	httpConfig.TLSConfig = promconfig.TLSConfig{
		CAFile:             config.TLS.CAFile,
		CertFile:           config.TLS.CertFile,
		KeyFile:            config.TLS.KeyFile,
		ServerName:         config.TLS.ServerName,
		InsecureSkipVerify: config.TLS.InsecureSkipVerify,
	}
	if config.Authorization != nil {
		httpConfig.Authorization = &promconfig.Authorization{
			Type:            config.Authorization.Type,
			CredentialsFile: config.Authorization.CredentialsFile,
		}
	}
	if config.BasicAuth != nil {
		httpConfig.BasicAuth = &promconfig.BasicAuth{
			UsernameFile: config.BasicAuth.UsernameFile,
			PasswordFile: config.BasicAuth.PasswordFile,
		}
	}
	if err := httpConfig.Validate(); err != nil {
		return nil, fmt.Errorf("validate Prometheus HTTP transport: %w", err)
	}
	roundTripper, err := promconfig.NewRoundTripperFromConfig(httpConfig, "hami-webui-prometheus")
	if err != nil {
		return nil, fmt.Errorf("create Prometheus HTTP transport: %w", err)
	}
	if config.LegacyAuthorization != "" {
		roundTripper = &authorizationRoundTripper{auth: config.LegacyAuthorization, next: roundTripper}
	}
	if config.LegacyAuthorization != "" || config.Authorization != nil || config.BasicAuth != nil {
		roundTripper = &credentialOriginRoundTripper{origin: parsedAddress, next: roundTripper}
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
