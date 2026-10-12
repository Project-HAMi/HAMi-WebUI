package hygon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/util"
)

func TestHCURegistrationPreservesPhysicalSerialAndAllocationIdentity(t *testing.T) {
	// k8s-hcu-device-plugin cc7d33f register.go + util.EncodeNodeDevices:
	// id,count,MiB,core percentage,type,NUMA,health,index,mode.
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "hcu-node",
		Annotations: map[string]string{
			HCURegisterAnnos: "HCU-serial-with-hyphens,4,65536,100,HCU-K100_AI,1,true,3,hami:",
		},
	}}
	provider := NewHCU(log.NewHelper(log.DefaultLogger), "")
	devices, err := provider.FetchDevices(node)
	if err != nil || len(devices) != 1 {
		t.Fatalf("FetchDevices() = (%v, %v), want one device", devices, err)
	}
	device := devices[0]
	if device.ID != "serial-with-hyphens" || device.AliasId != "HCU-serial-with-hyphens" || device.Index != 3 {
		t.Fatalf("HCU serial/index/allocation identity = %+v", device)
	}
	if device.Count != 4 || device.Devmem != 65536 || device.Devcore != 100 || device.Type != "HCU-K100_AI" || !device.Health {
		t.Fatalf("HCU registered capacity/model/health changed: %+v", device)
	}
	// The registered mode is a format default, not a HAMi split mode.
	if device.Mode != "" {
		t.Fatalf("HCU mode = %q, want none", device.Mode)
	}
	if provider.GetProvider() != "HCU" {
		t.Fatalf("provider = %q, want HCU", provider.GetProvider())
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			"hami.io/hcu-devices-allocated": ";HCU-serial-with-hyphens,HCU,8192,25:;",
		}},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init"}},
			Containers:     []corev1.Container{{Name: "worker"}},
		},
	}
	allocations, err := util.DecodePodDevices(pod, log.NewHelper(log.DefaultLogger))
	if err != nil {
		t.Fatal(err)
	}
	slots := allocations[HygonHCUDevice]
	if len(slots) != 2 || len(slots[0]) != 0 || len(slots[1]) != 1 {
		t.Fatalf("HCU container slots = %+v, want empty init and one worker", slots)
	}
	allocated := slots[1][0]
	if allocated.UUID != device.AliasId || allocated.Type != "HCU" || allocated.Usedmem != 8192 || allocated.Usedcores != 25 {
		t.Fatalf("HCU allocation does not join registration: %+v", allocated)
	}
}

func TestHCUInvalidRegistrationIDOnlySkipsThatDevice(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "hcu-node",
		Annotations: map[string]string{
			HCURegisterAnnos: "HCU-,4,65536,100,HCU-K100_AI,0,true,0,hami:" +
				"HCU-serial-b,4,65536,100,HCU-K100_AI,0,true,1,hami:" +
				"serial-c,4,65536,100,HCU-K100_AI,1,true,2,hami:",
		},
	}}
	devices, err := NewHCU(log.NewHelper(log.DefaultLogger), "").FetchDevices(node)
	if err != nil {
		t.Fatalf("FetchDevices() error = %v, want the valid device to remain", err)
	}
	if len(devices) != 1 || devices[0].ID != "serial-b" || devices[0].AliasId != "HCU-serial-b" || devices[0].Index != 1 {
		t.Fatalf("FetchDevices() = %+v, want only HCU-serial-b", devices)
	}
}

func TestHCUSelectorWorksWithOlderDirectConfig(t *testing.T) {
	for _, tt := range []struct {
		selector string
		matching labels.Set
	}{
		{"", labels.Set{"hcu": "on"}},
		{"hcu=on", labels.Set{"hcu": "on"}},
	} {
		provider := NewHCU(log.NewHelper(log.DefaultLogger), tt.selector)
		selector, err := provider.GetNodeDevicePluginLabels()
		if err != nil || !selector.Matches(tt.matching) || selector.Matches(labels.Set{"gpu": "on"}) {
			t.Fatalf("HCU selector %q = (%v, %v)", tt.selector, selector, err)
		}
	}
}

func TestHCUProviderDoesNotClaimLegacyDCURegistration(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		RegisterAnnos: "DCU-3,4,65536,100,DCU-K100,0,true:",
	}}}
	devices, err := NewHCU(log.NewHelper(log.DefaultLogger), "").FetchDevices(node)
	if err != nil || len(devices) != 0 {
		t.Fatalf("HCU provider claimed legacy DCU registration: %v, %v", devices, err)
	}
}

func TestLegacyDCUInventoryAndAllocationStillUseMinorNumberAlias(t *testing.T) {
	promServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if got := r.Form.Get("query"); got != `dcu_temp{node="legacy-dcu-node"}` {
			t.Errorf("legacy inventory query = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"node":"legacy-dcu-node","minor_number":"3","device_id":"legacy-serial"},"value":[1,"42"]}]}}`); err != nil {
			t.Errorf("write Prometheus response: %v", err)
		}
	}))
	defer promServer.Close()
	client, err := prom.NewClient(promServer.URL, time.Second, prom.HTTPConfig{}, log.DefaultLogger)
	if err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "legacy-dcu-node", Annotations: map[string]string{
		RegisterAnnos: "DCU-3,4,65536,100,DCU-K100,0,true,3,hami-core:",
	}}}
	devices, err := NewHygon(client, log.NewHelper(log.DefaultLogger), "dcu=on").FetchDevices(node)
	if err != nil || len(devices) != 1 {
		t.Fatalf("legacy DCU inventory = %v, %v", devices, err)
	}
	if devices[0].ID != "legacy-serial" || devices[0].AliasId != "legacy-dcu-node-dcu-3" || devices[0].Mode != "" {
		t.Fatalf("legacy DCU identity = %+v", devices[0])
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			util.AssignedNodeAnnotations:    node.Name,
			"hami.io/dcu-devices-allocated": "DCU-3,DCU,8192,25:;",
		}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
	}
	allocations, err := util.DecodePodDevices(pod, log.NewHelper(log.DefaultLogger))
	if err != nil || len(allocations[HygonDCUDevice]) != 1 || len(allocations[HygonDCUDevice][0]) != 1 {
		t.Fatalf("legacy DCU allocations = %v, %v", allocations, err)
	}
	if allocations[HygonDCUDevice][0][0].UUID != devices[0].AliasId {
		t.Fatalf("legacy DCU allocation no longer joins inventory: %+v", allocations)
	}
}
