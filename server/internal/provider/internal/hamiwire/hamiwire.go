// Package hamiwire decodes the shared HAMi Node registration and Pod allocation wire formats.
// It holds no vendor names or Kubernetes dependencies.
package hamiwire

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// ContainerDeviceSeparator separates multiple devices assigned to one container.
	ContainerDeviceSeparator = ":"
)

// RegistrationRecord captures one device entry from a HAMi Node registration
// annotation alongside per-field presence flags (distinguishing explicit zero
// values from omitempty-omitted fields).
type RegistrationRecord struct {
	ID, Type, Mode, RawMode string
	Count, Devmem, Devcore  int32
	Numa                    int
	Index                   uint
	Health                  bool
	Known                   map[string]bool
}

// AllocationSegment captures one "id,type,memory,cores" allocation entry inside
// a container's annotation slot.
type AllocationSegment struct {
	Idx         int
	ID, DevType string
	Mem, Cores  int32
}

// DecodeRegistrations parses both HAMi JSON array and colon/comma 7- or 9-field
// node registration payloads while preserving per-field presence evidence.
func DecodeRegistrations(raw string) ([]RegistrationRecord, error) {
	rows, errs := extractRegistrationRows(raw)
	if rows == nil && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	ids := map[string]int{}
	for _, row := range rows {
		ids[row["id"]]++
	}
	result := []RegistrationRecord{}
	for _, row := range rows {
		id := row["id"]
		if id == "" || ids[id] != 1 {
			errs = append(errs, fmt.Errorf("missing or duplicate registration identity"))
			continue
		}
		record := RegistrationRecord{
			ID:      id,
			Type:    row["type"],
			Mode:    row["mode"],
			RawMode: row["mode"],
			Known: map[string]bool{
				"type": row["type"] != "",
				"mode": row["mode"] != "",
			},
		}
		for key, target := range map[string]*int32{
			"count":   &record.Count,
			"devmem":  &record.Devmem,
			"devcore": &record.Devcore,
		} {
			if value, err := strconv.ParseInt(row[key], 10, 32); err == nil && value >= 0 {
				*target = int32(value)
				record.Known[key] = true
			}
		}
		if numa, err := strconv.Atoi(row["numa"]); err == nil {
			record.Numa = numa
			record.Known["numa"] = true
		}
		if idx, err := strconv.ParseUint(row["index"], 10, 32); err == nil {
			record.Index = uint(idx)
			record.Known["index"] = true
		}
		if health, err := strconv.ParseBool(row["health"]); err == nil {
			record.Health = health
			record.Known["health"] = true
		}
		result = append(result, record)
	}
	return result, errors.Join(errs...)
}

func extractRegistrationRows(raw string) ([]map[string]string, []error) {
	var rows []map[string]string
	var errs []error
	if strings.HasPrefix(strings.TrimSpace(raw), "[") {
		var input []map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			return nil, []error{err}
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
					row[key] = strings.TrimSpace(string(value))
				}
			}
			rows = append(rows, row)
		}
		return rows, nil
	}
	if !strings.Contains(raw, ContainerDeviceSeparator) {
		return nil, []error{fmt.Errorf("invalid registration framing")}
	}
	for _, encoded := range strings.Split(raw, ContainerDeviceSeparator) {
		if encoded == "" {
			continue
		}
		fields := strings.Split(encoded, ",")
		if len(fields) != 7 && len(fields) != 9 {
			errs = append(errs, fmt.Errorf("invalid registration field count"))
			continue
		}
		row := map[string]string{
			"id":      fields[0],
			"count":   fields[1],
			"devmem":  fields[2],
			"devcore": fields[3],
			"type":    fields[4],
			"numa":    fields[5],
			"health":  fields[6],
		}
		if len(fields) == 9 {
			row["index"] = fields[7]
			row["mode"] = fields[8]
		}
		rows = append(rows, row)
	}
	return rows, errs
}

// ParseAllocationSegment decodes one "id,type,memory,cores" allocation entry.
// HAMi may append optional trailing fields after index 3.
func ParseAllocationSegment(raw string) (AllocationSegment, error) {
	fields := strings.Split(raw, ",")
	if len(fields) < 4 {
		return AllocationSegment{}, fmt.Errorf("allocation %q has fewer than 4 fields", raw)
	}
	if fields[0] == "" || fields[1] == "" {
		return AllocationSegment{}, fmt.Errorf("allocation %q has no device id or type", raw)
	}
	mem, err := parseNonNegativeInt32(fields[2])
	if err != nil {
		return AllocationSegment{}, fmt.Errorf("allocation %q memory: %w", raw, err)
	}
	cores, err := parseNonNegativeInt32(fields[3])
	if err != nil {
		return AllocationSegment{}, fmt.Errorf("allocation %q cores: %w", raw, err)
	}
	return AllocationSegment{ID: fields[0], DevType: fields[1], Mem: mem, Cores: cores}, nil
}

// DecodeContainerAllocations decodes all non-empty device segments within one
// container allocation slot.
func DecodeContainerAllocations(raw string) ([]AllocationSegment, error) {
	segments := []AllocationSegment{}
	for i, entry := range strings.Split(raw, ContainerDeviceSeparator) {
		if entry == "" {
			continue
		}
		seg, err := ParseAllocationSegment(entry)
		if err != nil {
			return nil, err
		}
		seg.Idx = i
		segments = append(segments, seg)
	}
	return segments, nil
}

func parseNonNegativeInt32(s string) (int32, error) {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative value %d", v)
	}
	return int32(v), nil
}
