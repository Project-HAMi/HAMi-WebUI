package resourceview

import (
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/api/resource"

	viewv1 "vgpu/api/observability/v1"
)

// Quantity preserves exact source units and optional value presence. The state
// is the shared API evidence contract, not another provider-specific enum.
type Quantity struct {
	Value *resource.Quantity
	Unit  string
	State *viewv1.FieldState
}

func (q Quantity) DeepCopy() Quantity {
	if q.Value != nil {
		value := q.Value.DeepCopy()
		q.Value = &value
	}
	if q.State != nil {
		q.State = proto.Clone(q.State).(*viewv1.FieldState)
	}
	return q
}
