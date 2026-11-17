package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/buildinfo"
	"vgpu/internal/data/prom"

	"github.com/go-kratos/kratos/v2/log"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/prometheus/common/model"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestHAMiSupport(t *testing.T) {
	for version, want := range map[string]string{
		"v2.10.0":         hamiSupported,
		"2.11.3":          hamiSupported,
		"v2.9.0":          hamiUnsupported,
		"v2.12.0":         hamiUnsupported,
		"v2.10.0-1a2b3c4": hamiDevelopment,
		"v0.0.0-master":   hamiDevelopment,
		"f4079c0-f4079c0": hamiDevelopment,
		"":                hamiDevelopment,
	} {
		if got := hamiSupport(version); got != want {
			t.Errorf("hamiSupport(%q) = %q, want %q", version, got, want)
		}
	}
}

func TestGetAboutGroupsVersionsAcrossComponents(t *testing.T) {
	restore := setBuildInfo("v2.0.0", "4f5875c")
	defer restore()
	service := newAboutTestService(t, func(query string) string {
		switch query {
		case hamiBuildInfoQuery:
			return `[
				{"metric":{"version":"v2.9.0","revision":"aaa","build_date":"d1","job":"monitor","instance":"10.0.0.9:9394"},"value":[1,"1"]},
				{"metric":{"version":"v2.10.0","revision":"bbb","build_date":"d2","job":"accelerator-control","instance":"10.0.0.2:31993"},"value":[1,"1"]},
				{"metric":{"version":"v2.10.0","revision":"bbb","build_date":"d2","job":"monitor","instance":"10.0.0.3:9394","pod":"hami-device-plugin-a"},"value":[1,"1"]},
				{"metric":{"version":"v2.10.0","revision":"bbb","build_date":"d2","job":"hami-scheduler","instance":"hami-scheduler.kube-system.svc:31993"},"value":[1,"1"]}
			]`
		case metricsRefreshQuery:
			return `[{"metric":{},"value":[1,"1792943310"]}]`
		}
		return `[]`
	}, []*biz.Node{
		{Name: "a", KubeletVersion: "v1.30.10"},
		{Name: "b", KubeletVersion: "v1.29.4"},
		{Name: "c", KubeletVersion: "v1.30.10"},
		{Name: "no-status"},
	})

	reply := getAbout(t, service)
	if reply.Webui.Version != "v2.0.0" || reply.Webui.Commit != "4f5875c" {
		t.Fatalf("webui = %+v", reply.Webui)
	}
	if reply.Hami.State != hamiStateOK || len(reply.Hami.Versions) != 2 {
		t.Fatalf("hami = %+v", reply.Hami)
	}
	current, old := reply.Hami.Versions[0], reply.Hami.Versions[1]
	if current.Version != "v2.10.0" || current.Support != hamiSupported || len(current.Targets) != 3 {
		t.Fatalf("most common version = %+v", current)
	}
	// By Pod name, then by Pod address; a job name alone says nothing reliable.
	for _, target := range append(current.Targets, old.Targets...) {
		want := map[string][2]string{
			"10.0.0.9:9394":                        {biz.HAMiDevicePlugin, "uid-b"},
			"10.0.0.2:31993":                       {biz.HAMiScheduler, "uid-s"},
			"10.0.0.3:9394":                        {biz.HAMiDevicePlugin, "uid-a"},
			"hami-scheduler.kube-system.svc:31993": {"", ""},
		}[target.Instance]
		if target.Component != want[0] || target.PodUid != want[1] {
			t.Fatalf("target %+v, want component %q and Pod %q", target, want[0], want[1])
		}
	}
	if reply.KubernetesVersion != "v1.31.0" || len(reply.Hami.Components) != 2 || reply.Hami.Components[0].Ready != 1 {
		t.Fatalf("cluster = %q %+v", reply.KubernetesVersion, reply.Hami.Components)
	}
	if old.Version != "v2.9.0" || old.Support != hamiUnsupported || old.Revision != "aaa" || old.BuildDate != "d1" {
		t.Fatalf("other version = %+v", old)
	}
	if len(reply.Kubelets) != 2 || reply.Kubelets[0].Version != "v1.30.10" || reply.Kubelets[0].Nodes != 2 {
		t.Fatalf("kubelets = %+v", reply.Kubelets)
	}
	if !reply.Prometheus.Reachable || reply.Prometheus.MetricsRefreshedAt != "2026-10-25T15:48:30Z" || reply.Prometheus.Version != "3.14.0" {
		t.Fatalf("prometheus = %+v", reply.Prometheus)
	}
	if reply.CheckedAt != "2026-10-25T15:09:00Z" {
		t.Fatalf("checkedAt = %q", reply.CheckedAt)
	}
}

