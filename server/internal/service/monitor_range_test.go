package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/data/prom"

	kratosjson "github.com/go-kratos/kratos/v2/encoding/json"
	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func TestQueryRangeHTTPTimezones(t *testing.T) {
	// A process per timezone avoids changing time.Local while HTTP transport
	// goroutines from another case are still running.
	zone := os.Getenv("HAMI_RANGE_TEST_TIMEZONE")
	if zone == "" {
		for _, zone := range []string{"UTC", "Asia/Shanghai"} {
			t.Run(zone, func(t *testing.T) {
				command := exec.Command(os.Args[0], "-test.run=^TestQueryRangeHTTPTimezones$", "-test.v")
				command.Env = append(os.Environ(), "TZ="+zone, "HAMI_RANGE_TEST_TIMEZONE="+zone)
				output, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("server timezone %s: %v\n%s", zone, err, output)
				}
				t.Logf("%s", output)
			})
		}
		return
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	if time.Local.String() != zone {
		t.Fatalf("server timezone = %s, want %s", time.Local, zone)
	}
	handler, requests := newRangeHTTPHandler(t)
	utcStart := time.Date(2026, 10, 7, 4, 0, 0, 0, time.UTC)
	dstStart := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	legacyStart := time.Date(2026, 10, 7, 12, 0, 0, 0, location)
	tests := []struct {
		name      string
		start     string
		end       string
		wantStart time.Time
	}{
		{"UTC", "2026-10-07T04:00:00Z", "2026-10-07T04:01:00Z", utcStart},
		{"positive offset", "2026-10-07T12:00:00+08:00", "2026-10-07T12:01:00+08:00", utcStart},
		{"negative offset", "2026-10-07T00:00:00-04:00", "2026-10-07T00:01:00-04:00", utcStart},
		{"fractional seconds", "2026-10-07T04:00:00.125Z", "2026-10-07T04:01:00.125Z", utcStart.Add(125 * time.Millisecond)},
		{"DST repeated hour first occurrence", "2026-11-01T01:30:00-04:00", "2026-11-01T01:31:00-04:00", dstStart},
		{"DST repeated hour second occurrence", "2026-11-01T01:30:00-05:00", "2026-11-01T01:31:00-05:00", dstStart.Add(time.Hour)},
		{"DST rollback crossing", "2026-11-01T01:59:30-04:00", "2026-11-01T01:00:30-05:00", time.Date(2026, 11, 1, 5, 59, 30, 0, time.UTC)},
		{"legacy server local", "2026-10-07 12:00:00", "2026-10-07 12:01:00", legacyStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := queryRangeHTTP(t, handler, tt.start, tt.end, "1m")
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
			}
			var request url.Values
			select {
			case request = <-requests:
			default:
				t.Fatal("Prometheus received no range request")
			}
			for key, want := range map[string]time.Time{"start": tt.wantStart, "end": tt.wantStart.Add(time.Minute)} {
				got, err := strconv.ParseFloat(request.Get(key), 64)
				if err != nil || got != float64(want.UnixMilli())/1000 {
					t.Errorf("Prometheus %s = %q, want instant %s", key, request.Get(key), want.UTC().Format(time.RFC3339Nano))
				}
			}
			if request.Get("query") != "up" || request.Get("step") != "60" {
				t.Errorf("Prometheus query/step = %q/%q, want up/60", request.Get("query"), request.Get("step"))
			}
			var result pb.RangeResponse
			if err := kratosjson.UnmarshalOptions.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode range response: %v", err)
			}
			if len(result.Data) != 1 {
				t.Fatalf("series count = %d, want 1", len(result.Data))
			}
			assertSamplePairs(t, result.Data[0].Values, []*pb.SamplePair{
				{Timestamp: tt.wantStart.UnixMilli(), Value: 0},
				{Timestamp: tt.wantStart.Add(time.Minute).UnixMilli(), Missing: true},
			})
		})
	}
}

func TestQueryRangeHTTPRejectsInvalidInput(t *testing.T) {
	handler, requests := newRangeHTTPHandler(t)
	tests := []struct {
		name  string
		start string
		end   string
		step  string
	}{
		{"invalid start", "not-a-time", "2026-10-07T04:01:00Z", "1m"},
		{"invalid end", "2026-10-07T04:00:00Z", "not-a-time", "1m"},
		{"missing timezone", "2026-10-07T04:00:00", "2026-10-07T04:01:00Z", "1m"},
		{"invalid offset", "2026-10-07T04:00:00+25:00", "2026-10-07T04:01:00Z", "1m"},
		{"invalid step", "2026-10-07T04:00:00Z", "2026-10-07T04:01:00Z", "not-a-duration"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := queryRangeHTTP(t, handler, tt.start, tt.end, tt.step)
			var result struct {
				Code   int    `json:"code"`
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Code != 522 || result.Code != 522 || result.Reason != "TRANSFORM_ERROR" {
				t.Errorf("response = %d %s, want existing 522 TRANSFORM_ERROR", response.Code, response.Body.String())
			}
			select {
			case request := <-requests:
				t.Errorf("invalid input reached Prometheus: %v", request)
			default:
			}
		})
	}
}

func newRangeHTTPHandler(t *testing.T) (http.Handler, <-chan url.Values) {
	t.Helper()
	requests := make(chan url.Values, 1)
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			http.Error(w, "unexpected Prometheus path", http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse Prometheus request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"job":"test"},"values":[[%s,"0"]]}]}}`, r.Form.Get("start"))
	}))
	t.Cleanup(prometheus.Close)
	client, err := prom.NewClient(prometheus.URL, time.Second, prom.HTTPConfig{}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatalf("create Prometheus client: %v", err)
	}
	handler := kratoshttp.NewServer()
	pb.RegisterMonitorHTTPServer(handler, NewMonitorService(client, nil, nil))
	return handler, requests
}

func queryRangeHTTP(t *testing.T, handler http.Handler, start, end, step string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(&pb.QueryRangeRequest{Query: "up", Range: &pb.Range{Start: start, End: end, Step: step}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/monitor/query/range-vector", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
