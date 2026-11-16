package resourceview

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// DeviceID uses the agreed identity tuple: provider/node/device for HAMi and
// native plugins, or driver/pool/device for DRA. Callers must prove identity,
// not substitute display names or UUID similarity. The result is URL-safe.
func DeviceID(authority, location, device string) (string, error) {
	parts := []string{authority, location, device}
	for _, part := range parts {
		if part == "" || strings.ContainsRune(part, '\x00') {
			return "", fmt.Errorf("device identity parts must be nonempty and contain no NUL")
		}
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "\x00"))), nil
}

// ParseDeviceID rejects noncanonical encodings and malformed tuples. IDs are
// opaque to clients; this helper is only for server-side lookups and validation.
func ParseDeviceID(id string) ([3]string, error) {
	var result [3]string
	data, err := base64.RawURLEncoding.Strict().DecodeString(id)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != id {
		return result, fmt.Errorf("invalid device identity encoding")
	}
	parts := strings.Split(string(data), "\x00")
	if len(parts) != 3 {
		return result, fmt.Errorf("invalid device identity tuple")
	}
	if _, err := DeviceID(parts[0], parts[1], parts[2]); err != nil {
		return result, err
	}
	copy(result[:], parts)
	return result, nil
}

// HygonBoardSerial is the identity shared by the registration and exporter.
// Old DCU plugins could register a node-local index instead of a board serial.
func (id DeviceIdentity) HygonBoardSerial() (string, bool) {
	if id.Provider != "HCU" && id.Provider != "DCU" {
		return "", false
	}
	serial, ok := strings.CutPrefix(id.DeviceID, id.Provider+"-")
	if !ok || serial == "" || serial == "N/A" || (id.Provider == "DCU" && strings.Trim(serial, "0123456789") == "") {
		return "", false
	}
	return serial, true
}
