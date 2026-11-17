package metax

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/data/prom"
)

func TestRegisteredSGPUsSurviveMissingTelemetry(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Annotations: map[string]string{
		MetaxSDeviceAnno: `[{"uuid":"card-A","model":"sgpu","totalVRam":65536},{"uuid":"card-B","model":"C500","totalCompute":100}]`,
	}}}
	for _, tc := range []struct {
		name, samples, modelA string
		indexA                uint
	}{
		{"cold start", "[]", "", 0},
		{"only card A scraped", `[{"metric":{"uuid":"card-A","deviceId":"0","modelName":"MXC500"},"value":[1,"1"]}]`, "MXC500", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if q := r.FormValue("query"); q != `mx_gpu_state{Hostname="node"}` {
					t.Errorf("unexpected query %q", q)
				}
				if _, err := fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":%s}}`, tc.samples); err != nil {
					t.Errorf("write metrics response: %v", err)
				}
			}))
			defer srv.Close()
			logger := log.NewStdLogger(io.Discard)
			client, err := prom.NewClient(srv.URL, time.Second, prom.HTTPConfig{}, logger)
			if err != nil {
				t.Fatal(err)
			}
			got, err := NewMetax(client, log.NewHelper(logger), "").FetchDevices(node)
			if err != nil || len(got) != 2 {
				t.Fatalf("devices = %+v, error = %v", got, err)
			}
			if a := got[0]; a.ID != "card-A" || a.AliasId != a.ID || a.Type != tc.modelA || a.Index != tc.indexA || a.Devmem != 65536 {
				t.Errorf("card A = %+v", a)
			}
			if b := got[1]; b.ID != "card-B" || b.AliasId != b.ID || b.Type != "C500" || b.Devcore != 100 {
				t.Errorf("card B = %+v", b)
			}
		})
	}
}
