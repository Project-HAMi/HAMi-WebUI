package hygon

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"vgpu/internal/provider/util"
)

// BoardSerial extracts the hardware board serial shared by HCU/DCU registrations and exporters.
// Legacy index-only DCU registrations (all digits or "N/A") return false.
func BoardSerial(provider, deviceID string) (string, bool) {
	if provider != "HCU" && provider != "DCU" {
		return "", false
	}
	serial, ok := strings.CutPrefix(deviceID, provider+"-")
	if !ok || serial == "" || serial == "N/A" || (provider == "DCU" && strings.Trim(serial, "0123456789") == "") {
		return "", false
	}
	return serial, true
}

// RegistrationFacts handles only the Kubernetes registration and verified
// allocation ID conversion. It never needs a telemetry-provided physical UUID.
func RegistrationFacts(raw, node string, hcu bool) ([]util.RegistrationDevice, error) {
	facts, err := util.DecodeRegistrationFacts(raw)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	result := []util.RegistrationDevice{}
	for _, fact := range facts {
		device := fact.Device
		if hcu {
			serial, ok := strings.CutPrefix(device.ID, "HCU-")
			if !ok || serial == "" {
				errs = append(errs, fmt.Errorf("invalid HCU serial registration"))
				continue
			}
			device.AliasId = device.ID
			device.ID = serial
		} else {
			minor, ok := strings.CutPrefix(device.ID, "DCU-")
			if !ok || minor == "" {
				errs = append(errs, fmt.Errorf("invalid DCU registration identity"))
				continue
			}
			// Registration IDs are retained verbatim. Only the verified numeric
			// minor form supports the legacy node-dcu-N allocation alias.
			if number, parseErr := strconv.Atoi(minor); parseErr == nil && number >= 0 {
				device.AliasId = fmt.Sprintf("%s-dcu-%d", node, number)
			}
		}
		// The common registration's default mode is not this provider's actual mode.
		device.Mode = ""
		fact.Known["mode"] = false
		result = append(result, fact)
	}
	return result, errors.Join(errs...)
}
