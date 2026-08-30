package prom

import (
	"context"
	"encoding/pem"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
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
	client, err := NewClient(server.URL, time.Second, authorization, TLSConfig{}, log.DefaultLogger)
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
	server.Config.ErrorLog = stdlog.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func newTestClient(t *testing.T, address string, tlsConfig TLSConfig) *Client {
	t.Helper()
	client, err := NewClient(address, time.Second, "", tlsConfig, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

func TestQueryReturnsSuccessfulResultWithWarnings(t *testing.T) {
	logger := &recordingLogger{}
	server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/query" {
			http.Error(w, "unexpected query path", http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"test"},"value":[1788050000,"42"]}]},"warnings":["partial response"]}`)
	})
	client := newTestClientWithLogger(t, server.URL, TLSConfig{}, logger)

	value, err := client.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("Query returned a warning as an error: %v", err)
	}
	vector, ok := value.(model.Vector)
	if !ok || len(vector) != 1 || vector[0].Value != 42 {
		t.Fatalf("Query result = %#v, want one sample with value 42", value)
	}
	logger.requireWarning(t, "instant", "partial response")
}

func TestQueryRangeReturnsSuccessfulResultWithWarnings(t *testing.T) {
	logger := &recordingLogger{}
	server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/query_range" {
			http.Error(w, "unexpected query range path", http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"test"},"values":[[1788050000,"1"],[1788050060,"2"]]}]},"warnings":["partial range"]}`)
	})
	client := newTestClientWithLogger(t, server.URL, TLSConfig{}, logger)

	value, err := client.QueryRange(context.Background(), "up", v1.Range{
		Start: time.Unix(1788050000, 0),
		End:   time.Unix(1788050060, 0),
		Step:  time.Minute,
	})
	if err != nil {
		t.Fatalf("QueryRange returned a warning as an error: %v", err)
	}
	matrix, ok := value.(model.Matrix)
	if !ok || len(matrix) != 1 || len(matrix[0].Values) != 2 {
		t.Fatalf("QueryRange result = %#v, want one series with two samples", value)
	}
	logger.requireWarning(t, "range", "partial range")
}

func TestQueryPreservesPrometheusErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantError  string
	}{
		{
			name:       "API error",
			statusCode: http.StatusUnprocessableEntity,
			response:   `{"status":"error","errorType":"bad_data","error":"invalid query"}`,
			wantError:  "invalid query",
		},
		{
			name:       "HTTP error",
			statusCode: http.StatusServiceUnavailable,
			response:   "service unavailable",
			wantError:  "server_error",
		},
		{
			name:       "invalid response",
			statusCode: http.StatusOK,
			response:   `{"status":"success","data":`,
			wantError:  "bad_response",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newPrometheusAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = fmt.Fprint(w, tt.response)
			})
			client := newTestClient(t, server.URL, TLSConfig{})

			if _, err := client.Query(context.Background(), "invalid("); err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Query error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestQueryPreservesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := server.URL
	server.Close()
	client := newTestClient(t, address, TLSConfig{})

	if _, err := client.Query(context.Background(), "up"); err == nil {
		t.Fatal("Query with an unavailable Prometheus endpoint succeeded")
	}
}

func newPrometheusAPIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		handler(w, req)
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestClientWithLogger(t *testing.T, address string, tlsConfig TLSConfig, logger log.Logger) *Client {
	t.Helper()
	client, err := NewClient(address, time.Second, "", tlsConfig, logger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	return client
}

type logEntry struct {
	level   log.Level
	keyvals []any
}

type recordingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

func (l *recordingLogger) Log(level log.Level, keyvals ...any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: level, keyvals: append([]any(nil), keyvals...)})
	return nil
}

func (l *recordingLogger) requireWarning(t *testing.T, operation, warning string) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) != 1 {
		t.Fatalf("log entries = %#v, want one warning", l.entries)
	}
	entry := l.entries[0]
	if entry.level != log.LevelWarn {
		t.Fatalf("log level = %v, want warning", entry.level)
	}
	fields := make(map[any]any, len(entry.keyvals)/2)
	for i := 0; i+1 < len(entry.keyvals); i += 2 {
		fields[entry.keyvals[i]] = entry.keyvals[i+1]
	}
	if fields["operation"] != operation {
		t.Fatalf("operation = %v, want %q", fields["operation"], operation)
	}
	if fields["warning_count"] != 1 {
		t.Fatalf("warning_count = %v, want 1", fields["warning_count"])
	}
	if !reflect.DeepEqual(fields["warnings"], []string{warning}) {
		t.Fatalf("warnings = %#v, want %q", fields["warnings"], warning)
	}
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
