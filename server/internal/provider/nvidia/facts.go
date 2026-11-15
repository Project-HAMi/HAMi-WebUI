package nvidia

import (
	"github.com/go-kratos/kratos/v2/log"

	"vgpu/internal/provider/util"
)

type RegistrationDevice = util.RegistrationDevice

// RegistrationFacts adds NVIDIA's verified legacy mode default and MIG details
// to the shared decoder's field-presence evidence.
func RegistrationFacts(raw string, logger *log.Helper) ([]RegistrationDevice, error) {
	facts, err := util.DecodeRegistrationFacts(raw)
	profiles := map[string]*util.DeviceInfo{}
	if legacy, decodeErr := registeredDevices(raw, logger); decodeErr == nil {
		for _, device := range legacy {
			if device != nil {
				profiles[device.ID] = device
			}
		}
	}
	for i := range facts {
		device := facts[i].Device
		device.Mode = registrationMode(device.Mode)
		facts[i].Known["mode"] = true
		if legacy := profiles[device.ID]; legacy != nil {
			device.MigProfiles = legacy.MigProfiles
		}
	}
	return facts, err
}
