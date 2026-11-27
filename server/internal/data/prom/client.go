package prom

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
)

var (
	// ErrResponseTooLarge indicates a Prometheus query response exceeded the client byte limit.
	ErrResponseTooLarge = errors.New("prometheus query response exceeded size limit")
	// ErrCircuitOpen indicates the Prometheus circuit breaker is open after repeated upstream failures.
	ErrCircuitOpen = errors.New("prometheus circuit breaker is open")
)

const (
	defaultMaxConcurrentQueries    = 4
	defaultMaxQueryResponseBytes   int64 = 8 << 20
	defaultCircuitFailureThreshold = 3
	defaultCircuitCooldown         = 15 * time.Second
)

type circuitState uint8

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

type Client struct {
	client      api.Client
	queryClient api.Client
	httpClient  *http.Client
	timeout     time.Duration
	log         *log.Helper

	sem              chan struct{}
	now              func() time.Time
	circuitMu        sync.Mutex
	circuit          circuitState
	circuitEpoch     uint64
	consecutiveFails int
	openUntil        time.Time
	failureThreshold int
	cooldown         time.Duration

	queriesOK              atomic.Uint64
	queriesError           atomic.Uint64
	queriesRejectedCircuit atomic.Uint64
	queriesTooLarge        atomic.Uint64
	queriesCanceled        atomic.Uint64
	circuitOpens           atomic.Uint64
}

// QueryStats reports bounded-client query outcomes and circuit-breaker state.
type QueryStats struct {
	QueriesOK              uint64
	QueriesError           uint64
	QueriesRejectedCircuit uint64
	QueriesTooLarge        uint64
	QueriesCanceled        uint64
	CircuitOpens           uint64
	CircuitOpen            bool
}

