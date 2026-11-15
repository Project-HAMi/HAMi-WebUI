package service

import (
	"slices"

	"google.golang.org/protobuf/proto"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

// A partial source can still account for every field in the rows it returned.
func newOverviewCoverage(base *pb.Coverage, entity string, incompleteIdentity bool) *pb.Coverage {
	c := &pb.Coverage{Entity: entity, Complete: base.GetComplete(), Sources: base.GetSources()}
	if incompleteIdentity {
		c.Complete = false
		c.Sources = append(slices.Clone(c.Sources), &pb.SourceState{Source: "overview_identity", State: resourceview.Unknown, Reason: "unverified"})
	}
	if c.Complete {
		c.Total = proto.Int32(0)
	}
	return c
}

func countRow(c *pb.Coverage, states map[string]*pb.FieldState) {
	c.Observed++
	if c.Total != nil {
		*c.Total = c.Observed
	}
	if len(states) == 0 {
		return
	}
	if c.Fields == nil {
		c.Fields = make(map[string]*pb.FieldCoverage, len(states))
	}
	for name, state := range states {
		count := c.Fields[name]
		if count == nil {
			count = &pb.FieldCoverage{}
			c.Fields[name] = count
		}
		countFieldState(count, state)
	}
}

func countFieldState(count *pb.FieldCoverage, state *pb.FieldState) {
	switch state.GetState() {
	case resourceview.Known:
		count.Known++
		return
	case resourceview.Stale:
		count.Stale++
	case resourceview.Conflict:
		count.Conflict++
	case resourceview.NotApplicable:
		count.NotApplicable++
	default:
		count.Unknown++
	}
	reason := state.GetReason()
	if reason == "" {
		reason = "unverified"
	}
	if count.Reasons == nil {
		count.Reasons = make(map[string]int32)
	}
	count.Reasons[reason]++
}
