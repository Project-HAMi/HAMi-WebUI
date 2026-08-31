package prom

import (
	"context"
	"encoding/pem"
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
