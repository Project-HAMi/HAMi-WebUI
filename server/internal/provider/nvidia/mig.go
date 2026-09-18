package nvidia

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// HAMi's scheduler records the MIG device it reserved for each allocated
	// GPU (pkg/device/nvidia/mig_allocations.go); the device plugin fills in
	// the runtime identity.
	MigAllocationsAnnotation = "hami.io/vgpu-mig-allocations"
	// The mode a Pod asks for; HAMi only places it on GPUs registered in that mode.
	VGPUModeAnnotation = "nvidia.com/vgpu-mode"
)

type migAllocation struct {
	ContainerIndex    int          `json:"containerIndex"`
	DeviceIndex       int          `json:"deviceIndex"`
	GPUUUID           string       `json:"gpuUUID"`
	Profile           string       `json:"profile"`
	Placement         migPlacement `json:"placement"`
	MigUUID           string       `json:"migUUID,omitempty"`
	GPUInstanceID     *uint32      `json:"gpuInstanceID,omitempty"`
	ComputeInstanceID *uint32      `json:"computeInstanceID,omitempty"`
}

type migPlacement struct {
	Start uint32 `json:"start"`
	Size  uint32 `json:"size"`
}

type slot struct{ container, device int }

// decodeMigAllocations applies the checks of HAMi's DecodeMigAllocations: one
// incomplete or duplicated entry makes the whole annotation unusable.
func decodeMigAllocations(raw string) (map[slot]migAllocation, error) {
	var allocations []migAllocation
	if err := json.Unmarshal([]byte(raw), &allocations); err != nil {
		return nil, err
	}
	result := make(map[slot]migAllocation, len(allocations))
	for i, allocation := range allocations {
		if allocation.ContainerIndex < 0 || allocation.DeviceIndex < 0 || allocation.GPUUUID == "" ||
			allocation.Profile == "" || allocation.Placement.Size == 0 ||
			allocation.Placement.Start > math.MaxInt32 || allocation.Placement.Size > math.MaxInt32 {
			return nil, fmt.Errorf("MIG allocation %d is incomplete", i)
		}
		runtimeFields := 0
		for _, set := range []bool{allocation.MigUUID != "", allocation.GPUInstanceID != nil, allocation.ComputeInstanceID != nil} {
			if set {
				runtimeFields++
			}
		}
		if runtimeFields != 0 && runtimeFields != 3 {
			return nil, fmt.Errorf("MIG allocation %d has partial runtime identity", i)
		}
		key := slot{allocation.ContainerIndex, allocation.DeviceIndex}
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("duplicate MIG allocation for container %d device %d", key.container, key.device)
		}
		result[key] = allocation
	}
	return result, nil
}

// LegacyMigUUID returns the GPU of a MIG allocation recorded by HAMi before
// v2.10, which appended the template and slot to the UUID, as in GPU-x[1-2].
func LegacyMigUUID(uuid string) (string, bool) {
	left := strings.Index(uuid, "[")
	if left <= 0 || !strings.HasSuffix(uuid, "]") {
		return "", false
	}
	parts := strings.Split(uuid[left+1:len(uuid)-1], "-")
	if len(parts) != 2 {
		return "", false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return "", false
		}
	}
	return uuid[:left], true
}

// Allocations holds what one Pod's annotations and its node's registration
// say about the Pod's GPUs.
type Allocations struct {
	reservations map[slot]migAllocation
	err          error
	podMode      string
	deviceModes  map[string]string
}

// ReadAllocations reads the Pod's MIG reservations. deviceModes comes from
// RegisteredModes and is nil when the node could not be read.
func ReadAllocations(pod *corev1.Pod, deviceModes map[string]string) Allocations {
	allocations := Allocations{podMode: pod.Annotations[VGPUModeAnnotation], deviceModes: deviceModes}
	if raw := pod.Annotations[MigAllocationsAnnotation]; raw != "" {
		allocations.reservations, allocations.err = decodeMigAllocations(raw)
	}
	return allocations
}

// Err reports why the MIG reservations could not be used.
func (a Allocations) Err() error {
	return a.err
}

// Why an allocated GPU's mode cannot be told.
const (
	ReasonMigReservationInvalid  = "mig_reservation_invalid"
	ReasonMigReservationMissing  = "mig_reservation_missing"
	ReasonMigReservationMismatch = "mig_reservation_mismatch"
	ReasonDeviceModeUnknown      = "device_mode_unknown"
)

// Device is one allocated GPU as WebUI reports it.
type Device struct {
	UUID string
	// ModeHamiCore or ModeMig; empty with a Reason when it cannot be told.
	Mode    string
	Reason  string
	Profile string
	Start   int32
	Size    int32
}

// Device resolves the GPU at a container slot and device index. It names a
// mode only when the reservation and the registration agree on it.
func (a Allocations) Device(container, index int, uuid string) Device {
	if parent, legacy := LegacyMigUUID(uuid); legacy {
		return Device{UUID: parent, Mode: ModeMig}
	}
	mode, registered := a.deviceModes[uuid]
	if !registered {
		mode = a.podMode
		if mode == modeMps {
			mode = ModeHamiCore
		}
	}
	reservation, reserved := a.reservations[slot{container, index}]
	switch {
	case a.err != nil && mode != ModeHamiCore:
		return Device{UUID: uuid, Reason: ReasonMigReservationInvalid}
	case reserved && reservation.GPUUUID == uuid && (mode == ModeMig || mode == ""):
		return Device{
			UUID:    uuid,
			Mode:    ModeMig,
			Profile: reservation.Profile,
			Start:   int32(reservation.Placement.Start),
			Size:    int32(reservation.Placement.Size),
		}
	case reserved:
		// A reservation for another GPU, or on one not in MIG mode: HAMi's
		// scheduler treats both as unhealthy.
		return Device{UUID: uuid, Reason: ReasonMigReservationMismatch}
	case mode == ModeHamiCore:
		return Device{UUID: uuid, Mode: mode}
	case mode == ModeMig:
		return Device{UUID: uuid, Reason: ReasonMigReservationMissing}
	default:
		return Device{UUID: uuid, Reason: ReasonDeviceModeUnknown}
	}
}
