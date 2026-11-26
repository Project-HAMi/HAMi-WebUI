package util

import "vgpu/internal/provider/internal/hamiwire"

// RegistrationDevice retains identity when an individual scalar is missing or
// invalid. Known prevents legacy zero defaults from becoming new-view evidence.
type RegistrationDevice struct {
	RawMode string
	Device  *DeviceInfo
	Known   map[string]bool
}

// DecodeRegistrationFacts reads the shared HAMi registration formats via
// hamiwire while preserving field presence and legacy DeviceInfo shape.
func DecodeRegistrationFacts(raw string) ([]RegistrationDevice, error) {
	records, err := hamiwire.DecodeRegistrations(raw)
	result := make([]RegistrationDevice, 0, len(records))
	for _, rec := range records {
		device := &DeviceInfo{
			ID:      rec.ID,
			AliasId: rec.ID,
			Count:   rec.Count,
			Devmem:  rec.Devmem,
			Devcore: rec.Devcore,
			Type:    rec.Type,
			Numa:    rec.Numa,
			Mode:    rec.Mode,
			Index:   rec.Index,
			Health:  rec.Health,
		}
		result = append(result, RegistrationDevice{Device: device, Known: rec.Known, RawMode: rec.RawMode})
	}
	return result, err
}
