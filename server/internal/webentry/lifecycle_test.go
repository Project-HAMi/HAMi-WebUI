package webentry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestLifecycleServerServesAndStops(t *testing.T) {
	t.Parallel()

	server, err := NewHTTPServer(HTTPServerConfig{
		Address:        "127.0.0.1:0",
		Handler:        http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }),
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	lifecycle, err := NewLifecycleServer(server)
	if err != nil {
		t.Fatalf("NewLifecycleServer: %v", err)
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- lifecycle.Start(context.Background())
	}()

	address := waitForLifecycleAddress(t, lifecycle)
	response, err := http.Get("http://" + address)
	if err != nil {
		t.Fatalf("GET public server: %v", err)
	}
	defer closeResponseBody(t, response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Fatalf("response = {%d %q}, want {200 %q}", response.StatusCode, body, "ok")
	}

	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := <-serveResult; err != nil {
		t.Fatalf("Start returned: %v", err)
	}
}

func TestLifecycleServerPropagatesListenFailure(t *testing.T) {
	t.Parallel()

	server, err := NewHTTPServer(HTTPServerConfig{
		Address:        "invalid-address",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	lifecycle, err := NewLifecycleServer(server)
	if err != nil {
		t.Fatalf("NewLifecycleServer: %v", err)
	}
	if err := lifecycle.Start(context.Background()); err == nil {
		t.Fatal("Start succeeded with an invalid listen address")
	}
}

func TestLifecycleServerStopBeforeStartDoesNotOpenListener(t *testing.T) {
	t.Parallel()

	server, err := NewHTTPServer(HTTPServerConfig{
		Address:        "invalid-address",
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	lifecycle, err := NewLifecycleServer(server)
	if err != nil {
		t.Fatalf("NewLifecycleServer: %v", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Start after Stop: %v", err)
	}
}

func TestLifecycleServerBoundsGracefulShutdown(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server, err := NewHTTPServer(HTTPServerConfig{
		Address: "127.0.0.1:0",
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			close(requestStarted)
			<-releaseRequest
		}),
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewHTTPServer: %v", err)
	}
	lifecycle, err := NewLifecycleServer(server)
	if err != nil {
		t.Fatalf("NewLifecycleServer: %v", err)
	}
	serveResult := make(chan error, 1)
	go func() {
		serveResult <- lifecycle.Start(context.Background())
	}()

	address := waitForLifecycleAddress(t, lifecycle)
	clientResult := make(chan struct{})
	go func() {
		response, getErr := http.Get("http://" + address)
		if getErr == nil {
			_ = response.Body.Close()
		}
		close(clientResult)
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := lifecycle.Stop(ctx); !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("Stop error = %v, want ErrShutdownTimeout", err)
	}
	close(releaseRequest)
	select {
	case <-clientResult:
	case <-time.After(time.Second):
		t.Fatal("client did not return after forced close")
	}
	if err := <-serveResult; err != nil {
		t.Fatalf("Start returned: %v", err)
	}
}

func TestNewLifecycleServerRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewLifecycleServer(nil); err == nil {
		t.Fatal("NewLifecycleServer accepted nil")
	}
	if _, err := NewLifecycleServer(&http.Server{}); err == nil {
		t.Fatal("NewLifecycleServer accepted an empty listen address")
	}
}

func waitForLifecycleAddress(t *testing.T, server *LifecycleServer) string {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		listener := server.listener
		server.mu.Unlock()
		if listener != nil {
			return listener.Addr().String()
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("server did not open its listener")
	return ""
}