// Stats returns a point-in-time snapshot of Prometheus query counters and breaker state.
func (c *Client) Stats() QueryStats {
	if c == nil {
		return QueryStats{}
	}
	c.circuitMu.Lock()
	circuitOpen := c.circuit != circuitClosed
	c.circuitMu.Unlock()
	return QueryStats{
		QueriesOK:              c.queriesOK.Load(),
		QueriesError:           c.queriesError.Load(),
		QueriesRejectedCircuit: c.queriesRejectedCircuit.Load(),
		QueriesTooLarge:        c.queriesTooLarge.Load(),
		QueriesCanceled:        c.queriesCanceled.Load(),
		CircuitOpens:           c.circuitOpens.Load(),
		CircuitOpen:            circuitOpen,
	}
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

type responseLimitRoundTripper struct {
	limit int64
	next  http.RoundTripper
}

func (t *responseLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if t.limit > 0 {
		if resp.ContentLength > t.limit {
			_ = resp.Body.Close()
			return nil, ErrResponseTooLarge
		}
		resp.Body = &limitedReadCloser{rc: resp.Body, remaining: t.limit}
	}
	return resp, nil
}

func (t *responseLimitRoundTripper) CloseIdleConnections() {
	if closeIdler, ok := t.next.(interface{ CloseIdleConnections() }); ok {
		closeIdler.CloseIdleConnections()
	}
}

type limitedReadCloser struct {
	rc        io.ReadCloser
	remaining int64
}

func (r *limitedReadCloser) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.rc.Read(probe[:])
		if n > 0 {
			return 0, ErrResponseTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}
	n, err := r.rc.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func (r *limitedReadCloser) Close() error {
	return r.rc.Close()
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

	// WithTimeout only bounds evaluation on the server; this bounds the whole request.
	// Keep the unwrapped client for Conn() and streaming WalkActiveTargets, and apply
	// the query response size limit only to PromQL query calls through queryClient.
	httpClient := &http.Client{Transport: roundTripper, Timeout: timeout}
	client, err := api.NewClient(api.Config{
		Address: address,
		Client:  httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create Prometheus client: %w", err)
	}
	queryHTTPClient := &http.Client{
		Transport: &responseLimitRoundTripper{limit: defaultMaxQueryResponseBytes, next: roundTripper},
		Timeout:   timeout,
	}
	queryClient, err := api.NewClient(api.Config{
		Address: address,
		Client:  queryHTTPClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create Prometheus query client: %w", err)
	}
	return &Client{
		client:           client,
		queryClient:      queryClient,
		httpClient:       httpClient,
		timeout:          timeout,
		log:              log.NewHelper(log.With(logger, "module", "prometheus-client")),
		sem:              make(chan struct{}, defaultMaxConcurrentQueries),
		now:              time.Now,
		failureThreshold: defaultCircuitFailureThreshold,
		cooldown:         defaultCircuitCooldown,
	}, nil
}

func (c *Client) clockNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Client) isCircuitRejected() bool {
	now := c.clockNow()
	c.circuitMu.Lock()
	defer c.circuitMu.Unlock()
	return c.circuit == circuitHalfOpen || (c.circuit == circuitOpen && now.Before(c.openUntil))
}

func (c *Client) beforeQuery() (isProbe bool, epoch uint64, err error) {
	now := c.clockNow()
	c.circuitMu.Lock()
	defer c.circuitMu.Unlock()
	switch c.circuit {
	case circuitOpen:
		if now.Before(c.openUntil) {
			return false, 0, ErrCircuitOpen
		}
		c.circuit = circuitHalfOpen
		c.circuitEpoch++
		return true, c.circuitEpoch, nil
	case circuitHalfOpen:
		return false, 0, ErrCircuitOpen
	default:
		return false, c.circuitEpoch, nil
	}
}

func (c *Client) abortProbe(epoch uint64) {
	c.circuitMu.Lock()
	if c.circuit == circuitHalfOpen && c.circuitEpoch == epoch {
		c.circuit = circuitOpen
		c.circuitEpoch++
	}
	c.circuitMu.Unlock()
}

func (c *Client) afterQuery(isProbe bool, epoch uint64, err, ctxErr error) {
	threshold := c.failureThreshold
	if threshold <= 0 {
		threshold = defaultCircuitFailureThreshold
	}
	cooldown := c.cooldown
	if cooldown <= 0 {
		cooldown = defaultCircuitCooldown
	}
	now := c.clockNow()

	c.circuitMu.Lock()
	if epoch != c.circuitEpoch {
		c.circuitMu.Unlock()
		return
	}
	if err == nil {
		if isProbe {
			c.circuit = circuitClosed
			c.consecutiveFails = 0
			c.openUntil = time.Time{}
			c.circuitEpoch++
		} else if c.circuit == circuitClosed {
			c.consecutiveFails = 0
		}
		c.circuitMu.Unlock()
		return
	}
	if errors.Is(ctxErr, context.Canceled) || errors.Is(err, context.Canceled) {
		if isProbe && c.circuit == circuitHalfOpen {
			c.circuit = circuitOpen
			c.circuitEpoch++
		}
		c.circuitMu.Unlock()
		return
	}
	if !isUpstreamAvailabilityFailure(err) {
		if isProbe && c.circuit == circuitHalfOpen {
			c.circuit = circuitClosed
			c.consecutiveFails = 0
			c.openUntil = time.Time{}
			c.circuitEpoch++
		}
		c.circuitMu.Unlock()
		return
	}
	if isProbe {
		c.circuit = circuitOpen
		c.openUntil = now.Add(cooldown)
		c.circuitEpoch++
		c.circuitOpens.Add(1)
		c.circuitMu.Unlock()
		return
	}
	if c.circuit != circuitClosed {
		c.circuitMu.Unlock()
		return
	}
	c.consecutiveFails++
	opened := false
	if c.consecutiveFails >= threshold {
		c.circuit = circuitOpen
		c.openUntil = now.Add(cooldown)
		c.circuitEpoch++
		c.circuitOpens.Add(1)
		opened = true
	}
	fails := c.consecutiveFails
	c.circuitMu.Unlock()
	if opened && c.log != nil {
		c.log.Warnw("msg", "Prometheus circuit breaker opened", "consecutive_failures", fails, "cooldown", cooldown)
	}
}

func isUpstreamAvailabilityFailure(err error) bool {
	if errors.Is(err, ErrResponseTooLarge) {
		return false
	}
	var apiErr *v1.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Type {
		case v1.ErrBadData, v1.ErrClient, v1.ErrCanceled:
			return false
		default:
			return true
		}
	}
	return true
}

func (c *Client) recordOutcome(err, ctxErr error) {
	switch {
	case err == nil:
		c.queriesOK.Add(1)
	case errors.Is(err, ErrCircuitOpen):
		c.queriesRejectedCircuit.Add(1)
	case errors.Is(ctxErr, context.Canceled) || errors.Is(err, context.Canceled):
		c.queriesCanceled.Add(1)
	case errors.Is(err, ErrResponseTooLarge):
		c.queriesTooLarge.Add(1)
	default:
		c.queriesError.Add(1)
	}
}

func (c *Client) runBounded(ctx context.Context, fn func() error) error {
	if c.isCircuitRejected() {
		c.recordOutcome(ErrCircuitOpen, nil)
		return ErrCircuitOpen
	}
	if err := ctx.Err(); err != nil {
		c.recordOutcome(err, err)
		return err
	}
	if c.sem != nil {
		select {
		case c.sem <- struct{}{}:
		case <-ctx.Done():
			err := ctx.Err()
			c.recordOutcome(err, err)
			return err
		}
		defer func() { <-c.sem }()
		if err := ctx.Err(); err != nil {
			c.recordOutcome(err, err)
			return err
		}
	}
	isProbe, epoch, err := c.beforeQuery()
	if err != nil {
		c.recordOutcome(err, nil)
		return err
	}
	completed := false
	defer func() {
		if isProbe && !completed {
			c.abortProbe(epoch)
		}
	}()
	queryErr := fn()
	completed = true
	ctxErr := ctx.Err()
	c.afterQuery(isProbe, epoch, queryErr, ctxErr)
	c.recordOutcome(queryErr, ctxErr)
	return queryErr
}

func (c *Client) Conn() (api.Client, error) {
	return c.client, nil
}

// Query evaluates query at the current time.
func (c *Client) Query(ctx context.Context, query string) (model.Value, error) {
	return c.QueryAt(ctx, query, time.Now())
}

// QueryAt evaluates a query at a caller-selected instant. A collector cycle uses
// one instant for all of its bounded batches.
func (c *Client) QueryAt(ctx context.Context, query string, at time.Time) (model.Value, error) {
	var (
		result   model.Value
		warnings v1.Warnings
	)
	err := c.runBounded(ctx, func() error {
		var err error
		result, warnings, err = v1.NewAPI(c.queryClient).Query(ctx, query, at, v1.WithTimeout(c.timeout))
		return err
	})
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
	var (
		result   model.Value
		warnings v1.Warnings
	)
	err := c.runBounded(ctx, func() error {
		var err error
		result, warnings, err = v1.NewAPI(c.queryClient).QueryRange(ctx, query, r, v1.WithTimeout(c.timeout))
		return err
	})
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
