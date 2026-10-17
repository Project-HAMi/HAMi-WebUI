package util

import "testing"

// Malformed mthreads allocations must be rejected instead of silently
// becoming 0 memory or a full-card 100 core allocation.
func TestDecodeMthreadsContainerDevicesRejectsMalformed(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "non numeric memory", in: "GPU-0,Mthreads,abc,4"},
		{name: "non numeric cores", in: "GPU-0,Mthreads,1024,abc"},
		{name: "negative memory", in: "GPU-0,Mthreads,-1024,4"},
		{name: "negative cores", in: "GPU-0,Mthreads,1024,-1"},
		{name: "cores above vendor range", in: "GPU-0,Mthreads,1024,17"},
		{name: "missing fields", in: "GPU-0,Mthreads,1024"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeMthreadsContainerDevices(tt.in, "")
			if err == nil {
				t.Fatalf("decodeMthreadsContainerDevices(%q) = %+v, want error", tt.in, got)
			}
			if len(got) != 0 {
				t.Fatalf("rejected decode returned devices: %+v", got)
			}
		})
	}
}

// Raw core 0 is the vendor's "whole sliced card" value and must map to the
// full 100 baseline; every other vendor core unit scales linearly from 16.
func TestDecodeMthreadsContainerDevicesCoreScale(t *testing.T) {
	tests := []struct {
		name      string
		rawCores  string
		wantCores int32
	}{
		{name: "zero means whole sliced card", rawCores: "0", wantCores: 100},
		{name: "half a card", rawCores: "8", wantCores: 50},
		{name: "full card", rawCores: "16", wantCores: 100},
		{name: "one unit", rawCores: "1", wantCores: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeMthreadsContainerDevices("GPU-0,Mthreads,1024,"+tt.rawCores, "high")
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("devices = %+v, want 1", got)
			}
			if got[0].Usedcores != tt.wantCores {
				t.Fatalf("Usedcores = %d, want %d", got[0].Usedcores, tt.wantCores)
			}
			if got[0].Usedmem != 1024 {
				t.Fatalf("Usedmem = %d, want 1024", got[0].Usedmem)
			}
			if got[0].Priority != "high" {
				t.Fatalf("Priority = %q, want %q", got[0].Priority, "high")
			}
		})
	}
}