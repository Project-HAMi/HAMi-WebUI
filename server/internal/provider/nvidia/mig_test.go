package nvidia

import (
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const reservedGPUM = `[{"containerIndex":1,"deviceIndex":0,"gpuUUID":"GPU-M","profile":"3g.40gb","placement":{"start":4,"size":4},
	"migUUID":"MIG-1","gpuInstanceID":2,"computeInstanceID":0}]`

func podWith(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
}

func TestAllocationsNameAModeOnlyWhenHAMiStatesIt(t *testing.T) {
	registered := map[string]string{"GPU-C": ModeHamiCore, "GPU-M": ModeMig}
	reserved := podWith(map[string]string{MigAllocationsAnnotation: reservedGPUM})
	broken := podWith(map[string]string{MigAllocationsAnnotation: `[{"containerIndex":"1"}]`})
	for _, tt := range []struct {
		name      string
		pod       *corev1.Pod
		modes     map[string]string
		container int
		uuid      string
		want      Device
	}{
		{"hami-core GPU", reserved, registered, 0, "GPU-C", Device{UUID: "GPU-C", Mode: ModeHamiCore}},
		{"reserved MIG device", reserved, registered, 1, "GPU-M", Device{UUID: "GPU-M", Mode: ModeMig, Profile: "3g.40gb", Start: 4, Size: 4}},
		{"MIG GPU without its reservation", reserved, registered, 0, "GPU-M", Device{UUID: "GPU-M", Reason: ReasonMigReservationMissing}},
		{"reservation for another GPU", reserved, map[string]string{"GPU-X": ModeMig}, 1, "GPU-X", Device{UUID: "GPU-X", Reason: ReasonMigReservationMismatch}},
		{"reservation on a hami-core GPU", podWith(map[string]string{MigAllocationsAnnotation: `[{"containerIndex":1,"deviceIndex":0,"gpuUUID":"GPU-C","profile":"1g.5gb","placement":{"start":0,"size":1}}]`}), registered, 1, "GPU-C", Device{UUID: "GPU-C", Reason: ReasonMigReservationMismatch}},
		{"GPU recorded before HAMi v2.10", podWith(nil), registered, 0, "GPU-M[0-1]", Device{UUID: "GPU-M", Mode: ModeMig}},
		{"unusable reservations on a hami-core GPU", broken, registered, 0, "GPU-C", Device{UUID: "GPU-C", Mode: ModeHamiCore}},
		{"unusable reservations on a MIG GPU", broken, registered, 1, "GPU-M", Device{UUID: "GPU-M", Reason: ReasonMigReservationInvalid}},
		{"node unread, reservation matches", reserved, nil, 1, "GPU-M", Device{UUID: "GPU-M", Mode: ModeMig, Profile: "3g.40gb", Start: 4, Size: 4}},
		{"node unread, nothing stated", podWith(nil), nil, 0, "GPU-C", Device{UUID: "GPU-C", Reason: ReasonDeviceModeUnknown}},
		{"node unread, Pod asked for hami-core", podWith(map[string]string{VGPUModeAnnotation: "hami-core"}), nil, 0, "GPU-C", Device{UUID: "GPU-C", Mode: ModeHamiCore}},
		{"node unread, Pod asked for mps", podWith(map[string]string{VGPUModeAnnotation: "mps"}), nil, 0, "GPU-C", Device{UUID: "GPU-C", Mode: ModeHamiCore}},
		{"GPU missing from the registration", podWith(nil), registered, 0, "GPU-Z", Device{UUID: "GPU-Z", Reason: ReasonDeviceModeUnknown}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReadAllocations(tt.pod, tt.modes).Device(tt.container, 0, tt.uuid); got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	if ReadAllocations(broken, registered).Err() == nil || ReadAllocations(reserved, registered).Err() != nil {
		t.Fatal("Err does not report unusable reservations")
	}
}

// The checks of HAMi v2.10.0 pkg/device/nvidia/mig_allocations.go.
func TestMigReservationsFollowHAMisDecoder(t *testing.T) {
	entry := func(extra string) string {
		return `[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-M","profile":"1g.5gb","placement":{"start":0,"size":1}` + extra + `}]`
	}
	for _, raw := range []string{
		`not json`,
		`[{"containerIndex":"0","deviceIndex":0,"gpuUUID":"GPU-M","profile":"1g.5gb","placement":{"start":0,"size":1}}]`,
		`[{"containerIndex":-1,"deviceIndex":0,"gpuUUID":"GPU-M","profile":"1g.5gb","placement":{"start":0,"size":1}}]`,
		`[{"containerIndex":0,"deviceIndex":0,"profile":"1g.5gb","placement":{"start":0,"size":1}}]`,
		`[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-M","placement":{"start":0,"size":1}}]`,
		`[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-M","profile":"1g.5gb","placement":{"start":0,"size":0}}]`,
		entry(`,"migUUID":"MIG-1"`),
		`[{"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-M","profile":"1g.5gb","placement":{"start":0,"size":1}},
		  {"containerIndex":0,"deviceIndex":0,"gpuUUID":"GPU-N","profile":"1g.5gb","placement":{"start":1,"size":1}}]`,
	} {
		if _, err := decodeMigAllocations(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{entry(""), entry(`,"migUUID":"MIG-1","gpuInstanceID":0,"computeInstanceID":0`)} {
		if _, err := decodeMigAllocations(raw); err != nil {
			t.Errorf("rejected %s: %v", raw, err)
		}
	}
}

func TestLegacyMigUUIDs(t *testing.T) {
	for uuid, want := range map[string]string{
		"GPU-0a1b-2c3d[1-2]": "GPU-0a1b-2c3d",
		"GPU-0a1b-2c3d":      "",
		"GPU-0a1b[1]":        "",
		"GPU-0a1b[a-2]":      "",
		"[1-2]":              "",
		"GPU-0a1b[1-2]x":     "",
	} {
		got, ok := LegacyMigUUID(uuid)
		if got != want || ok != (want != "") {
			t.Errorf("%q: got %q %v", uuid, got, ok)
		}
	}
}

func TestRegistrationKeepsGPUsWhoseProfilesDoNotDecode(t *testing.T) {
	logger := log.NewHelper(log.NewStdLogger(io.Discard))
	encoded := `[
		{"id":"GPU-A","count":10,"devmem":40960,"devcore":100,"type":"NVIDIA A100","mode":"mig","health":true,"migProfiles":{}},
		{"id":"GPU-B","count":7,"devmem":40960,"devcore":100,"type":"NVIDIA A100","mode":"mig","health":true,
		 "migProfiles":[{"name":"1g.5gb","memoryMB":4864,"core":14,"sliceCount":1,"placements":[{"start":0,"size":1}]}]},
		{"id":"GPU-C","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA A10","mode":"mps","health":true},
		{"id":"GPU-D","count":10,"devmem":24576,"devcore":100,"type":"NVIDIA A10","health":true}]`
	devices, err := registeredDevices(encoded, logger)
	if err != nil || len(devices) != 4 {
		t.Fatalf("devices = %v, err = %v", devices, err)
	}
	if devices[0].ID != "GPU-A" || devices[0].MigProfiles != nil || devices[0].Mode != ModeMig {
		t.Fatalf("GPU with malformed profiles = %+v", devices[0])
	}
	if len(devices[1].MigProfiles) != 1 || devices[1].MigProfiles[0].Placements[0].Size != 1 {
		t.Fatalf("GPU with profiles = %+v", devices[1])
	}
	// HAMi's plugin registers mps but runs HAMi-core; an unregistered mode predates both.
	if devices[2].Mode != ModeHamiCore || devices[3].Mode != ModeHamiCore {
		t.Fatalf("modes = %q, %q", devices[2].Mode, devices[3].Mode)
	}

	legacy, err := registeredDevices("GPU-L,10,24576,100,NVIDIA A10,0,true,0,mig:", logger)
	if err != nil || len(legacy) != 1 || legacy[0].Mode != ModeMig {
		t.Fatalf("legacy registration = %v, err = %v", legacy, err)
	}

	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{RegisterAnnos: encoded}}}
	modes := RegisteredModes(node, logger)
	if modes["GPU-A"] != ModeMig || modes["GPU-C"] != ModeHamiCore || len(modes) != 4 {
		t.Fatalf("modes = %v", modes)
	}
	if RegisteredModes(nil, logger) != nil || RegisteredModes(&corev1.Node{}, logger) != nil {
		t.Fatal("an unread registration produced modes")
	}
}
