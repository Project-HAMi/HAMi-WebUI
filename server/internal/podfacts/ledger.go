package podfacts

import (
	"slices"
	"sort"
	"sync"

	"k8s.io/apimachinery/pkg/types"

	viewv1 "vgpu/api/observability/v1"
	"vgpu/internal/resourceview"
)

type Entry struct {
	Assignment    NodeAssignment `json:"-"`
	InputRevision string         `json:"-"` // producer evidence identity, not a public resource version
	Facts         *Facts
	Occupancies   []Occupancy
}
type Inventory struct {
	Entries  []Entry
	Coverage *viewv1.Coverage
}

// Ledger owns immutable per-Pod facts. Only the informer can establish coverage.
type Ledger struct {
	mu          sync.RWMutex
	pods        map[types.UID]*Facts
	names       map[string]types.UID
	revisions   map[types.UID]string
	assignments map[types.UID]NodeAssignment
}

func NewLedger() *Ledger {
	return &Ledger{pods: map[types.UID]*Facts{}, names: map[string]types.UID{}, revisions: map[types.UID]string{}, assignments: map[types.UID]NodeAssignment{}}
}

// Put runs the cache-current predicate inside the commit lock. Use the same
// guard for event updates and catalog replays, so neither restores old objects.
func (l *Ledger) Put(f *Facts, current func() bool) bool { return l.PutWithRevision(f, current, "") }

// PutWithRevision binds facts to the producer's exact dependency snapshot.
// Readers may use this opaque local token to detect pending dependent replays.
func (l *Ledger) PutWithRevision(f *Facts, current func() bool, revision string, assignment ...NodeAssignment) bool {
	if f == nil || f.Metadata.UID == "" || current == nil {
		return false
	}
	copy := Clone(f)
	l.mu.Lock()
	defer l.mu.Unlock()
	if !current() {
		return false
	}
	key := f.Metadata.Namespace + "/" + f.Metadata.Name
	if previous := l.names[key]; previous != "" && previous != f.Metadata.UID {
		delete(l.pods, previous)
		delete(l.revisions, previous)
		delete(l.assignments, previous)
	}
	l.names[key] = f.Metadata.UID
	// The scheduler keeps a release once observed for this UID. While
	// terminating it preserves the cached reservation, including an init peak.
	if previous := l.pods[f.Metadata.UID]; previous != nil && previous.hasInitReservation {
		copy.hasInitReservation = true
		if previous.InitUsageReleased || copy.Metadata.DeletionTimestamp != nil {
			copy.InitUsageReleased = previous.InitUsageReleased
		}
	}
	l.pods[f.Metadata.UID] = copy
	l.revisions[f.Metadata.UID] = revision
	if len(assignment) > 0 {
		l.assignments[f.Metadata.UID] = assignment[0]
	}
	return true
}
func (l *Ledger) Delete(uid types.UID) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if f := l.pods[uid]; f != nil {
		key := f.Metadata.Namespace + "/" + f.Metadata.Name
		if l.names[key] == uid {
			delete(l.names, key)
		}
		delete(l.pods, uid)
		delete(l.revisions, uid)
		delete(l.assignments, uid)
	}
}
func (l *Ledger) Get(uid types.UID) (Entry, bool) {
	l.mu.RLock()
	f := Clone(l.pods[uid])
	revision := l.revisions[uid]
	assignment := l.assignments[uid]
	l.mu.RUnlock()
	if f == nil {
		return Entry{}, false
	}
	return Entry{Assignment: assignment, InputRevision: revision, Facts: f, Occupancies: EffectiveOccupancies(f)}, true
}

func (l *Ledger) Snapshot() Inventory { return l.snapshot(Clone) }

// AllocationSnapshot keeps every Pod identity for informer continuity. Device
// accounting does not need the declarations of terminal or ordinary CPU Pods.
func (l *Ledger) AllocationSnapshot() Inventory {
	return l.snapshot(func(f *Facts) *Facts {
		view := *f
		if f.Terminal || !slices.ContainsFunc(f.Containers, func(c Container) bool {
			return len(c.Requests) != 0 || len(c.Claims) != 0 || len(c.Allocations) != 0
		}) {
			view.Containers = nil
		}
		return Clone(&view)
	})
}

