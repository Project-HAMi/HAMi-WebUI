package service

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

type overviewSource resourceview.Snapshot

func (s overviewSource) Snapshot(context.Context) (resourceview.Snapshot, error) {
	return resourceview.Snapshot(s), nil
}

func testNode(name string, ready *bool) resourceview.Node {
	return resourceview.Node{Name: name, UID: types.UID(name + "-uid"), Ready: ready, Schedulable: proto.Bool(true),
		States: map[string]*pb.FieldState{"ready": observedState("status", "node"), "schedulable": observedState("spec", "node")}, DeviceCoverage: &pb.Coverage{Complete: true}}
}

func testCard(node resourceview.Node, provider, id, uuid, model string) resourceview.Device {
	known := observedState("registration", "device")
	return resourceview.Device{Identity: resourceview.DeviceIdentity{Provider: provider, Node: node.Name, NodeUID: node.UID, DeviceID: id},
		Kind: "physical", UUID: proto.String(uuid), Model: model, Healthy: proto.Bool(true), States: map[string]*pb.FieldState{"kind": known, "uuid": known, "model": known, "health": known}}
}

func testMetric(text, unit string) resourceview.Quantity {
	value := resource.MustParse(text)
	return resourceview.Quantity{Value: &value, Unit: unit, State: &pb.FieldState{State: "known", Source: "native", Scope: "device", ObservedAt: timestamppb.New(time.Unix(500, 0))}}
}

func observedState(source, scope string) *pb.FieldState {
	return &pb.FieldState{State: resourceview.Known, Source: source, Scope: scope}
}
