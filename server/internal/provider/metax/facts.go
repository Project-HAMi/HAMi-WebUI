package metax

import (
	"encoding/json"

	"vgpu/internal/provider/util"
)

// SGPURegistrationFacts retains the fields actually present in the authoritative
// sGPU annotation. Telemetry is not involved in identity, capacity or health.
func SGPURegistrationFacts(raw string) ([]util.RegistrationDevice, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil, err
	}
	normalized := make([]map[string]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		translated := map[string]json.RawMessage{}
		for from, to := range map[string]string{"uuid": "id", "model": "type", "totalDevCount": "count", "totalVRam": "devmem", "totalCompute": "devcore", "healthy": "health"} {
			if value, present := row[from]; present {
				translated[to] = value
			}
		}
		normalized = append(normalized, translated)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return util.DecodeRegistrationFacts(string(encoded))
}
