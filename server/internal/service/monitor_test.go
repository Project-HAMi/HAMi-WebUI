package service

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/data/prom"

	kratosjson "github.com/go-kratos/kratos/v2/encoding/json"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

func TestSamplePairJSONKeepsZeroDistinctFromMissing(t *testing.T) {
	tests := []struct {
		name    string
		pair    *pb.SamplePair
		missing bool
	}{
		{name: "real zero", pair: &pb.SamplePair{Value: 0, Timestamp: 1_000}},
		{name: "missing slot", pair: &pb.SamplePair{Value: 0, Timestamp: 2_000, Missing: true}, missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := kratosjson.MarshalOptions.Marshal(tt.pair)
			if err != nil {
				t.Fatalf("marshal sample pair: %v", err)
			}

			var fields map[string]any
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatalf("decode sample pair JSON: %v", err)
			}
			if value, ok := fields["value"]; !ok || value != float64(0) {
				t.Fatalf("value field = %#v (present %t), want an explicit zero", value, ok)
			}
			if missing, ok := fields["missing"].(bool); !ok || missing != tt.missing {
				t.Fatalf("missing field = %#v, want %t", fields["missing"], tt.missing)
			}
		})
	}
}

func TestFillLessSamplePointPreservesRealZeroAndMarksInternalGap(t *testing.T) {
	start := time.UnixMilli(1_000)
	end := time.UnixMilli(3_000)
	values := []*pb.SamplePair{
		{Value: 0, Timestamp: 1_000},
		{Value: 2, Timestamp: 3_000},
	}

	got := fillLessSamplePoint(start, end, time.Second, values)
	want := []*pb.SamplePair{
		{Value: 0, Timestamp: 1_000, Missing: false},
		{Value: 0, Timestamp: 2_000, Missing: true},
		{Value: 2, Timestamp: 3_000, Missing: false},
	}
	assertSamplePairs(t, got, want)
}

func TestFillLessSamplePointMarksLeadingAndTrailingGaps(t *testing.T) {
	start := time.UnixMilli(1_000)
	end := time.UnixMilli(5_000)
	values := []*pb.SamplePair{
		{Value: 7, Timestamp: 3_000},
	}

	got := fillLessSamplePoint(start, end, time.Second, values)
	want := []*pb.SamplePair{
		{Value: 0, Timestamp: 1_000, Missing: true},
		{Value: 0, Timestamp: 2_000, Missing: true},
		{Value: 7, Timestamp: 3_000, Missing: false},
		{Value: 0, Timestamp: 4_000, Missing: true},
		{Value: 0, Timestamp: 5_000, Missing: true},
	}
	assertSamplePairs(t, got, want)
}

func TestFillLessSamplePointKeepsVictoriaMetricsAlignment(t *testing.T) {
	start := time.UnixMilli(1_000)
	end := time.UnixMilli(3_500)
	values := []*pb.SamplePair{
		{Value: 0, Timestamp: 1_500},
		{Value: 9, Timestamp: 3_500},
	}

	got := fillLessSamplePoint(start, end, time.Second, values)
	want := []*pb.SamplePair{
		{Value: 0, Timestamp: 1_500, Missing: false},
		{Value: 0, Timestamp: 2_500, Missing: true},
		{Value: 9, Timestamp: 3_500, Missing: false},
	}
	assertSamplePairs(t, got, want)
}

func assertSamplePairs(t *testing.T, got, want []*pb.SamplePair) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sample count = %d, want %d: got %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].GetValue() != want[i].GetValue() ||
			got[i].GetTimestamp() != want[i].GetTimestamp() ||
			got[i].GetMissing() != want[i].GetMissing() {
			t.Errorf("sample %d = {value:%v timestamp:%d missing:%t}, want {value:%v timestamp:%d missing:%t}",
				i,
				got[i].GetValue(), got[i].GetTimestamp(), got[i].GetMissing(),
				want[i].GetValue(), want[i].GetTimestamp(), want[i].GetMissing(),
			)
		}
	}
}

func TestInstantQuerySendsRequestedTime(t *testing.T) {
	seen := make(chan string, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.FormValue("time")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer ts.Close()
	client, err := prom.NewClient(ts.URL, time.Second, prom.HTTPConfig{}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	s := NewMonitorService(client, nil, nil)
	if _, err := s.QueryInstant(context.Background(), &pb.QueryInstantRequest{Query: "up", Time: "2026-11-14T21:00:00.125+08:00"}); err != nil {
		t.Fatal(err)
	}
	expected, _ := time.Parse(time.RFC3339Nano, "2026-11-14T13:00:00.125Z")
	if actual := <-seen; actual != strconv.FormatFloat(float64(expected.UnixMilli())/1000, 'f', 3, 64) {
		t.Fatalf("unexpected evaluation time %q", actual)
	}
	if _, err := s.QueryInstant(context.Background(), &pb.QueryInstantRequest{Query: "up", Time: "yesterday"}); !errors.IsBadRequest(err) {
		t.Fatalf("invalid time returned %v, want HTTP 400", err)
	}
	if _, err := s.QueryInstant(context.Background(), &pb.QueryInstantRequest{Query: "up"}); err != nil {
		t.Fatal(err)
	}
	actual, err := strconv.ParseFloat(<-seen, 64)
	if err != nil || math.Abs(float64(time.Now().Unix())-actual) > 5 {
		t.Fatalf("legacy request time %v, err %v", actual, err)
	}
}
