package prom

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func TestQueryAtUsesCallerInstant(t *testing.T) {
	got := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if err := req.ParseForm(); err != nil {
			t.Error(err)
		}
		got <- req.Form
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, time.Second, HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	value, err := client.QueryAt(context.Background(), "vector(0)", time.Unix(1700000000, 123000000))
	if err != nil {
		t.Fatal(err)
	}
	if vector, ok := value.(model.Vector); !ok || len(vector) != 0 {
		t.Fatalf("response = %#v, want successful empty vector", value)
	}
	form := <-got
	if form.Get("time") != "1700000000.123" || form.Get("query") != "vector(0)" {
		t.Fatalf("query instant changed: %v", form)
	}
}

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
	client, err := NewClient(server.URL, time.Second, HTTPConfig{LegacyAuthorization: authorization}, log.DefaultLogger)
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

func TestAuthorizationUsesCredentialsFileAndReloadsIt(t *testing.T) {
	credentialsFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(credentialsFile, []byte("token-one\n"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	received := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, time.Second, HTTPConfig{
		Authorization: &AuthorizationConfig{CredentialsFile: credentialsFile},
	}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request with authorization failed: %v", err)
	}
	if got := <-received; got != "Bearer token-one" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer token-one")
	}

	if err := os.WriteFile(credentialsFile, []byte("token-two\n"), 0o600); err != nil {
		t.Fatalf("rotate credentials file: %v", err)
	}
	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request after credential rotation failed: %v", err)
	}
	if got := <-received; got != "Bearer token-two" {
		t.Fatalf("Authorization header after rotation = %q, want %q", got, "Bearer token-two")
	}
}

