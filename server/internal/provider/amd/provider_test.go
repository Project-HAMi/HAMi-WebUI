package amd

import (
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/provider/util"
)

func TestFetchDevicesDecodesThePluginRegistration(t *testing.T) {
	provider := NewAMD(log.NewHelper(log.NewStdLogger(io.Discard)), "")
	// What amd-device-plugin writes: health is omitted for an unhealthy GPU.
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "gpu-1", Annotations: map[string]string{
		RegisterAnnos: `[{"id":"uuid-0","index":0,"count":2,"devmem":16304,"devcore":32,"type":"AMD Radeon Graphics","numa":-1,"health":true,"devicevendor":"AMD","custominfo":{"pciBDF":"0000:06:00.0"}},` +
			`{"id":"uuid-1","index":1,"count":2,"devmem":16304,"devcore":32,"type":"AMD Radeon Graphics","numa":-1,"devicevendor":"AMD"}]`,
	}}}
	devices, err := provider.FetchDevices(node)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 || devices[0].ID != "uuid-0" || devices[0].Devmem != 16304 || devices[0].Count != 2 {
		t.Fatalf("devices = %+v", devices)
	}
	if !devices[0].Health || devices[1].Health {
		t.Fatalf("health = %v/%v, want true/false (omitted means unhealthy)", devices[0].Health, devices[1].Health)
	}
	if provider.GetProvider() != "AMD" {
		t.Fatalf("provider = %q", provider.GetProvider())
	}
}

func TestFetchDevicesWithoutRegistration(t *testing.T) {
	provider := NewAMD(log.NewHelper(log.NewStdLogger(io.Discard)), "")
	devices, err := provider.FetchDevices(&corev1.Node{})
	if err != nil || len(devices) != 0 {
		t.Fatalf("no annotation = %v, %v, want nothing and no error", devices, err)
	}
	if _, err := provider.FetchDevices(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{RegisterAnnos: "{"}}}); err == nil {
		t.Fatal("a broken annotation should be an error")
	}
}

func TestNodeSelectorDefaultsToTheLabellerLabel(t *testing.T) {
	selector, err := NewAMD(log.NewHelper(log.NewStdLogger(io.Discard)), "").GetNodeDevicePluginLabels()
	if err != nil {
		t.Fatal(err)
	}
	if !selector.Matches(labelSet{"amd.com/gpu.family": "GC_12_0_0"}) || selector.Matches(labelSet{"gpu": "on"}) {
		t.Fatalf("selector %s does not follow the AMD labeller label", selector)
	}
}

func TestRegistersTheAllocationAnnotations(t *testing.T) {
	if util.InRequestDevices[AMDGPUDevice] != "hami.io/amd-devices-to-allocate" || util.SupportDevices[AMDGPUDevice] != "hami.io/amd-devices-allocated" {
		t.Fatalf("annotations = %q / %q", util.InRequestDevices[AMDGPUDevice], util.SupportDevices[AMDGPUDevice])
	}
}

type labelSet map[string]string

func (l labelSet) Has(k string) bool              { _, ok := l[k]; return ok }
func (l labelSet) Get(k string) string            { return l[k] }
func (l labelSet) Lookup(k string) (string, bool) { v, ok := l[k]; return v, ok }
