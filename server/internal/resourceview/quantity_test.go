package resourceview

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
)

func TestQuantityDeepCopyPreservesPresenceUnitsAndEvidence(t *testing.T) {
	zero := resource.MustParse("0")
	original := Quantity{Value: &zero, Unit: "bytes", State: &viewv1.FieldState{State: Known, Source: "registration", Params: map[string]string{"key": "capacity"}}}
	copy := original.DeepCopy()
	copy.Value.Add(resource.MustParse("1"))
	copy.State.Params["key"] = "other"
	if original.Value.Sign() != 0 || original.State.Params["key"] != "capacity" || copy.Unit != "bytes" {
		t.Fatal("copy aliases evidence or changes units")
	}
	absent := Quantity{State: &viewv1.FieldState{State: Unknown, Reason: "unverified"}}.DeepCopy()
	if absent.Value != nil || absent.State.State != Unknown {
		t.Fatal("missing quantity became zero")
	}
}
