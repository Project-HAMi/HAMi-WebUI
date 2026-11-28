package server

import (
	"context"

	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/encoding/protojson"

	pb "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
	"vgpu/internal/service"
)

type overviewFixture struct{ complete bool }

func (s overviewFixture) Snapshot(context.Context) (resourceview.Snapshot, error) {
	return resourceview.Snapshot{NodeCoverage: &pb.Coverage{Complete: s.complete}, DeviceCoverage: &pb.Coverage{Complete: s.complete}}, nil
}
func newOverviewHTTPFixture(complete bool) nethttp.Handler {
	overview := service.NewOverviewService(resourceview.WithObservations(overviewFixture{complete: complete}, nil))
	return NewAPIHandler(nil, nil, nil, nil, nil, nil, overview, nil, nil, nil)
}

func TestOverviewHTTPContract(t *testing.T) {
	const route = "/api/v1/overview/inventory"
	for _, complete := range []bool{true, false} {
		h := newOverviewHTTPFixture(complete)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest("GET", route, nil))
		if response.Code != 200 {
			t.Fatalf("GET inventory: %d", response.Code)
		}
		var result pb.InventoryReply
		if err := protojson.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		for _, c := range []*pb.Coverage{result.NodeCoverage, result.DeviceCoverage} {
			if c == nil || c.Complete != complete || (c.Total != nil) != complete || c.GetTotal() != 0 {
				t.Fatalf("complete=%t, coverage=%v", complete, c)
			}
		}
		for _, tc := range []struct {
			method, path string
			want         int
		}{
			{"GET", route + "?pageSize=1", 400}, {"GET", route + "?bad=%zz", 400}, {"POST", route, 405},
		} {
			response := httptest.NewRecorder()
			h.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
			if response.Code != tc.want {
				t.Errorf("%s %s: %d", tc.method, tc.path, response.Code)
			}
		}
	}
}

func TestOverviewRoutesUseServerMiddleware(t *testing.T) {
	var op string
	block := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				t.Fatal("missing HTTP transport")
			}
			op = tr.Operation()
			return nil, errors.Forbidden("DENIED", "denied by middleware")
		}
	}
	srv := http.NewServer(http.Middleware(block))
	pb.RegisterOverviewHTTPServer(srv, nil)
	response := httptest.NewRecorder()
	srv.Handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/overview/inventory", nil))
	if response.Code != 403 || op != pb.OperationOverviewGetInventory {
		t.Fatalf("inventory operation %q = %d", op, response.Code)
	}
}