func TestAuthenticationIsNotForwardedAcrossRedirectOrigins(t *testing.T) {
	directory := t.TempDir()
	credentialsFile := filepath.Join(directory, "token")
	usernameFile := filepath.Join(directory, "username")
	passwordFile := filepath.Join(directory, "password")
	if err := os.WriteFile(credentialsFile, []byte("redirect-secret"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	if err := os.WriteFile(usernameFile, []byte("metrics-user"), 0o600); err != nil {
		t.Fatalf("write username file: %v", err)
	}
	if err := os.WriteFile(passwordFile, []byte("redirect-password"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	destinationRequests := make(chan string, 1)
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		destinationRequests <- req.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(destination.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, destination.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	tests := []struct {
		name   string
		config HTTPConfig
	}{
		{
			name: "authorization",
			config: HTTPConfig{
				Authorization: &AuthorizationConfig{CredentialsFile: credentialsFile},
			},
		},
		{
			name: "basic auth",
			config: HTTPConfig{
				BasicAuth: &BasicAuthConfig{UsernameFile: usernameFile, PasswordFile: passwordFile},
			},
		},
		{name: "legacy authorization", config: HTTPConfig{LegacyAuthorization: "Bearer legacy-secret"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(redirector.URL, time.Second, tt.config, log.DefaultLogger)
			if err != nil {
				t.Fatalf("create client: %v", err)
			}
			err = request(t, client, redirector.URL)
			select {
			case authorization := <-destinationRequests:
				t.Errorf("cross-origin redirect reached the destination with Authorization %q", authorization)
			default:
			}
			if err == nil || !strings.Contains(err.Error(), "refusing to send Prometheus credentials to a different origin") {
				t.Fatalf("cross-origin redirect error = %v, want credential boundary rejection", err)
			}
		})
	}
}

func TestAuthenticationAllowsRedirectsWithinConfiguredOrigin(t *testing.T) {
	credentialsFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(credentialsFile, []byte("same-origin-secret"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/redirect" {
			http.Redirect(w, req, "/final", http.StatusFound)
			return
		}
		received <- req.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, time.Second, HTTPConfig{
		Authorization: &AuthorizationConfig{CredentialsFile: credentialsFile},
	}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := request(t, client, server.URL+"/redirect"); err != nil {
		t.Fatalf("same-origin redirect failed: %v", err)
	}
	if got := <-received; got != "Bearer same-origin-secret" {
		t.Fatalf("Authorization header after same-origin redirect = %q", got)
	}
}

func TestSameOriginUsesSchemeHostAndEffectivePort(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{name: "same HTTPS origin", left: "https://prometheus.example", right: "https://PROMETHEUS.example:443/path", want: true},
		{name: "same HTTP origin", left: "http://prometheus.example", right: "http://prometheus.example:80/path", want: true},
		{name: "scheme downgrade", left: "https://prometheus.example", right: "http://prometheus.example", want: false},
		{name: "different port", left: "https://prometheus.example", right: "https://prometheus.example:9090", want: false},
		{name: "subdomain", left: "https://prometheus.example", right: "https://redirect.prometheus.example", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left, err := url.Parse(tt.left)
			if err != nil {
				t.Fatalf("parse left URL: %v", err)
			}
			right, err := url.Parse(tt.right)
			if err != nil {
				t.Fatalf("parse right URL: %v", err)
			}
			if got := sameOrigin(left, right); got != tt.want {
				t.Fatalf("sameOrigin(%q, %q) = %t, want %t", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestBasicAuthUsesCredentialFilesAndReloadsThem(t *testing.T) {
	directory := t.TempDir()
	usernameFile := filepath.Join(directory, "username")
	passwordFile := filepath.Join(directory, "password")
	if err := os.WriteFile(usernameFile, []byte("metrics-user\n"), 0o600); err != nil {
		t.Fatalf("write username file: %v", err)
	}
	if err := os.WriteFile(passwordFile, []byte("p@ss:word\n"), 0o600); err != nil {
		t.Fatalf("write password file: %v", err)
	}
	type credentials struct {
		username string
		password string
		ok       bool
	}
	received := make(chan credentials, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username, password, ok := req.BasicAuth()
		received <- credentials{username: username, password: password, ok: ok}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, time.Second, HTTPConfig{
		BasicAuth: &BasicAuthConfig{
			UsernameFile: usernameFile,
			PasswordFile: passwordFile,
		},
	}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request with basic auth failed: %v", err)
	}
	if got := <-received; !got.ok || got.username != "metrics-user" || got.password != "p@ss:word" {
		t.Fatalf("basic auth credentials = %#v, want metrics-user/p@ss:word", got)
	}

	if err := os.WriteFile(usernameFile, []byte("rotated-user\n"), 0o600); err != nil {
		t.Fatalf("rotate username file: %v", err)
	}
	if err := os.WriteFile(passwordFile, []byte("rotated-password\n"), 0o600); err != nil {
		t.Fatalf("rotate password file: %v", err)
	}
	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("request after basic auth rotation failed: %v", err)
	}
	if got := <-received; !got.ok || got.username != "rotated-user" || got.password != "rotated-password" {
		t.Fatalf("basic auth credentials after rotation = %#v, want rotated-user/rotated-password", got)
	}
}

func TestAuthenticationComposesWithTLS(t *testing.T) {
	credentialsFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(credentialsFile, []byte("secure-token"), 0o600); err != nil {
		t.Fatalf("write credentials file: %v", err)
	}
	received := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		received <- req.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	server.Config.ErrorLog = stdlog.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	caFile := filepath.Join(t.TempDir(), "ca.crt")
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, certificate, 0o600); err != nil {
		t.Fatalf("write CA file: %v", err)
	}
	client, err := NewClient(server.URL, time.Second, HTTPConfig{
		TLS:           TLSConfig{CAFile: caFile},
		Authorization: &AuthorizationConfig{CredentialsFile: credentialsFile},
	}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	if err := request(t, client, server.URL); err != nil {
		t.Fatalf("authenticated TLS request failed: %v", err)
	}
	if got := <-received; got != "Bearer secure-token" {
		t.Fatalf("Authorization header = %q, want %q", got, "Bearer secure-token")
	}
}

func TestAuthenticationConfigurationIsValidated(t *testing.T) {
	tests := []struct {
		name      string
		config    HTTPConfig
		wantError string
	}{
		{
			name: "legacy and authorization",
			config: HTTPConfig{
				LegacyAuthorization: "Bearer legacy",
				Authorization:       &AuthorizationConfig{CredentialsFile: "/credentials"},
			},
			wantError: "legacy Prometheus auth cannot be combined",
		},
		{
			name: "authorization and basic auth",
			config: HTTPConfig{
				Authorization: &AuthorizationConfig{CredentialsFile: "/credentials"},
				BasicAuth: &BasicAuthConfig{
					UsernameFile: "/username",
					PasswordFile: "/password",
				},
			},
			wantError: "mutually exclusive",
		},
		{
			name:      "authorization without credentials file",
			config:    HTTPConfig{Authorization: &AuthorizationConfig{}},
			wantError: "authorization credentials file is required",
		},
		{
			name:      "basic auth without both files",
			config:    HTTPConfig{BasicAuth: &BasicAuthConfig{UsernameFile: "/username"}},
			wantError: "username and password files are required",
		},
		{
			name: "basic authorization scheme",
			config: HTTPConfig{Authorization: &AuthorizationConfig{
				Type:            "Basic",
				CredentialsFile: "/credentials",
			}},
			wantError: `authorization type cannot be set to "basic"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient("http://prometheus.example", time.Second, tt.config, log.DefaultLogger)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("NewClient error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestPrometheusAddressRejectsUserInformationWithoutExposingIt(t *testing.T) {
	const password = "super-secret"
	_, err := NewClient("https://metrics-user:"+password+"@prometheus.example", time.Second, HTTPConfig{}, log.DefaultLogger)
	if err == nil {
		t.Fatal("NewClient accepted an address with user information")
	}
	if strings.Contains(err.Error(), password) {
		t.Fatalf("NewClient error exposed the password: %v", err)
	}
	if !strings.Contains(err.Error(), "must not include user information") {
		t.Fatalf("NewClient error = %v, want user-information guidance", err)
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
	client, err := NewClient(address, time.Second, HTTPConfig{TLS: tlsConfig}, log.NewStdLogger(io.Discard))
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

func TestQueryTimesOutWhenPrometheusNeverResponds(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		select {
		case <-release:
		case <-req.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	client, err := NewClient(server.URL, 100*time.Millisecond, HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}

	for name, query := range map[string]func() error{
		"instant": func() error {
			_, err := client.Query(context.Background(), "up")
			return err
		},
		"range": func() error {
			_, err := client.QueryRange(context.Background(), "up", v1.Range{Start: time.Unix(0, 0), End: time.Unix(60, 0), Step: time.Minute})
			return err
		},
	} {
		done := make(chan error, 1)
		go func() { done <- query() }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("%s query against a silent Prometheus succeeded", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s query was not bounded by the client timeout", name)
		}
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
	client, err := NewClient(address, time.Second, HTTPConfig{TLS: tlsConfig}, logger)
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

func TestClientEnforcesConcurrencyLimitAndHonorsContextCancellation(t *testing.T) {
	var inFlight, maxSeen atomic.Int32
	entered := make(chan struct{}, 8)
	release := make(chan struct{})
	server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
		cur := inFlight.Add(1)
		for {
			prev := maxSeen.Load()
			if cur <= prev || maxSeen.CompareAndSwap(prev, cur) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-req.Context().Done():
		}
		inFlight.Add(-1)
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	})
	client := newTestClient(t, server.URL, TLSConfig{})

	errs := make(chan error, 5)
	for i := 0; i < 4; i++ {
		go func() {
			_, err := client.Query(context.Background(), "up")
			errs <- err
		}()
	}
	for i := 0; i < 4; i++ {
		<-entered
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	waiterErr := make(chan error, 1)
	go func() {
		_, err := client.QueryRange(canceledCtx, "up", v1.Range{Start: time.Unix(0, 0), End: time.Unix(60, 0), Step: time.Minute})
		waiterErr <- err
	}()
	select {
	case <-entered:
		t.Fatal("5th query bypassed concurrency cap of 4")
	case <-time.After(30 * time.Millisecond):
	}
	cancel()
	if err := <-waiterErr; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
	}

	close(release)
	for i := 0; i < 4; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("admitted query error = %v", err)
		}
	}
	if maxSeen.Load() != 4 {
		t.Fatalf("max concurrent queries = %d, want 4", maxSeen.Load())
	}
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("canceled waiter tripped breaker: %v", err)
	}
}

func TestClientRejectsOversizedQueryResponseWithoutOpeningCircuit(t *testing.T) {
	exactPayload := `{"status":"success","data":{"resultType":"vector","result":[]}}`
	oversized := exactPayload + strings.Repeat(" ", 128)
	var mode atomic.Int32 // 0=chunked oversized, 1=content-length oversized, 2=exact within limit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/api/v1/targets" {
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"activeTargets":[{"discoveredLabels":{"job":"`+strings.Repeat("a", 256)+`"},"scrapeUrl":"http://target:9100/metrics","health":"up"}]}}`)
			return
		}
		switch mode.Load() {
		case 0:
			if flusher, ok := w.(http.Flusher); ok {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, oversized[:32])
				flusher.Flush()
				_, _ = fmt.Fprint(w, oversized[32:])
				return
			}
			_, _ = fmt.Fprint(w, oversized)
		case 1:
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(oversized)))
			_, _ = fmt.Fprint(w, oversized)
		default:
			_, _ = fmt.Fprint(w, exactPayload)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, TLSConfig{})
	client.queryClient, _ = newLimitedAPIClient(server.URL, time.Second, int64(len(exactPayload)))

	for m := int32(0); m < 2; m++ {
		mode.Store(m)
		for i := 0; i < 4; i++ {
			if _, err := client.Query(context.Background(), "up"); !errors.Is(err, ErrResponseTooLarge) {
				t.Fatalf("mode %d attempt %d error = %v, want ErrResponseTooLarge", m, i, err)
			}
		}
	}

	mode.Store(2)
	if _, err := client.Query(context.Background(), "up"); err != nil {
		t.Fatalf("oversized responses wrongly opened circuit: %v", err)
	}

	var count int
	if err := client.WalkActiveTargets(context.Background(), func(v1.ActiveTarget) { count++ }); err != nil || count != 1 {
		t.Fatalf("WalkActiveTargets should stay exempt from query byte limit: count=%d err=%v", count, err)
	}

	mode.Store(1)
	conn, err := client.Conn()
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/v1/query?query=up", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if _, body, err := conn.Do(context.Background(), req); err != nil || len(body) <= len(exactPayload) {
		t.Fatalf("Conn().Do should stay exempt from query byte limit: len=%d err=%v", len(body), err)
	}
}

func newLimitedAPIClient(address string, timeout time.Duration, limit int64) (api.Client, error) {
	return api.NewClient(api.Config{
		Address: address,
		Client: &http.Client{
			Transport: &responseLimitRoundTripper{limit: limit, next: http.DefaultTransport},
			Timeout:   timeout,
		},
	})
}

func TestClientCircuitBreakerOpensAfterThreeUpstreamFailuresAndRecoversWithSingleProbe(t *testing.T) {
	var failCode atomic.Int32
	var hits atomic.Int32
	failCode.Store(http.StatusServiceUnavailable)
	server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
		hits.Add(1)
		switch code := failCode.Load(); code {
		case http.StatusBadRequest:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"parse error"}`)
		case http.StatusServiceUnavailable:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "unavailable")
		default:
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		}
	})
	now := time.Unix(1760000000, 0)
	client := newTestClient(t, server.URL, TLSConfig{})
	client.now = func() time.Time { return now }
	client.cooldown = 15 * time.Second

	// 4xx bad_data must not count toward breaker failure threshold.
	failCode.Store(http.StatusBadRequest)
	for i := 0; i < 4; i++ {
		if _, err := client.Query(context.Background(), "bad("); err == nil {
			t.Fatal("expected bad_data error")
		}
	}

	// Two 503 failures keep breaker closed; a third opens it.
	failCode.Store(http.StatusServiceUnavailable)
	for i := 0; i < 2; i++ {
		if _, err := client.Query(context.Background(), "up"); err == nil || errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("attempt %d unexpectedly opened circuit: %v", i+1, err)
		}
	}
	if _, err := client.Query(context.Background(), "up"); err == nil || errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("3rd failure should return upstream error, got %v", err)
	}

	beforeOpenHits := hits.Load()
	if _, err := client.Query(context.Background(), "up"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("4th call while open = %v, want ErrCircuitOpen", err)
	}
	if hits.Load() != beforeOpenHits {
		t.Fatalf("open breaker still hit Prometheus (%d -> %d)", beforeOpenHits, hits.Load())
	}

	// After cooldown, only one half-open probe is admitted; concurrent callers get ErrCircuitOpen.
	now = now.Add(15 * time.Second)
	probeStarted := make(chan struct{}, 1)
	releaseProbe := make(chan struct{})
	probeServer := newPrometheusAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		probeStarted <- struct{}{}
		<-releaseProbe
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	})
	probeClient := newTestClient(t, probeServer.URL, TLSConfig{})
	probeClient.now = func() time.Time { return now }
	probeClient.cooldown = 15 * time.Second
	probeClient.circuit = circuitOpen
	probeClient.openUntil = now

	probeDone := make(chan error, 1)
	go func() {
		_, err := probeClient.Query(context.Background(), "up")
		probeDone <- err
	}()
	<-probeStarted
	if _, err := probeClient.Query(context.Background(), "up"); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("concurrent caller during half-open probe = %v, want ErrCircuitOpen", err)
	}
	close(releaseProbe)
	if err := <-probeDone; err != nil {
		t.Fatalf("successful probe error = %v", err)
	}
	if _, err := probeClient.Query(context.Background(), "up"); err != nil {
		t.Fatalf("breaker did not close after successful probe: %v", err)
	}
}

func TestClientCircuitBreakerConcurrencyInvariants(t *testing.T) {
	t.Run("queued callers fail fast after open and stale success preserves cooldown", func(t *testing.T) {
		var mode atomic.Int32 // 0=pre-seed 503, 1=hold 3 OKs + 1 fail, 2=should not be reached
		var serverHits atomic.Int32
		failStarted := make(chan struct{}, 1)
		releaseFail := make(chan struct{})
		okStarted := make(chan struct{}, 3)
		releaseOK := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-releaseFail:
			default:
				close(releaseFail)
			}
			select {
			case <-releaseOK:
			default:
				close(releaseOK)
			}
		})
		server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
			serverHits.Add(1)
			switch mode.Load() {
			case 0:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, "unavailable")
			case 1:
				if strings.Contains(req.FormValue("query"), "fail3") {
					failStarted <- struct{}{}
					select {
					case <-releaseFail:
					case <-req.Context().Done():
					}
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = fmt.Fprint(w, "unavailable")
					return
				}
				okStarted <- struct{}{}
				select {
				case <-releaseOK:
				case <-req.Context().Done():
				}
				_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			}
		})
		now := time.Unix(1760000000, 0)
		client := newTestClient(t, server.URL, TLSConfig{})
		client.now = func() time.Time { return now }
		client.cooldown = 15 * time.Second

		// Seed 2 failures while closed (threshold = 3).
		for i := 0; i < 2; i++ {
			if _, err := client.Query(context.Background(), "up"); err == nil {
				t.Fatal("expected upstream 503 failure")
			}
		}

		// Saturate all 4 semaphore permits: 3 slow 200 OKs + 1 slow 3rd failure.
		mode.Store(1)
		okErrs := make(chan error, 3)
		for i := 0; i < 3; i++ {
			go func() {
				_, err := client.Query(context.Background(), "up")
				okErrs <- err
			}()
		}
		for i := 0; i < 3; i++ {
			<-okStarted
		}
		fail3Err := make(chan error, 1)
		go func() {
			_, err := client.Query(context.Background(), "fail3")
			fail3Err <- err
		}()
		<-failStarted

		// 5th call queues on c.sem before circuit opens.
		waiterDone := make(chan error, 1)
		go func() {
			_, err := client.Query(context.Background(), "waiter")
			waiterDone <- err
		}()
		time.Sleep(25 * time.Millisecond)

		// Release only the 3rd failure; it opens the circuit BEFORE releasing its single permit to waiter.
		hitsBeforeRelease := serverHits.Load()
		close(releaseFail)
		if err := <-fail3Err; err == nil {
			t.Fatal("expected 3rd upstream 503 failure")
		}
		if err := <-waiterDone; !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("queued waiter after circuit opened = %v, want ErrCircuitOpen", err)
		}
		if serverHits.Load() != hitsBeforeRelease {
			t.Fatalf("queued waiter hit Prometheus (%d -> %d)", hitsBeforeRelease, serverHits.Load())
		}

		// Stale in-flight 200 OK calls finish; cooldown must still hold.
		close(releaseOK)
		for i := 0; i < 3; i++ {
			if err := <-okErrs; err != nil {
				t.Fatalf("stale in-flight success error = %v", err)
			}
		}
		hitsBefore := serverHits.Load()
		if _, err := client.Query(context.Background(), "up"); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("stale in-flight success cleared breaker cooldown: err=%v", err)
		}
		if serverHits.Load() != hitsBefore {
			t.Fatalf("open breaker still hit Prometheus (%d -> %d)", hitsBefore, serverHits.Load())
		}
	})

	t.Run("stale success during half-open probe does not close circuit", func(t *testing.T) {
		var seq atomic.Int32
		staleStarted := make(chan struct{}, 1)
		releaseStale := make(chan struct{})
		probeStarted := make(chan struct{}, 1)
		releaseProbe := make(chan struct{})
		t.Cleanup(func() {
			select {
			case <-releaseStale:
			default:
				close(releaseStale)
			}
			select {
			case <-releaseProbe:
			default:
				close(releaseProbe)
			}
		})
		server := newPrometheusAPIServer(t, func(w http.ResponseWriter, req *http.Request) {
			if seq.Add(1) == 1 {
				staleStarted <- struct{}{}
				select {
				case <-releaseStale:
				case <-req.Context().Done():
				}
				_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
				return
			}
			probeStarted <- struct{}{}
			select {
			case <-releaseProbe:
			case <-req.Context().Done():
			}
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		})
		now := time.Unix(1760000000, 0)
		var mu sync.Mutex
		client := newTestClient(t, server.URL, TLSConfig{})
		client.now = func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		}
		client.cooldown = 15 * time.Second

		staleDone := make(chan error, 1)
		go func() {
			_, err := client.Query(context.Background(), "up")
			staleDone <- err
		}()
		<-staleStarted

		// Simulate circuit opening after other concurrent failures while the 1st call stayed in flight.
		client.circuitMu.Lock()
		client.circuit = circuitOpen
		client.circuitEpoch++
		client.openUntil = now.Add(15 * time.Second)
		client.circuitMu.Unlock()

		mu.Lock()
		now = now.Add(15 * time.Second)
		mu.Unlock()

		probeDone := make(chan error, 1)
		go func() {
			_, err := client.Query(context.Background(), "up")
			probeDone <- err
		}()
		<-probeStarted

		// Stale in-flight request completes with 200 OK while half-open probe is active.
		close(releaseStale)
		if err := <-staleDone; err != nil {
			t.Fatalf("stale call error = %v", err)
		}
		// Single half-open probe is still running, so another concurrent caller must be rejected.
		if _, err := client.Query(context.Background(), "up"); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("stale success prematurely closed half-open probe: err=%v", err)
		}

		close(releaseProbe)
		if err := <-probeDone; err != nil {
			t.Fatalf("probe error = %v", err)
		}
	})

	t.Run("pre-expired deadlines do not hit upstream or open circuit", func(t *testing.T) {
		var hits atomic.Int32
		server := newPrometheusAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		})
		client := newTestClient(t, server.URL, TLSConfig{})
		for i := 0; i < 4; i++ {
			expiredCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			_, err := client.Query(expiredCtx, "up")
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("attempt %d error = %v, want DeadlineExceeded", i, err)
			}
		}
		if hits.Load() != 0 {
			t.Fatalf("pre-expired contexts hit Prometheus %d times", hits.Load())
		}
		if _, err := client.Query(context.Background(), "up"); err != nil {
			t.Fatalf("pre-expired contexts opened circuit: %v", err)
		}
	})
}
