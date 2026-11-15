package v1

import (
	"encoding/json"
	"testing"

	kratosjson "github.com/go-kratos/kratos/v2/encoding/json"

	observability "vgpu/api/observability/v1"
)

func TestNodeInventoryCoverageJSON(t *testing.T) {
	for _, tc := range []struct {
		name     string
		coverage *observability.Coverage
	}{
		{"default", nil},
		{"inventory", &observability.Coverage{Entity: "device"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply := &NodesReply{List: []*NodeReply{{InventoryCoverage: tc.coverage}}}
			if tc.coverage != nil {
				reply.InventoryCoverage = &observability.Coverage{Entity: "node"}
			}
			data, err := kratosjson.MarshalOptions.Marshal(reply)
			if err != nil {
				t.Fatal(err)
			}
			// RawMessage distinguishes an absent field from an explicit JSON null.
			var wire struct {
				InventoryCoverage json.RawMessage
				List              []struct{ InventoryCoverage json.RawMessage }
			}
			if err := json.Unmarshal(data, &wire); err != nil || len(wire.List) != 1 {
				t.Fatalf("invalid node list %s: %v", data, err)
			}
			for entity, raw := range map[string]json.RawMessage{"node": wire.InventoryCoverage, "device": wire.List[0].InventoryCoverage} {
				if tc.coverage == nil {
					if raw != nil {
						t.Errorf("default %s coverage = %s, want field omitted", entity, raw)
					}
					continue
				}
				var coverage struct{ Entity string }
				if err := json.Unmarshal(raw, &coverage); err != nil || coverage.Entity != entity {
					t.Errorf("%s coverage = %s, error = %v", entity, raw, err)
				}
			}
		})
	}
}
