package webentry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// LifecycleServer adapts the bounded public HTTP server to the lifecycle used
// by the HAMi-WebUI application. It opens the listener in Start, propagates
// serve failures to the application, and force-closes connections if graceful
// shutdown exceeds the application deadline.
type LifecycleServer struct {
	server *http.Server

	mu       sync.Mutex
	listener net.Listener
	started  bool
	stopped  bool
}

// NewLifecycleServer wraps a public HTTP server without opening its listener.
func NewLifecycleServer(server *http.Server) (*LifecycleServer, error) {
	if server == nil {
		return nil, errors.New("HTTP server is required")
	}
	if server.Addr == "" {
		return nil, errors.New("HTTP listen address is required")
	}
	return &LifecycleServer{server: server}, nil
}

// Start opens the configured TCP listener and serves until Stop is called or
// the HTTP server fails. A failure is returned so the application stops its
// other transports instead of leaving a partially running process.
func (s *LifecycleServer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("HTTP server already started")
	}
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.server.Addr, err)
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		_ = listener.Close()
		return nil
	}
	s.listener = listener
	s.server.BaseContext = func(net.Listener) context.Context { return ctx }
	s.mu.Unlock()

	err = normalizeServeError(s.server.Serve(listener))
	_ = listener.Close()
	return err
}

// Stop gracefully drains the public listener. If the caller's deadline is
// exceeded, active connections are force-closed so application shutdown stays
// bounded.
func (s *LifecycleServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.stopped = true
	listener := s.listener
	s.mu.Unlock()

	if listener == nil {
		return nil
	}
	if err := s.server.Shutdown(ctx); err != nil {
		_ = s.server.Close()
		_ = listener.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w", ErrShutdownTimeout)
		}
		return fmt.Errorf("shut down HTTP server: %w", err)
	}
	_ = listener.Close()
	return nil
}
