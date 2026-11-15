package resourceview

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/encoding"
	_ "github.com/go-kratos/kratos/v2/encoding/json"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	viewv1 "vgpu/api/observability/v1"
)

func TestFieldPresenceAndEvidenceRules(t *testing.T) {
	at := timestamppb.New(time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC))
	cases := []struct {
		name        string
		value       bool
		observation bool
		state       *viewv1.FieldState
		valid       bool
	}{
		{"known ledger zero", true, false, nil, true},
		{"missing ledger is not zero", false, false, nil, false},
		{"observation needs provenance", true, true, nil, false},
		{"observation sample time unknown", true, true, &viewv1.FieldState{State: Known, Source: "dcgm", Scope: "device"}, true},
		{"unknown ledger explained", false, false, &viewv1.FieldState{State: Unknown, Reason: "missing_config"}, true},
		{"unknown cannot retain value", true, false, &viewv1.FieldState{State: Unknown, Reason: "missing_config"}, false},
		{"conflict cannot become zero", true, false, &viewv1.FieldState{State: Conflict, Reason: "ambiguous"}, false},
		{"stale may retain sample", true, true, &viewv1.FieldState{State: Stale, Reason: "stale_data", Source: "dcgm", Scope: "device", ObservedAt: at}, true},
		{"stale may omit sample", false, true, &viewv1.FieldState{State: Stale, Reason: "stale_data", Source: "dcgm", Scope: "device", ObservedAt: at}, true},
		{"stale without time", true, true, &viewv1.FieldState{State: Stale, Reason: "stale_data", Source: "dcgm", Scope: "device"}, false},
		{"known must have a value", false, false, &viewv1.FieldState{State: Known}, false},
		{"known cannot carry failure", true, false, &viewv1.FieldState{State: Known, Reason: "no_permission"}, false},
		{"invalid state", false, false, &viewv1.FieldState{State: "incomplete", Reason: "no_permission"}, false},
		{"unknown observation scope", true, true, &viewv1.FieldState{State: Known, Source: "dcgm", Scope: "cluster"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateField(c.value, c.state, c.observation); (err == nil) != c.valid {
				t.Fatalf("valid=%v, error=%v", c.valid, err)
			}
		})
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -0.01} {
		if ValidateRatio(value) == nil {
			t.Fatalf("accepted invalid resource number %v", value)
		}
	}
	for _, value := range []float64{0, 0.25, 1, 1.25} {
		if err := ValidateRatio(value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIncompleteInventoryDoesNotBecomeConfirmedEmpty(t *testing.T) {
	incomplete := &viewv1.Coverage{Entity: "device", Observed: 0, Sources: []*viewv1.SourceState{
		{Source: "resource_slice", State: Unknown, Reason: "no_permission"},
	}}
	if err := ValidateCoverage(incomplete); err != nil {
		t.Fatal(err)
	}
	encoded, err := encoding.GetCodec("json").Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["total"]; exists {
		t.Fatalf("unavailable inventory claimed a total: %s", encoded)
	}
	if wire["complete"] != false || wire["observed"] != float64(0) {
		t.Fatal(string(encoded))
	}
	incomplete.Complete = true
	incomplete.Total = proto.Int32(0)
	if ValidateCoverage(incomplete) == nil {
		t.Fatal("unavailable source was reported as complete")
	}
	incomplete.Sources[0] = &viewv1.SourceState{Source: "resource_slice", State: Known}
	if err := ValidateCoverage(incomplete); err != nil {
		t.Fatal(err)
	}
}

func TestCoveragePartitionsObservedEntitiesWithoutOverflow(t *testing.T) {
	base := &viewv1.Coverage{Entity: "device", Observed: 3, Total: proto.Int32(3), Complete: true,
		Sources: []*viewv1.SourceState{{Source: "hami_registration", State: Known}},
		Fields:  map[string]*viewv1.FieldCoverage{"memoryUsedBytes": {Known: 1, Unknown: 1, Stale: 1, Reasons: map[string]int32{"metric_missing": 1, "stale_data": 1}}},
	}
	if err := ValidateCoverage(base); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*viewv1.Coverage)
	}{
		{"slots are not device total", func(c *viewv1.Coverage) { c.Total = proto.Int32(80) }},
		{"duplicate source", func(c *viewv1.Coverage) { c.Sources = append(c.Sources, c.Sources[0]) }},
		{"double counted field", func(c *viewv1.Coverage) { c.Fields["memoryUsedBytes"].Known = 2 }},
		{"invalid field entity", func(c *viewv1.Coverage) {
			c.Fields["memoryUsedBytes"].Entity = "typo"
			c.Fields["memoryUsedBytes"].Known = 999
		}},
		{"negative field", func(c *viewv1.Coverage) { c.Fields["memoryUsedBytes"].Known = -1 }},
		{"overflow", func(c *viewv1.Coverage) { c.Fields["memoryUsedBytes"].Known = math.MaxInt32 }},
		{"impossible reasons", func(c *viewv1.Coverage) { c.Fields["memoryUsedBytes"].Reasons["metric_missing"] = 2 }},
		{"missing reasons", func(c *viewv1.Coverage) { c.Fields["memoryUsedBytes"].Reasons = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			copy := proto.Clone(base).(*viewv1.Coverage)
			c.change(copy)
			if ValidateCoverage(copy) == nil {
				t.Fatal("accepted inconsistent coverage")
			}
		})
	}
}
