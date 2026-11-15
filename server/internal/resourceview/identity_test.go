package resourceview

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDeviceIdentityKeepsProviderAndNodeBoundaries(t *testing.T) {
	inputs := [][3]string{{"hami", "node-a", "GPU-1"}, {"native", "node-a", "GPU-1"}, {"hami", "node-b", "GPU-1"}, {"gpu.nvidia.com", "pool/node-a", "slice/0"}}
	seen := map[string]bool{}
	for _, parts := range inputs {
		id, err := DeviceID(parts[0], parts[1], parts[2])
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] || strings.ContainsAny(id, "/+=") {
			t.Fatalf("ambiguous or unsafe ID: %q", id)
		}
		seen[id] = true
		decoded, err := ParseDeviceID(id)
		if err != nil || decoded != parts {
			t.Fatalf("identity did not roundtrip: %v, %v", decoded, err)
		}
	}
}

func TestMalformedIdentityIsRejected(t *testing.T) {
	for _, parts := range [][3]string{{"", "n", "d"}, {"p", "", "d"}, {"p", "n", ""}, {"p", "n\x00other", "d"}} {
		if _, err := DeviceID(parts[0], parts[1], parts[2]); err == nil {
			t.Fatalf("accepted %q", parts)
		}
	}
	valid, _ := DeviceID("p", "n", "d")
	for _, id := range []string{"", "!!!", valid + "=", valid + "\n", base64.RawURLEncoding.EncodeToString([]byte("p\x00n")), base64.RawURLEncoding.EncodeToString([]byte("p\x00n\x00"))} {
		if _, err := ParseDeviceID(id); err == nil {
			t.Fatalf("accepted malformed ID %q", id)
		}
	}
}