func TestGetAboutReportsMissingBuildInfo(t *testing.T) {
	service := newAboutTestService(t, func(string) string { return `[]` }, nil)

	reply := getAbout(t, service)
	if reply.Hami.State != hamiStateMissing || len(reply.Hami.Versions) != 0 {
		t.Fatalf("hami = %+v", reply.Hami)
	}
	if !reply.Prometheus.Reachable || reply.Prometheus.MetricsRefreshedAt != "" {
		t.Fatalf("prometheus = %+v", reply.Prometheus)
	}
	if len(reply.Hami.Supported) != 2 {
		t.Fatalf("supported = %v", reply.Hami.Supported)
	}
}

func TestGetAboutTargetsAreTheOnlyAvailablePrometheusAPI(t *testing.T) {
	const target = `{"discoveredLabels":{"__meta_kubernetes_pod_uid":"p","__meta_kubernetes_pod_controller_kind":"DaemonSet","__meta_kubernetes_pod_container_name":"neuron-monitor","__meta_kubernetes_pod_container_image":"public.ecr.aws/neuron/neuron-monitor:1.3.0"},"health":"up"}`
	for _, tc := range []struct {
		name, response string
		complete       bool
	}{
		{"status follows targets", `{"data":{"activeTargets":[` + target + `]},"status":"success"}`, true},
		{"truncated after a valid target", `{"status":"success","data":{"activeTargets":[` + target + `,`, false},
		{"missing target array", `{"status":"success","data":{}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/targets" {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				_, _ = io.WriteString(w, tc.response)
			}))
			t.Cleanup(upstream.Close)
			reply := getAbout(t, newAboutService(t, upstream.URL, nil))
			sources := reply.DeviceSources
			if reply.Prometheus.Reachable != tc.complete || sources.ExportersChecked != tc.complete || !sources.PluginsChecked {
				t.Fatalf("prometheus=%v sources=%v", reply.Prometheus, sources)
			}
			if tc.complete {
				if len(sources.Exporters) != 1 || sources.Exporters[0].Up != 1 || sources.Exporters[0].Id != "neuron-monitor" {
					t.Fatalf("exporters=%v", sources.Exporters)
				}
			} else if len(sources.Exporters) != 0 {
				t.Fatalf("partial exporters leaked: %v", sources.Exporters)
			}
		})
	}
}

func TestGetAboutKeepsBuildWhenPrometheusIsDown(t *testing.T) {
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(prometheus.Close)
	service := newAboutService(t, prometheus.URL, nil)

	reply := getAbout(t, service)
	if reply.Hami.State != hamiStateUnavailable || reply.Prometheus.Reachable || reply.Prometheus.MetricsRefreshedAt != "" || reply.Prometheus.Version != "" {
		t.Fatalf("reply = %+v", reply)
	}
	if reply.Webui.Version != buildinfo.Version || reply.KubernetesVersion != "v1.31.0" || len(reply.Hami.Components) != 2 {
		t.Fatalf("Kubernetes data must not depend on Prometheus: %+v", reply)
	}
}

func TestHAMiPodIndexDecidesOnlyOnOnePod(t *testing.T) {
	index := newHAMiPodIndex([]*biz.HAMiComponent{
		{Name: biz.HAMiScheduler, Members: []biz.HAMiPod{
			{UID: "s-host", Name: "hami-scheduler-0", Namespace: "kube-system", IPs: []string{"192.0.2.10"}},
			{UID: "s-agent", Name: "hami-agent", Namespace: "tenant-scheduler", IPs: []string{"10.1.0.1"}},
		}},
		{Name: biz.HAMiDevicePlugin, Members: []biz.HAMiPod{
			{UID: "p-host", Name: "hami-device-plugin-0", Namespace: "kube-system", IPs: []string{"192.0.2.10"}},
			{UID: "p-agent", Name: "hami-agent", Namespace: "tenant-plugin", IPs: []string{"10.1.0.2"}},
			{UID: "p-other", Name: "hami-device-plugin-1", Namespace: "kube-system", IPs: []string{"10.1.0.3"}},
		}},
	})
	for _, tt := range []struct {
		name   string
		labels model.Metric
		want   string
	}{
		{name: "host-network Pods share the address", labels: model.Metric{"instance": "192.0.2.10:9394"}, want: ""},
		{name: "namespace picks one of two same-named Pods", labels: model.Metric{"pod": "hami-agent", "namespace": "tenant-plugin", "instance": "10.1.0.2:9394"}, want: "p-agent"},
		{name: "address picks one of two same-named Pods", labels: model.Metric{"pod": "hami-agent", "instance": "10.1.0.1:31993"}, want: "s-agent"},
		{name: "same-named Pods without a deciding address", labels: model.Metric{"pod": "hami-agent", "instance": "10.9.9.9:1"}, want: ""},
		{name: "unknown Pod name does not fall back to the address", labels: model.Metric{"pod": "other", "instance": "10.1.0.3:9394"}, want: ""},
		{name: "unique address", labels: model.Metric{"job": "accelerator-control", "instance": "10.1.0.3:9394"}, want: "p-other"},
		{name: "Service name", labels: model.Metric{"instance": "hami-scheduler.kube-system.svc:31993"}, want: ""},
	} {
		if got := index.match(tt.labels).uid; got != tt.want {
			t.Errorf("%s: matched %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestGetAboutTellsAFailedQueryFromAMissingSeries(t *testing.T) {
	service := newAboutTestService(t, func(query string) string {
		if query == hamiBuildInfoQuery {
			return `[]`
		}
		return ""
	}, nil)

	reply := getAbout(t, service)
	if !reply.Prometheus.Reachable || !reply.Prometheus.RefreshQueryFailed || reply.Prometheus.MetricsRefreshedAt != "" {
		t.Fatalf("prometheus = %+v", reply.Prometheus)
	}
	if reply.Hami.State != hamiStateMissing {
		t.Fatalf("hami = %+v", reply.Hami)
	}
}

func newAboutTestService(t *testing.T, result func(query string) string, nodes []*biz.Node) *AboutService {
	t.Helper()
	prometheus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/targets" && r.URL.Query().Get("state") == "active" {
			_, _ = io.WriteString(w, `{"status":"success","data":{"activeTargets":[]}}`)
			return
		}
		if r.URL.Path == "/api/v1/status/buildinfo" {
			_, _ = io.WriteString(w, `{"status":"success","data":{"version":"3.14.0"}}`)
			return
		}
		if err := r.ParseForm(); err != nil || r.URL.Path != "/api/v1/query" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		body := result(r.Form.Get("query"))
		if body == "" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, `{"status":"success","data":{"resultType":"vector","result":`+body+`}}`)
	}))
	t.Cleanup(prometheus.Close)
	return newAboutService(t, prometheus.URL, nodes)
}

func newAboutService(t *testing.T, address string, nodes []*biz.Node) *AboutService {
	t.Helper()
	client, err := prom.NewClient(address, time.Second, prom.HTTPConfig{}, log.NewStdLogger(io.Discard))
	if err != nil {
		t.Fatalf("create Prometheus client: %v", err)
	}
	cluster := &aboutTestCluster{components: []*biz.HAMiComponent{
		{Name: biz.HAMiScheduler, Namespace: "kube-system", Pods: 1, Ready: 1, Nodes: 1, Members: []biz.HAMiPod{
			{UID: "uid-s", Name: "hami-scheduler-x", Namespace: "kube-system", IPs: []string{"10.0.0.2"}},
		}},
		{Name: biz.HAMiDevicePlugin, Namespace: "kube-system", Pods: 2, Ready: 2, Nodes: 2, Members: []biz.HAMiPod{
			{UID: "uid-a", Name: "hami-device-plugin-a", Namespace: "kube-system", IPs: []string{"10.0.0.3"}},
			{UID: "uid-b", Name: "hami-device-plugin-b", Namespace: "kube-system", IPs: []string{"10.0.0.9"}},
		}},
	}}
	service := NewAboutService(client, biz.NewNodeUsecase(&containerTestNodeRepo{nodes: nodes}, log.DefaultLogger), cluster, log.NewStdLogger(io.Discard))
	service.now = func() time.Time { return time.Date(2026, 10, 25, 23, 9, 0, 0, time.FixedZone("CST", 8*3600)) }
	return service
}

// getAbout goes through the HTTP route so the JSON contract the page reads is covered.
func getAbout(t *testing.T, service *AboutService) *pb.AboutReply {
	t.Helper()
	server := kratoshttp.NewServer()
	pb.RegisterAboutHTTPServer(server, service)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/about", nil).WithContext(context.Background()))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/about = %d: %s", response.Code, response.Body)
	}
	var fields map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"webui", "hami", "kubelets", "prometheus", "checkedAt"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("response lacks %q: %s", field, response.Body)
		}
	}
	reply := &pb.AboutReply{}
	if err := protojson.Unmarshal(response.Body.Bytes(), reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

type aboutTestCluster struct{ components []*biz.HAMiComponent }

func (c *aboutTestCluster) ServerVersion(context.Context) (string, error) { return "v1.31.0", nil }

func (c *aboutTestCluster) Components(context.Context) (biz.ClusterComponents, error) {
	return biz.ClusterComponents{HAMi: c.components}, nil
}

func setBuildInfo(version, commit string) func() {
	previousVersion, previousCommit := buildinfo.Version, buildinfo.Commit
	buildinfo.Version, buildinfo.Commit = version, commit
	return func() { buildinfo.Version, buildinfo.Commit = previousVersion, previousCommit }
}
