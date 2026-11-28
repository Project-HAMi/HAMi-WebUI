package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var smokeRunSeq atomic.Int64

func TestUnifiedBinarySmokeEndToEnd(t *testing.T) {
	cases := []struct {
		name         string
		basePath     string
		devicePrefix string
		promStall    bool
	}{
		{name: "root_path_prom_503", basePath: "/", devicePrefix: "GPU-smoke-root", promStall: false},
		{name: "subpath_prom_timeout", basePath: "/gpu-ui/", devicePrefix: "GPU-smoke-subpath", promStall: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deviceUUID := fmt.Sprintf("%s-%d", tc.devicePrefix, smokeRunSeq.Add(1))
			k8s := newSmokeK8sServer(t, deviceUUID)
			prom, promHits := newSmokePromServer(t, tc.promStall)

			backendAddr := freeLoopbackAddr(t)
			webAddr := freeLoopbackAddr(t)
			tempDir := t.TempDir()
			kubeconfigPath := writeSmokeKubeconfig(t, tempDir, k8s.URL)
			configPath := writeSmokeConfig(t, tempDir, backendAddr, prom.URL)
			t.Setenv("KUBECONFIG", kubeconfigPath)

			staticDir := newStaticDir(t)
			type initResult struct {
				app     *kratos.App
				cleanup func()
				err     error
			}
			initCh := make(chan initResult, 1)
			go func() {
				app, cleanup, err := initApp(configPath, webConfig{
					listenAddress:  webAddr,
					staticDir:      staticDir,
					basePath:       tc.basePath,
					frameAncestors: []string{"'self'"},
				}, context.Background())
				initCh <- initResult{app: app, cleanup: cleanup, err: err}
			}()

			var res initResult
			select {
			case res = <-initCh:
			case <-time.After(10 * time.Second):
				go func() {
					if late := <-initCh; late.cleanup != nil {
						if late.app != nil {
							_ = late.app.Stop()
						}
						late.cleanup()
					}
				}()
				t.Fatal("timed out waiting for initApp (K8s informer sync likely stalled)")
			}
			if res.err != nil {
				t.Fatalf("initApp: %v", res.err)
			}
			app, cleanup := res.app, res.cleanup

			runErrCh := make(chan error, 1)
			go func() {
				runErrCh <- app.Run()
			}()
			t.Cleanup(func() {
				_ = app.Stop()
				cleanup()
				select {
				case err := <-runErrCh:
					if err != nil && !errors.Is(err, context.Canceled) {
						t.Errorf("app.Run returned unexpected error: %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Error("timed out waiting for unified app shutdown")
				}
			})

			client := &http.Client{Timeout: 3 * time.Second}
			backendBase := "http://" + backendAddr
			webBase := "http://" + webAddr
			prefix := strings.TrimSuffix(tc.basePath, "/")

			waitHTTP200(t, client, backendBase+"/readyz")
			waitHTTP200(t, client, webBase+"/health_check")

			// 1. Public index serves injected base href and CSP frame-ancestors.
			indexResp, indexBody := doRequest(t, client, http.MethodGet, webBase+tc.basePath, nil)
			if indexResp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, body = %s", tc.basePath, indexResp.StatusCode, indexBody)
			}
			wantBaseTag := fmt.Sprintf(`<base data-hami-webui-base href="%s">`, tc.basePath)
			if !strings.Contains(string(indexBody), wantBaseTag) {
				t.Fatalf("index missing %s: %s", wantBaseTag, indexBody)
			}
			if csp := indexResp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
				t.Fatalf("Content-Security-Policy = %q, want frame-ancestors 'self'", csp)
			}

			// 2. Wait for asynchronous K8s WATCH establishment, then assert fast uncached /api/v1/overview/inventory.
			type inventoryReply struct {
				Nodes []struct {
					Name            string `json:"name"`
					Ready           bool   `json:"ready"`
					PhysicalDevices int    `json:"physicalDevices"`
				} `json:"nodes"`
				Devices []struct {
					Node             string   `json:"node"`
					Provider         string   `json:"provider"`
					DeviceID         string   `json:"deviceId"`
					Model            string   `json:"model"`
					Healthy          bool     `json:"healthy"`
					UtilizationRatio *float64 `json:"utilizationRatio"`
					MemoryUsageRatio *float64 `json:"memoryUsageRatio"`
				} `json:"devices"`
				NodeCoverage struct {
					Complete bool `json:"complete"`
				} `json:"nodeCoverage"`
				DeviceCoverage struct {
					Complete bool `json:"complete"`
				} `json:"deviceCoverage"`
			}
			invURL := webBase + prefix + "/api/v1/overview/inventory"
			var inv inventoryReply
			invDeadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(invDeadline) {
				invResp, invBody := doRequest(t, client, http.MethodGet, invURL, nil)
				if invResp.StatusCode == http.StatusOK && json.Unmarshal(invBody, &inv) == nil && inv.NodeCoverage.Complete && inv.DeviceCoverage.Complete {
					break
				}
				time.Sleep(25 * time.Millisecond)
			}
			if !inv.NodeCoverage.Complete || !inv.DeviceCoverage.Complete {
				t.Fatalf("inventory coverage did not become complete after WATCH connect: node=%+v device=%+v", inv.NodeCoverage, inv.DeviceCoverage)
			}

			invStart := time.Now()
			invResp, invBody := doRequest(t, client, http.MethodGet, invURL, nil)
			invElapsed := time.Since(invStart)
			if invResp.StatusCode != http.StatusOK {
				t.Fatalf("GET inventory status = %d, body = %s", invResp.StatusCode, invBody)
			}
			if invElapsed > 500*time.Millisecond {
				t.Fatalf("GET inventory took %s, want < 500ms even when Prometheus is degraded", invElapsed)
			}
			inv = inventoryReply{}
			if err := json.Unmarshal(invBody, &inv); err != nil {
				t.Fatalf("decode inventory: %v (%s)", err, invBody)
			}
			if len(inv.Nodes) != 1 || inv.Nodes[0].Name != "node-gpu-01" || !inv.Nodes[0].Ready || inv.Nodes[0].PhysicalDevices != 1 {
				t.Fatalf("unexpected inventory nodes: %+v", inv.Nodes)
			}
			if len(inv.Devices) != 1 || inv.Devices[0].DeviceID != deviceUUID || inv.Devices[0].Provider != "NVIDIA" || inv.Devices[0].Model != "NVIDIA-A100" || !inv.Devices[0].Healthy {
				t.Fatalf("unexpected inventory devices: %+v", inv.Devices)
			}
			if !inv.NodeCoverage.Complete || !inv.DeviceCoverage.Complete {
				t.Fatalf("unexpected coverage: node=%+v device=%+v", inv.NodeCoverage, inv.DeviceCoverage)
			}
			if inv.Devices[0].UtilizationRatio != nil || inv.Devices[0].MemoryUsageRatio != nil {
				t.Fatalf("expected nil telemetry ratios under Prometheus outage, got util=%v mem=%v", inv.Devices[0].UtilizationRatio, inv.Devices[0].MemoryUsageRatio)
			}

			// 3. Nodes list via /api/v1/nodes.
			nodesResp, nodesBody := doRequest(t, client, http.MethodGet, webBase+prefix+"/api/v1/nodes", nil)
			if nodesResp.StatusCode != http.StatusOK || !strings.Contains(string(nodesBody), `"node-gpu-01"`) {
				t.Fatalf("GET /api/v1/nodes = (%d) %s", nodesResp.StatusCode, nodesBody)
			}

			// 4. Devices list & detail (200 and 404).
			gpusResp, gpusBody := doRequest(t, client, http.MethodGet, webBase+prefix+"/api/v1/devices", nil)
			if gpusResp.StatusCode != http.StatusOK || !strings.Contains(string(gpusBody), `"`+deviceUUID+`"`) {
				t.Fatalf("GET /api/v1/devices = (%d) %s", gpusResp.StatusCode, gpusBody)
			}
			gpuResp, gpuBody := doRequest(t, client, http.MethodGet, webBase+prefix+"/api/v1/device?id="+deviceUUID, nil)
			if gpuResp.StatusCode != http.StatusOK {
				t.Fatalf("GET /api/v1/device status = %d, body = %s", gpuResp.StatusCode, gpuBody)
			}
			var gpuDetail struct {
				UUID        string `json:"uuid"`
				CoreUsed    int    `json:"coreUsed"`
				MemoryUsed  int    `json:"memoryUsed"`
				MemoryTotal int    `json:"memoryTotal"`
			}
			if err := json.Unmarshal(gpuBody, &gpuDetail); err != nil {
				t.Fatalf("decode device detail: %v (%s)", err, gpuBody)
			}
			if gpuDetail.UUID != deviceUUID || gpuDetail.CoreUsed != 50 || gpuDetail.MemoryUsed != 16384 || gpuDetail.MemoryTotal != 81920 {
				t.Fatalf("unexpected device detail: %+v", gpuDetail)
			}
			missResp, missBody := doRequest(t, client, http.MethodGet, webBase+prefix+"/api/v1/device?id=GPU-missing", nil)
			if missResp.StatusCode != http.StatusNotFound || !strings.Contains(string(missBody), `"RESOURCE_GPU_NOT_FOUND"`) {
				t.Fatalf("GET missing device = (%d) %s", missResp.StatusCode, missBody)
			}

			// 5. Containers list.
			wkResp, wkBody := doRequest(t, client, http.MethodGet, webBase+prefix+"/api/v1/containers?page=1&page_size=10", nil)
			if wkResp.StatusCode != http.StatusOK || !strings.Contains(string(wkBody), `"train-pod-0"`) || !strings.Contains(string(wkBody), `"`+deviceUUID+`"`) {
				t.Fatalf("GET /api/v1/containers = (%d) %s", wkResp.StatusCode, wkBody)
			}

			// 6. Legacy routes stay sunset and private routes stay unreachable on public Web port;
			// business APIs stay unreachable on the :8000 metrics/readiness listener.
			for _, blocked := range []string{"/metrics", "/readyz", "/api/vgpu/v1/nodes"} {
				privResp, _ := doRequest(t, client, http.MethodGet, webBase+prefix+blocked, nil)
				if privResp.StatusCode != http.StatusNotFound {
					t.Fatalf("public %s status = %d, want 404", prefix+blocked, privResp.StatusCode)
				}
			}
			for _, backendBlocked := range []string{"/api/v1/nodes", "/api/v1/overview/inventory", "/v1/nodes", "/q/openapi.yaml"} {
				privResp, _ := doRequest(t, client, http.MethodGet, backendBase+backendBlocked, nil)
				if privResp.StatusCode != http.StatusNotFound {
					t.Fatalf("backend %s status = %d, want 404", backendBlocked, privResp.StatusCode)
				}
			}

			// 7. Wait for background collector cycle commit and verify real hami_vgpu_count series on /metrics.
			wantLabel := fmt.Sprintf(`device_uuid=%q`, deviceUUID)
			metricsDeadline := time.Now().Add(6 * time.Second)
			var matched bool
			var lastMetrics string
			for time.Now().Before(metricsDeadline) {
				mResp, mBody := doRequest(t, client, http.MethodGet, backendBase+"/metrics", nil)
				lastMetrics = string(mBody)
				if mResp.StatusCode == http.StatusOK && hasMetricSeries(lastMetrics, "hami_vgpu_count{", wantLabel) {
					matched = true
					break
				}
				time.Sleep(50 * time.Millisecond)
			}
			if !matched {
				t.Fatalf("timed out waiting for hami_vgpu_count with %s on /metrics:\n%.800s", wantLabel, lastMetrics)
			}
			mStart := time.Now()
			mResp, mBody := doRequest(t, client, http.MethodGet, backendBase+"/metrics", nil)
			mElapsed := time.Since(mStart)
			if mResp.StatusCode != http.StatusOK || mElapsed > 500*time.Millisecond {
				t.Fatalf("GET /metrics = (%d in %s)", mResp.StatusCode, mElapsed)
			}
			if !hasMetricSeries(string(mBody), "hami_vgpu_count{", wantLabel) {
				t.Fatalf("/metrics missing hami_vgpu_count for %s", wantLabel)
			}
			if promHits.Load() == 0 {
				t.Fatal("expected MetricsGenerator to attempt upstream Prometheus queries")
			}
		})
	}
}

