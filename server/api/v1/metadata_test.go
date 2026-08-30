package v1

import (
	"testing"

	openapiv2 "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2/options"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestPublicOperationSummaries(t *testing.T) {
	want := map[protoreflect.FullName]string{
		"api.v1.Card.GetAllGPUs":            "List accelerators",
		"api.v1.Card.GetAllGPUTypes":        "List accelerator types",
		"api.v1.Card.GetGPU":                "Get accelerator details",
		"api.v1.Container.GetAllContainers": "List workloads",
		"api.v1.Container.GetContainer":     "Get workload details",
		"api.v1.Node.GetSummary":            "Get cluster resource summary",
		"api.v1.Node.GetAllNodes":           "List nodes",
		"api.v1.Node.GetNode":               "Get node details",
		"api.v1.Monitor.QueryRange":         "Query range vector",
		"api.v1.Monitor.QueryInstant":       "Query instant vector",
		"api.v1.Monitor.Summary":            "Get resource usage summary",
	}

	files := []protoreflect.FileDescriptor{
		File_api_v1_card_proto,
		File_api_v1_container_proto,
		File_api_v1_node_proto,
		File_api_v1_monitor_proto,
	}

	for _, file := range files {
		services := file.Services()
		for serviceIndex := 0; serviceIndex < services.Len(); serviceIndex++ {
			methods := services.Get(serviceIndex).Methods()
			for methodIndex := 0; methodIndex < methods.Len(); methodIndex++ {
				method := methods.Get(methodIndex)
				wantSummary, ok := want[method.FullName()]
				if !ok {
					continue
				}
				if got := openAPISummary(t, method); got != wantSummary {
					t.Errorf("%s summary = %q, want %q", method.FullName(), got, wantSummary)
				}
				delete(want, method.FullName())
			}
		}
	}

	for method, summary := range want {
		t.Errorf("operation %s with summary %q was not generated", method, summary)
	}
}

func openAPISummary(t *testing.T, method protoreflect.MethodDescriptor) string {
	t.Helper()
	options, ok := method.Options().(*descriptorpb.MethodOptions)
	if !ok || !proto.HasExtension(options, openapiv2.E_Openapiv2Operation) {
		t.Fatalf("%s has no OpenAPI operation metadata", method.FullName())
	}
	operation, ok := proto.GetExtension(options, openapiv2.E_Openapiv2Operation).(*openapiv2.Operation)
	if !ok {
		t.Fatalf("%s has invalid OpenAPI operation metadata", method.FullName())
	}
	return operation.GetSummary()
}
