package webentry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 30 * time.Second
	defaultIdleTimeout       = 120 * time.Second
	requestTimeoutMargin     = 5 * time.Second
	defaultMaxHeaderBytes    = 1 << 20
)

// ErrShutdownTimeout indicates that in-flight requests did not finish within
// the configured graceful-shutdown window and were forcefully disconnected.
var ErrShutdownTimeout = errors.New("graceful shutdown timed out")

// HTTPServerConfig defines the bounded public HTTP server. RequestTimeout is
// used to keep the outer HTTP deadline longer than the application handler's
// own deadline, whether that handler is a reverse proxy or an in-process API.
type HTTPServerConfig struct {
	Address        string
	Handler        http.Handler
	RequestTimeout time.Duration
}

// NewHTTPServer constructs an HTTP server with explicit slow-client and idle
// connection bounds. It does not open a listener.
func NewHTTPServer(config HTTPServerConfig) (*http.Server, error) {
	if config.RequestTimeout <= 0 {
		return nil, errors.New("request timeout must be positive")
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if config.RequestTimeout > maxDuration-defaultReadTimeout-requestTimeoutMargin {
		return nil, errors.New("request timeout is too large")
	}
	handler := config.Handler
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	writeTimeout := defaultReadTimeout + config.RequestTimeout + requestTimeoutMargin
	return &http.Server{
		Addr:              config.Address,
		Handler:           handler,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}, nil
}

// Serve runs server on listener until the server exits or ctx is cancelled.
// Cancellation performs graceful shutdown and force-closes connections after
// shutdownTimeout, ensuring SIGTERM handling remains bounded.
func Serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	if server == nil || listener == nil {
		return errors.New("server and listener are required")
	}
	if shutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve(listener)
	}()

	select {
	case err := <-serveResult:
		return normalizeServeError(err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-serveResult
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("%w after %s", ErrShutdownTimeout, shutdownTimeout)
			}
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return normalizeServeError(<-serveResult)
	}
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}
