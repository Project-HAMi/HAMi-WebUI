package server

import (
	nethttp "net/http"
	"net/url"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport/http"

	pb "vgpu/api/observability/v1"
)

func overviewQuery(r *nethttp.Request, target any) error {
	if _, ok := target.(*pb.InventoryRequest); !ok {
		return http.DefaultRequestQuery(r, target)
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 0 {
		return errors.BadRequest("INVALID_ARGUMENT", "The overview inventory does not accept query parameters")
	}
	return nil
}
