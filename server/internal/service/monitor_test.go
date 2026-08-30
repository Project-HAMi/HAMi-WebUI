package service

import (
	"encoding/json"
	"testing"
	"time"

	pb "vgpu/api/v1"

	kratosjson "github.com/go-kratos/kratos/v2/encoding/json"
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