func (l *Ledger) snapshot(clone func(*Facts) *Facts) Inventory {
	l.mu.RLock()
	entries := make([]Entry, 0, len(l.pods))
	for uid, f := range l.pods {
		entries = append(entries, Entry{Facts: f, InputRevision: l.revisions[uid], Assignment: l.assignments[uid]})
	}
	l.mu.RUnlock()
	// Put replaces immutable records; detach them outside the lock so readers
	// do not block informer updates while copying and aggregating the inventory.
	for i := range entries {
		entries[i].Facts = clone(entries[i].Facts)
		entries[i].Occupancies = EffectiveOccupancies(entries[i].Facts)
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := &entries[i].Facts.Metadata, &entries[j].Facts.Metadata
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.UID < b.UID
	})
	return Inventory{Entries: entries, Coverage: &viewv1.Coverage{Entity: "pod", Observed: int32(len(entries)), Sources: []*viewv1.SourceState{{Source: "shared_pod_informer", State: resourceview.Unknown, Reason: "unverified", Params: map[string]string{"continuity": "not_tracked"}}}}}
}

func Clone(f *Facts) *Facts {
	if f == nil {
		return nil
	}
	out := *f
	out.UnreadableAllocations = slices.Clone(f.UnreadableAllocations)
	out.Metadata = *f.Metadata.DeepCopy()
	out.CandidateEvidence = append([]string(nil), f.CandidateEvidence...)
	out.Issues = append([]Issue(nil), f.Issues...)
	out.Requests = copyRequests(f.Requests)
	out.Containers = make([]Container, len(f.Containers))
	for i, c := range f.Containers {
		v := c
		v.States = copyStates(c.States)
		v.Requests = copyRequests(c.Requests)
		v.Claims = append(c.Claims[:0:0], c.Claims...)
		for j := range v.Claims {
			v.Claims[j].State = cloneState(c.Claims[j].State)
		}
		v.Allocations = make([]AllocationSet, len(c.Allocations))
		for j, set := range c.Allocations {
			copied := set
			copied.Source = cloneSource(set.Source)
			copied.Records = make([]Allocation, len(set.Records))
			for k, record := range set.Records {
				copied.Records[k], _ = cloneAllocation(record, set.Source.Source)
			}
			v.Allocations[j] = copied
		}
		out.Containers[i] = v
	}
	return &out
}
func copyStates(states map[string]*viewv1.FieldState) map[string]*viewv1.FieldState {
	result := map[string]*viewv1.FieldState{}
	for key, state := range states {
		result[key] = cloneState(state)
	}
	return result
}
func copyRequests(requests []Request) []Request {
	out := append(requests[:0:0], requests...)
	for i := range out {
		out[i].State = cloneState(requests[i].State)
	}
	return out
}

func (l *Ledger) NodeAssignment(uid types.UID) NodeAssignment {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.assignments[uid]
}

// NodeAllocation reads the assignment and release state from the same immutable
// Pod revision. Only callers accounting for init containers need phase peaks.
func (l *Ledger) NodeAllocation(uid types.UID, version, inputRevision string, includePeaks bool) (NodeAllocation, bool) {
	l.mu.RLock()
	f, assignment, revision := l.pods[uid], l.assignments[uid], l.revisions[uid]
	l.mu.RUnlock()
	if f == nil || f.Metadata.ResourceVersion != version || revision != inputRevision {
		return NodeAllocation{}, false
	}
	a := NodeAllocation{UID: assignment.UID, InitReleased: f.InitUsageReleased, Unreadable: slices.Clone(f.UnreadableAllocations)}
	if includePeaks {
		a.Peaks = EffectiveOccupancies(f)
	}
	return a, true
}

type NodeAllocation struct {
	Unreadable   []UnreadableAllocation
	UID          types.UID
	InitReleased bool
	Peaks        []Occupancy
}
