package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

func TestPrometheusBasicAuthConfigurationIsWiredEndToEnd(t *testing.T) {
	directory := t.TempDir()
	usernameFile := filepath.Join(directory, "username")
	passwordFile := filepath.Join(directory, "password")
	if err := os.WriteFile(usernameFile, []byte("metrics-user\n"), 0o600); err != nil {
		t.Fatalf("write username: %v", err)
	}
	if err := os.WriteFile(passwordFile, []byte("metrics-password\n"), 0o600); err != nil {
		t.Fatalf("write password: %v", err)
	}

	type credentials struct {
		username string
		password string
	}
	received := make(chan credentials, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username, password, _ := req.BasicAuth()
		received <- credentials{username: username, password: password}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	t.Cleanup(server.Close)

	configPath := filepath.Join(directory, "config.yaml")
	config := fmt.Sprintf(`prometheus:
  address: %q
  timeout: 1s
  basic_auth:
    username_file: %q
    password_file: %q
`, server.URL, usernameFile, passwordFile)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
	bootstrap, err := NewConfig(configPath)
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}
	client := NewPromClient(bootstrap, log.NewStdLogger(io.Discard))
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("query Prometheus: %v", err)
	}
	if got := <-received; got.username != "metrics-user" || got.password != "metrics-password" {
		t.Fatalf("basic auth credentials = %#v, want metrics-user/metrics-password", got)
	}
}
