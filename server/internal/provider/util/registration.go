package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// RegistrationDevice retains identity when an individual scalar is missing or
// invalid. Known prevents legacy zero defaults from becoming new-view evidence.
type RegistrationDevice struct {
	RawMode string
	Device  *DeviceInfo
	Known   map[string]bool
}

// DecodeRegistrationFacts reads the shared HAMi registration formats,
// while retaining field presence. JSON omitempty makes absent 0/false ambiguous;
// it is not a reason to discard an otherwise identified physical device.
func DecodeRegistrationFacts(raw string) ([]RegistrationDevice, error) {
	var rows []map[string]string
	var errs []error
	if strings.HasPrefix(strings.TrimSpace(raw), "[") {
		var input []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, err
		}
		for _, inputRow := range input {
			row := map[string]string{}
			for key, value := range inputRow {
				key = strings.ToLower(key)
				if key == "id" || key == "type" || key == "mode" {
					var text string
					if json.Unmarshal(value, &text) == nil {
						row[key] = text
					}
				} else {
					row[key] = string(value)
				}
			}
			rows = append(rows, row)
		}
	} else {
		if !strings.Contains(raw, ":") {
			return nil, fmt.Errorf("invalid registration framing")
		}
		for _, encoded := range strings.Split(raw, ":") {
			if encoded == "" {
				continue
			}
			fields := strings.Split(encoded, ",")
			if len(fields) != 7 && len(fields) != 9 {
				errs = append(errs, fmt.Errorf("invalid registration field count"))
				continue
			}
			row := map[string]string{"id": fields[0], "count": fields[1], "devmem": fields[2], "devcore": fields[3], "type": fields[4], "health": fields[6]}
			if len(fields) == 9 {
				row["mode"] = fields[8]
			}
			rows = append(rows, row)
		}
	}
	ids := map[string]int{}
	for _, row := range rows {
		ids[row["id"]]++
	}
	result := []RegistrationDevice{}
	for _, row := range rows {
		id := row["id"]
		if id == "" || ids[id] != 1 {
			errs = append(errs, fmt.Errorf("missing or duplicate registration identity"))
			continue
		}
		device := &DeviceInfo{ID: id, AliasId: id, Type: row["type"], Mode: row["mode"]}
		known := map[string]bool{"type": row["type"] != "", "mode": row["mode"] != ""}
		for key, target := range map[string]*int32{"count": &device.Count, "devmem": &device.Devmem, "devcore": &device.Devcore} {
			value, err := strconv.ParseInt(row[key], 10, 32)
			if err == nil && value >= 0 {
				*target = int32(value)
				known[key] = true
			}
		}
		if health, err := strconv.ParseBool(row["health"]); err == nil {
			device.Health = health
			known["health"] = true
		}
		result = append(result, RegistrationDevice{Device: device, Known: known, RawMode: row["mode"]})
	}
	return result, errors.Join(errs...)
}
