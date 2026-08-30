package webentry

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerUsesBoundedTimeouts(t *testing.T) {
	t.Parallel()

	server, err := NewHTTPServer(HTTPServerConfig{
		Address:        ":3000",
		Handler:        http.NotFoundHandler(),
		RequestTimeout: 65 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}

	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Fatalf("server timeouts must be bounded: %+v", server)
	}
	if server.WriteTimeout <= 65*time.Second {
		t.Errorf("WriteTimeout = %s, must exceed proxy timeout", server.WriteTimeout)
	}
	if server.MaxHeaderBytes <= 0 {
		t.Errorf("MaxHeaderBytes = %d, want positive limit", server.MaxHeaderBytes)
	}
}

func TestServeShutsDownGracefully(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		_, _ = io.WriteString(w, "done")
	})
	server, err := NewHTTPServer(HTTPServerConfig{
		Handler:        handler,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- Serve(ctx, server, listener, time.Second)
	}()

	responseResult := make(chan error, 1)
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			defer closeResponseBody(t, response.Body)
			_, err = io.ReadAll(response.Body)
		}
		responseResult <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}

	cancel()
	select {
	case err := <-serveResult:
		t.Fatalf("Serve returned before in-flight request completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseRequest)

	if err := <-responseResult; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}
	if err := <-serveResult; err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
}

func TestServeBoundsGracefulShutdown(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseRequest
	})
	server, err := NewHTTPServer(HTTPServerConfig{
		Handler:        handler,
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- Serve(ctx, server, listener, 30*time.Millisecond)
	}()
	clientResult := make(chan struct{})
	go func() {
		response, err := http.Get("http://" + listener.Addr().String())
		if err == nil {
			_ = response.Body.Close()
		}
		close(clientResult)
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}

	cancel()
	err = <-serveResult
	if !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("Serve error = %v, want ErrShutdownTimeout", err)
	}
	close(releaseRequest)
	select {
	case <-clientResult:
	case <-time.After(time.Second):
		t.Fatal("client did not return after forced close")
	}
}

func TestNewHTTPServerRejectsInvalidRequestTimeout(t *testing.T) {
	t.Parallel()

	if _, err := NewHTTPServer(HTTPServerConfig{}); err == nil {
		t.Fatal("NewHTTPServer succeeded with zero proxy timeout")
	}
}
