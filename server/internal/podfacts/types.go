// Package podfacts extracts immutable-in-practice Pod facts for the new ledger.
// It performs no I/O and does not change legacy repositories, DTOs or metrics.
package podfacts

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

type ContainerKind string

const (
	Regular       ContainerKind = "regular"
	Init          ContainerKind = "init"
	NativeSidecar ContainerKind = "native_sidecar"
	Ephemeral     ContainerKind = "ephemeral"
)

// ContainerKey is a logical identity. Runtime container IDs and restarts do not
// change it. Kubernetes requires names to be unique across all container kinds.
type ContainerKey struct {
	PodUID types.UID
	Name   string
}

type Candidate string

const (
	CandidateAccelerator Candidate = "accelerator_evidence"
	CandidateUnresolved  Candidate = "recognition_incomplete"
	// NoAcceleratorEvidence is not proof that the Pod cannot access a device.
	NoAcceleratorEvidence Candidate = "no_accelerator_evidence"
)

// UnreadableAllocation identifies cards touched by a damaged source. An empty
// DeviceID means any card from this provider may be affected.
type UnreadableAllocation struct{ Provider, DeviceID string }

type Facts struct {
	UnreadableAllocations []UnreadableAllocation `json:"-"`
	Metadata              metav1.ObjectMeta      // informer identity, deletion timestamp and allocation node annotation
	NodeName              string
	Terminal              bool
	// HAMi v2.11 releases ordinary init reservations together and keeps restartable sidecars.
	InitUsageReleased  bool
	hasInitReservation bool // this UID has had a decodable HAMi allocation
	Containers         []Container
	Requests           []Request // Pod-scoped annotations; never copied into every container
	Candidate          Candidate
	CandidateEvidence  []string
	Issues             []Issue
}

type Container struct {
	Key  ContainerKey
	Kind ContainerKind
	// Index is in its original spec array. Preserve init order for later peak accounting.
	Index       int
	Claims      []ClaimReference
	Requests    []Request
	Allocations []AllocationSet
	States      map[string]*viewv1.FieldState
}

type Request struct {
	Resource string // empty when a declaration cannot yet map to a resource key
	Field    string // requests, limits or exact annotation key
	Raw      string // observed canonical Quantity text or exact annotation string
	Source   string // pod_spec or kai_annotation; does not identify the allocator
	State    *viewv1.FieldState
	Positive bool
}

type ClaimReference struct {
	LocalName    string
	ClaimName    string
	TemplateName string
	Source       string             // pod_spec or dra_extended_resource
	State        *viewv1.FieldState // unresolved until the referenced request is classified
}

// Quantity carries a decoded allocation quantity and its actual unit. This
// layer neither converts units nor fabricates quantities from requests.
type Quantity = resourceview.Quantity

type Allocation struct {
	ComputeUnlimited *bool
	NodeUID          types.UID // observed allocation-context Node UID; missing cannot join a replacement
	Node             string    // allocation source node, independent of Pod binding
	RawDeviceID      string    // original provider ID when physical association is unverified
	Container        string
	RecordID         string
	Resource         string
	DeviceID         string // provider identity; not the public API's opaque device ID
	Quantities       map[string]Quantity
	// Configuration, actual mode and record completeness are independent facts.
	ConfiguredMode *string
	AllocationMode *string
	RawMode        string // retain unrecognized ENPU/aliases verbatim
	Template       *string
	States         map[string]*viewv1.FieldState
}

// AllocationSnapshot is one source's evidence for this exact Pod snapshot.
// Complete describes only this source, never all possible allocation sources.
const AccountingPodPeak = "pod_peak"

type AllocationSnapshot struct {
	HasAllocationEvidence bool   // source observed a nonempty allocation record, even when decoding failed
	Accounting            string // explicit source policy; empty never enables Pod-peak accounting
	PodUID                types.UID
	ResourceVersion       string
	Source                *viewv1.SourceState
	Complete              bool
	Records               []Allocation
}
type AllocationSet struct {
	Accounting string
	Source     *viewv1.SourceState
	Complete   bool
	Records    []Allocation
}

// Evidence binds decoded allocations to the informer Pod snapshot.
type Evidence struct {
	UnreadableAllocations []UnreadableAllocation
	Allocations           []AllocationSnapshot
}
type Issue struct {
	Field  string
	Reason string
}

// Options accepts exact configured accelerator names in addition to the known
// NVIDIA/AMD resource families. It never guesses by "gpu" suffix or vendor label.
type Options struct{ ResourceNames []corev1.ResourceName }
