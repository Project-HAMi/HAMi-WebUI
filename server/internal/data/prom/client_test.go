package prom

import (
	"context"
	"encoding/pem"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSVerificationDefaultsToEnabled(t *testing.T) {
	server := newTLSTestServer(t)
	client := newTestClient(t, server.URL, TLSConfig{})

	if err := request(t, client, server.URL); err == nil {
		t.Fatal("request with an untrusted certificate succeeded")
	}
}

func TestTLSVerificationCanBeExplicitlyDisabled(t *testing.T) {
	server := newTLSTestServer(t)
	client := newTestClient(t, server.URL, TLSConfig{InsecureSkipVerify: true})

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request with explicit insecure mode failed: %v", err)
	}
}

func TestTLSVerificationUsesConfiguredCA(t *testing.T) {
	server := newTLSTestServer(t)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	certificate := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: server.Certificate().Raw,
	})
	if err := os.WriteFile(caFile, certificate, 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	client := newTestClient(t, server.URL, TLSConfig{CAFile: caFile})

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request with configured CA failed: %v", err)
	}
}

func TestLegacyAuthorizationHeaderIsPreserved(t *testing.T) {
	const authorization = "Bearer test-token"
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, time.Second, authorization, TLSConfig{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request with authorization failed: %v", err)
	}
	if got := <-received; got != authorization {
		t.Fatalf("Authorization header = %q, want %q", got, authorization)
	}
}

func newTLSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newTestClient(t *testing.T, address string, tlsConfig TLSConfig) *Client {
	t.Helper()
	client, err := NewClient(address, time.Second, "", tlsConfig)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

func request(t *testing.T, client *Client, address string) error {
	t.Helper()
	connection, err := client.Conn()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	_, _, err = connection.Do(context.Background(), req)
	return err
}