func hasMetricSeries(text, prefix, labelMatch string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) && strings.Contains(line, labelMatch) {
			return true
		}
	}
	return false
}

func newSmokeK8sServer(t *testing.T, deviceUUID string) *httptest.Server {
	t.Helper()
	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "node-gpu-01",
			UID:             "node-uid-01",
			ResourceVersion: "10",
			Labels:          map[string]string{"gpu": "on"},
			Annotations:     map[string]string{"hami.io/node-nvidia-register": deviceUUID + ",10,81920,100,NVIDIA-A100,0,true:"},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.10"}},
		},
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns-smoke",
			Name:              "train-pod-0",
			UID:               "pod-uid-01",
			ResourceVersion:   "10",
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0)),
			Annotations: map[string]string{
				"hami.io/vgpu-node":              "node-gpu-01",
				"hami.io/vgpu-devices-allocated": deviceUUID + ",NVIDIA,16384,50:;",
				"hami.io/vgpu-time":              "1700000000",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-gpu-01",
			Containers: []corev1.Container{{
				Name: "trainer",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.244.0.9"},
	}

	stopCh := make(chan struct{})
	writeList := func(w http.ResponseWriter, r *http.Request, payload any) {
		if q := r.URL.Query().Get("watch"); q == "true" || q == "1" {
			select {
			case <-stopCh:
				return
			default:
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
			case <-stopCh:
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/nodes":
			writeList(w, r, corev1.NodeList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "NodeList"},
				ListMeta: metav1.ListMeta{ResourceVersion: "10"},
				Items:    []corev1.Node{node},
			})
		case "/api/v1/pods":
			writeList(w, r, corev1.PodList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"},
				ListMeta: metav1.ListMeta{ResourceVersion: "10"},
				Items:    []corev1.Pod{pod},
			})
		case "/api/v1/events":
			writeList(w, r, corev1.EventList{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "EventList"},
				ListMeta: metav1.ListMeta{ResourceVersion: "10"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		close(stopCh)
		srv.CloseClientConnections()
		srv.Close()
	})
	return srv
}

func newSmokePromServer(t *testing.T, stall bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if stall {
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
			return
		}
		http.Error(w, "prometheus unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	return srv, &hits
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func writeSmokeKubeconfig(t *testing.T, dir, serverURL string) string {
	t.Helper()
	p := filepath.Join(dir, "kubeconfig")
	content := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: mock
  cluster:
    server: %s
contexts:
- name: mock
  context:
    cluster: mock
    user: mock
current-context: mock
users:
- name: mock
  user:
    token: smoke-token
`, serverURL)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return p
}

func writeSmokeConfig(t *testing.T, dir, backendAddr, promURL string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	content := fmt.Sprintf(`server:
  http:
    addr: %s
    timeout: 5s
  grpc:
    addr: 127.0.0.1:0
    timeout: 5s
prometheus:
  address: %s
  timeout: 50ms
exporter:
  interval: 0.5s
  timeout: 1.5s
device_config:
  enabled: false
node_selectors:
  NVIDIA: gpu=on
`, backendAddr, promURL)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func waitHTTP200(t *testing.T, client *http.Client, target string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(target)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for HTTP 200 on %s", target)
}

func doRequest(t *testing.T, client *http.Client, method, target string, body []byte) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", method, target, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do(%s %s): %v", method, target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s %s): %v", method, target, err)
	}
	return resp, raw
}
