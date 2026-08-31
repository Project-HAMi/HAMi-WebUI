package ascend

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"vgpu/internal/provider/util"
)

func TestDecodeRegisteredDevicesCollectsAscendVariants(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "ascend-node",
		Annotations: map[string]string{
			"hami.io/node-register-Ascend910B4": `[{"id":"B4-0","index":0,"count":1,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true}]`,
			"hami.io/node-register-Ascend910C":  `[{"id":"C-2","index":2,"count":1,"devmem":65536,"devcore":20,"type":"Ascend910C","health":true}]`,
		},
	}}

	devices, err := decodeRegisteredDevices(node)
	if err != nil {
		t.Fatalf("decodeRegisteredDevices() error = %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("device count = %d, want 2", len(devices))
	}
	if devices[0].ID != "B4-0" || devices[1].ID != "C-2" {
		t.Fatalf("device order = [%s, %s], want producer index order", devices[0].ID, devices[1].ID)
	}
	got := map[string]string{}
	for _, device := range devices {
		got[device.ID] = device.Type
		if device.Devcore != 100 {
			t.Errorf("device %s normalized core capacity = %d, want 100", device.ID, device.Devcore)
		}
	}
	if got["B4-0"] != "Ascend910B4" || got["C-2"] != "Ascend910C" {
		t.Fatalf("concrete device types were not preserved: %v", got)
	}
}

func TestReconcileRegisteredDevicesPrefersVDIEIdentity(t *testing.T) {
	// The exporter id is a physical index and may be non-contiguous after
	// filtering. Matching registration index 0 first would incorrectly replace
	// vdie-1 with the unrelated vdie-0.
	registered := []*util.DeviceInfo{{ID: "vdie-1", Index: 0, Health: false}}
	telemetry := map[string]*util.DeviceInfo{
		"0":      {ID: "vdie-0", Health: false},
		"vdie-1": {ID: "vdie-1", Health: true},
	}
	matched := reconcileRegisteredDevices(registered, telemetry)
	if !matched[0] {
		t.Fatalf("matched = %v, want vdie identity reconciled", matched)
	}
	if registered[0].ID != "vdie-1" || registered[0].AliasId != "vdie-1" {
		t.Fatalf("registration was associated with the wrong telemetry device: %#v", registered[0])
	}
	if !registered[0].Health {
		t.Fatalf("exact UUID telemetry did not refresh device health: %#v", registered[0])
	}
}

func TestDecodeRegisteredDevicesNormalizesWebUICoreCapacity(t *testing.T) {
	tests := []struct {
		name, commonWord string
		producerDevcore  int
	}{
		{name: "B3 hard mode physical cores", commonWord: "Ascend910B3", producerDevcore: 20},
		{name: "B4 hard mode physical cores", commonWord: "Ascend910B4", producerDevcore: 20},
		{name: "310P hard mode physical cores", commonWord: "Ascend310P", producerDevcore: 8},
		{name: "hami-core already normalized", commonWord: "Ascend910B3", producerDevcore: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name: "ascend-node",
				Annotations: map[string]string{
					"hami.io/node-register-" + tt.commonWord: fmt.Sprintf(
						`[{"id":"vdie-0","count":1,"devmem":65536,"devcore":%d,"type":%q,"health":true}]`,
						tt.producerDevcore,
						tt.commonWord,
					),
				},
			}}
			devices, err := decodeRegisteredDevices(node)
			if err != nil || len(devices) != 1 {
				t.Fatalf("decodeRegisteredDevices() = (%#v, %v)", devices, err)
			}
			if devices[0].Devcore != 100 {
				t.Fatalf("WebUI percentage capacity = %d, want 100 (producer physical core value %d)", devices[0].Devcore, tt.producerDevcore)
			}
		})
	}
}

func TestDecodeRegisteredDevicesDeduplicatesStaleCommonWordByHandshake(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "ascend-node",
		Annotations: map[string]string{
			"hami.io/node-register-Ascend910B3":  `[{"id":"same-vdie","count":1,"devmem":65536,"devcore":20,"type":"Ascend910B3","health":true}]`,
			"hami.io/node-handshake-Ascend910B3": "Reported_2026.08.01 10:00:00",
			"hami.io/node-register-Ascend910B4":  `[{"id":"same-vdie","count":1,"devmem":32768,"devcore":20,"type":"Ascend910B4","health":true}]`,
			"hami.io/node-handshake-Ascend910B4": "Reported_2026.08.02 10:00:00",
		},
	}}

	devices, err := decodeRegisteredDevices(node)
	if err != nil {
		t.Fatalf("decodeRegisteredDevices() error = %v", err)
	}
	if len(devices) != 1 || devices[0].ID != "same-vdie" || devices[0].Type != "Ascend910B4" {
		t.Fatalf("deduplicated devices = %#v, want freshest B4 registration", devices)
	}
}

func TestDecodeRegisteredDevicesDiscoversFutureAscendCommonWord(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "ascend-node",
		Annotations: map[string]string{
			"hami.io/node-register-AscendFuture": `[{"id":"future-vdie","count":1,"devmem":12345,"devcore":7,"type":"AscendFuture","health":true}]`,
		},
	}}

	devices, err := decodeRegisteredDevices(node)
	if err != nil || len(devices) != 1 {
		t.Fatalf("decodeRegisteredDevices() = (%#v, %v)", devices, err)
	}
	if devices[0].Type != "AscendFuture" || devices[0].Devmem != 12345 || devices[0].Devcore != 100 {
		t.Fatalf("future Ascend inventory = %#v", devices[0])
	}
}
