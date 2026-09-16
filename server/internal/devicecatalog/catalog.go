// Package devicecatalog reads device models from HAMi's device configuration.
package devicecatalog

import (
	"sort"
	"time"
)

type State string

const (
	StateDisabled  State = "disabled"
	StateLoading   State = "loading"
	StateLoaded    State = "loaded"
	StateMissing   State = "missing"
	StateForbidden State = "forbidden"
	StateInvalid   State = "invalid"
	StateError     State = "error"
)

type Ref struct {
	Namespace string
	Name      string
	Key       string
}

type AscendTemplate struct {
	Name   string
	Memory int64
	AICore int32
	AICPU  int32
}

type AscendModel struct {
	CommonWord         string
	ChipName           string
	ResourceName       string
	ResourceMemoryName string
	ResourceCoreName   string
	MemoryAllocatable  int64
	MemoryCapacity     int64
	MemoryFactor       int32
	AICore             int32
	AICPU              int32
	Templates          []AscendTemplate // ascending memory, as HAMi sorts them
	SuperPod           bool
}

// Snapshot is an immutable view of the catalog.
type Snapshot struct {
	State           State
	Reason          string
	Ref             Ref
	ResourceVersion string
	ObservedAt      time.Time
	HamiVnpuCore    bool
	Ascend          map[string]AscendModel
	Issues          []string
}

type Source interface {
	Snapshot() *Snapshot
	// Subscribe runs fn after every snapshot change.
	Subscribe(fn func(previous, current *Snapshot))
}

type Static struct{ Current *Snapshot }

func (s Static) Snapshot() *Snapshot                         { return s.Current }
func (s Static) Subscribe(func(previous, current *Snapshot)) {}

func (s *Snapshot) Loaded() bool {
	return s != nil && s.State == StateLoaded
}

func (s *Snapshot) AscendModel(commonWord string) (AscendModel, bool) {
	if !s.Loaded() {
		return AscendModel{}, false
	}
	model, ok := s.Ascend[commonWord]
	return model, ok
}

// AscendCommonWords lists configured commonWords in a stable order.
func (s *Snapshot) AscendCommonWords() []string {
	if !s.Loaded() {
		return nil
	}
	words := make([]string, 0, len(s.Ascend))
	for word := range s.Ascend {
		words = append(words, word)
	}
	sort.Strings(words)
	return words
}

func (m AscendModel) Template(name string) (AscendTemplate, bool) {
	for _, template := range m.Templates {
		if template.Name == name {
			return template, true
		}
	}
	return AscendTemplate{}, false
}

// AscendResource reports the model and role (count, memory or core) of an extended resource name.
func (s *Snapshot) AscendResource(name string) (commonWord, role string, ok bool) {
	if !s.Loaded() || name == "" {
		return "", "", false
	}
	for word, model := range s.Ascend {
		switch name {
		case model.ResourceName:
			return word, "count", true
		case model.ResourceMemoryName:
			return word, "memory", true
		case model.ResourceCoreName:
			return word, "core", true
		}
	}
	return "", "", false
}

// PairsModules reports whether HAMi allocates modules of two NPUs, which it does only for a superPod Ascend910C.
func (m AscendModel) PairsModules() bool {
	return m.SuperPod && m.CommonWord == "Ascend910C"
}

// Round mirrors HAMi's trimMemory; ok is false where HAMi rejects the request.
func (m AscendModel) Round(memory int64) (rounded int64, template string, ok bool) {
	for _, candidate := range m.Templates {
		if memory <= candidate.Memory {
			return candidate.Memory, candidate.Name, true
		}
	}
	if memory <= m.MemoryCapacity {
		return m.MemoryAllocatable, "", true
	}
	return 0, "", false
}

// ComputeShare is the template's AI Core share in percent, rounded half up.
func (m AscendModel) ComputeShare(template AscendTemplate) (int32, bool) {
	if m.AICore <= 0 || template.AICore <= 0 {
		return 0, false
	}
	return (template.AICore*100 + m.AICore/2) / m.AICore, true
}
