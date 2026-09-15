package ascend

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/util"

	corev1 "k8s.io/api/core/v1"
)

const (
	VNPUModeAnnotation     = "huawei.com/vnpu-mode"
	VNPUModeHamiCore       = "hami-core"
	NodeHamiCoreAnnotation = "hami-vnpu-core"

	registerAnnotationPrefix  = "hami.io/node-register-"
	handshakeAnnotationPrefix = "hami.io/node-handshake-"
	allocationKeyPrefix       = "hami.io/"
	allocationKeySuffix       = "-devices-allocated"
	templateAnnotationPrefix  = "huawei.com/"
	// Used only to recognise models the device configuration does not list.
	legacyCommonWordPrefix = "Ascend"
)

// Allocation modes, as the device plugin applies them.
const (
	ModeSoft     = "soft"
	ModeTemplate = "template"
	ModeUnknown  = "unknown"
)

// Policies for annotation-less Pods on hami-vnpu-core nodes.
const (
	PolicyUnknown  = "unknown"
	PolicyNode     = "node"
	PolicyTemplate = "template"
)

// Shapes and the reasons a compute share is unknown.
const (
	ShapeWhole    = "whole"
	ShapeTemplate = "template"
	ShapeSoft     = "soft"
	ShapeUnknown  = "unknown"

	ReasonCatalogUnavailable    = "catalog_unavailable"
	ReasonModelNotConfigured    = "model_not_configured"
	ReasonTemplateNotConfigured = "template_not_configured"
	ReasonComputeNotConfigured  = "compute_not_configured"
	ReasonModeAmbiguous         = "mode_ambiguous"
	ReasonNodeModeUnknown       = "node_mode_unknown"
)

// ResolveMode follows the device plugin; the ambiguous case is Project-HAMi/ascend-device-plugin#134.
func ResolveMode(podMode, nodeHamiCore, policy string) (mode, reason string) {
	switch {
	case podMode == VNPUModeHamiCore:
		return ModeSoft, ""
	case podMode != "", nodeHamiCore == "false":
		return ModeTemplate, ""
	case nodeHamiCore == "true" && policy == PolicyNode:
		return ModeSoft, ""
	case nodeHamiCore == "true" && policy == PolicyTemplate:
		return ModeTemplate, ""
	case nodeHamiCore == "true":
		return ModeUnknown, ReasonModeAmbiguous
	default:
		return ModeUnknown, ReasonNodeModeUnknown
	}
}

type Facts struct {
	CommonWord    string
	Memory        int64
	AnnotatedCore int32
	Template      string
	// HAMi listed the device in huawei.com/<commonWord>, where it names a template only for template allocations.
	Recorded   bool
	CardMemory int64
	NodeRead   bool
	Mode       string
	ModeReason string
}

type Result struct {
	Shape    string
	Template string
	Cores    int32
	Known    bool
	Reason   string
}

// Interpret never guesses: without the configuration, template shares stay unknown.
func Interpret(snapshot *devicecatalog.Snapshot, facts Facts, policy string) Result {
	mode, reason := facts.Mode, facts.ModeReason
	// As in HAMi's scheduler, a node without the annotation follows the configured default.
	if mode == ModeUnknown && reason == ReasonNodeModeUnknown && facts.NodeRead && snapshot.Loaded() {
		mode, reason = ResolveMode("", strconv.FormatBool(snapshot.HamiVnpuCore), policy)
	}
	switch mode {
	case ModeSoft:
		return Result{Shape: ShapeSoft, Cores: facts.AnnotatedCore, Known: true}
	case ModeTemplate:
	default:
		return Result{Shape: ShapeUnknown, Template: facts.Template, Reason: reason}
	}
	model, configured := snapshot.AscendModel(facts.CommonWord)
	if facts.Recorded && facts.Template == "" {
		return Result{Shape: ShapeWhole, Cores: 100, Known: true}
	}
	if !facts.Recorded && facts.Memory > 0 && (configured && facts.Memory == model.MemoryAllocatable || facts.Memory == facts.CardMemory) {
		return Result{Shape: ShapeWhole, Cores: 100, Known: true}
	}
	result := Result{Shape: ShapeTemplate, Template: facts.Template}
	switch {
	case !snapshot.Loaded():
		result.Reason = ReasonCatalogUnavailable
		return result
	case !configured:
		result.Reason = ReasonModelNotConfigured
		return result
	}
	if result.Template == "" {
		for _, template := range model.Templates {
			if template.Memory == facts.Memory {
				result.Template = template.Name
				break
			}
		}
	}
	template, ok := model.Template(result.Template)
	if !ok {
		result.Reason = ReasonTemplateNotConfigured
		return result
	}
	if result.Cores, ok = model.ComputeShare(template); !ok {
		result.Reason = ReasonComputeNotConfigured
		return result
	}
	result.Known = true
	return result
}

type Device struct {
	Index int
	UUID  string
	Facts Facts
}

type Decoder struct {
	Catalog devicecatalog.Source
	Policy  string
}

// Snapshot is nil, meaning not loaded, when no catalog is wired.
func (d Decoder) Snapshot() *devicecatalog.Snapshot {
	if d.Catalog == nil {
		return nil
	}
	return d.Catalog.Snapshot()
}

// CommonWords includes unlisted Ascend* words so stale allocations stay visible.
func (d Decoder) CommonWords(annotations map[string]string) []string {
	seen := map[string]bool{}
	for _, word := range d.Snapshot().AscendCommonWords() {
		if _, ok := annotations[allocationKey(word)]; ok {
			seen[word] = true
		}
	}
	for key := range annotations {
		if word, ok := allocationWord(key); ok && strings.HasPrefix(word, legacyCommonWordPrefix) {
			seen[word] = true
		}
	}
	words := make([]string, 0, len(seen))
	for word := range seen {
		words = append(words, word)
	}
	sort.Strings(words)
	return words
}

// Decode returns container slots, init containers first; node may be nil. A
// malformed annotation drops only its own model and is reported in the error.
func (d Decoder) Decode(pod *corev1.Pod, node *corev1.Node) (map[string][][]Device, error) {
	words := d.CommonWords(pod.Annotations)
	if len(words) == 0 {
		return nil, nil
	}
	nodeHamiCore := ""
	if node != nil {
		nodeHamiCore = node.Annotations[NodeHamiCoreAnnotation]
	}
	mode, modeReason := ResolveMode(pod.Annotations[VNPUModeAnnotation], nodeHamiCore, d.Policy)
	slotCount := len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	result := make(map[string][][]Device, len(words))
	var errs []error
	for _, word := range words {
		cardMemory := registeredMemory(node, word)
		templates := allocatedTemplates(pod.Annotations[templateAnnotationPrefix+word])
		slots, err := d.decodeWord(pod, word, slotCount)
		if err != nil {
			errs = append(errs, fmt.Errorf("decode %s: %w", allocationKey(word), err))
			continue
		}
		for _, devices := range slots {
			for j := range devices {
				facts := &devices[j].Facts
				facts.CardMemory = cardMemory[devices[j].UUID]
				facts.Template, facts.Recorded = templates.lookup(devices[j].UUID, facts.Memory)
				facts.NodeRead = node != nil
				facts.Mode, facts.ModeReason = mode, modeReason
			}
		}
		result[word] = slots
	}
	return result, errors.Join(errs...)
}

func (d Decoder) decodeWord(pod *corev1.Pod, word string, slotCount int) ([][]Device, error) {
	slots := make([][]Device, 0, slotCount)
	for i, value := range strings.Split(pod.Annotations[allocationKey(word)], util.OnePodMultiContainerSplitSymbol) {
		if i >= slotCount {
			break
		}
		devices, err := decodeSlot(value, word)
		if err != nil {
			return nil, err
		}
		slots = append(slots, devices)
	}
	return slots, nil
}

func allocationKey(word string) string {
	return allocationKeyPrefix + word + allocationKeySuffix
}

func allocationWord(key string) (string, bool) {
	if !strings.HasPrefix(key, allocationKeyPrefix) || !strings.HasSuffix(key, allocationKeySuffix) {
		return "", false
	}
	word := strings.TrimSuffix(strings.TrimPrefix(key, allocationKeyPrefix), allocationKeySuffix)
	return word, word != ""
}

func decodeSlot(value, word string) ([]Device, error) {
	devices := []Device{}
	for i, entry := range strings.Split(value, util.OneContainerMultiDeviceSplitSymbol) {
		if !strings.Contains(entry, ",") {
			continue
		}
		fields := strings.Split(entry, ",")
		if len(fields) < 4 {
			return nil, fmt.Errorf("device %q has fewer than four fields", entry)
		}
		if fields[1] != word {
			return nil, fmt.Errorf("device type %q, want %q", fields[1], word)
		}
		memory, err := strconv.ParseInt(fields[2], 10, 32)
		if err != nil || memory < 0 {
			return nil, fmt.Errorf("invalid memory field %q", fields[2])
		}
		core, err := strconv.ParseInt(fields[3], 10, 32)
		if err != nil || core < 0 {
			return nil, fmt.Errorf("invalid core field %q", fields[3])
		}
		devices = append(devices, Device{Index: i, UUID: fields[0], Facts: Facts{CommonWord: word, Memory: memory, AnnotatedCore: int32(core)}})
	}
	return devices, nil
}

func registeredMemory(node *corev1.Node, word string) map[string]int64 {
	memory := map[string]int64{}
	if node == nil {
		return memory
	}
	devices, err := util.UnMarshalNodeDevices(node.Annotations[registerAnnotationPrefix+word])
	if err != nil {
		return memory
	}
	for _, device := range devices {
		if device != nil && device.Devmem > 0 {
			memory[device.ID] = int64(device.Devmem)
		}
	}
	return memory
}

// huawei.com/<commonWord> is one list per Pod, so entries are matched by UUID and memory.
type templateIndex map[string][]allocatedTemplate

type allocatedTemplate struct {
	UUID   string `json:"UUID"`
	Temp   string `json:"temp"`
	Memory int64  `json:"memory"`
}

func allocatedTemplates(value string) templateIndex {
	var entries []allocatedTemplate
	if value == "" || json.Unmarshal([]byte(value), &entries) != nil {
		return nil
	}
	index := templateIndex{}
	for _, entry := range entries {
		index[entry.UUID] = append(index[entry.UUID], entry)
	}
	return index
}

func (index templateIndex) lookup(uuid string, memory int64) (template string, recorded bool) {
	for _, entry := range index[uuid] {
		if entry.Memory == memory {
			return entry.Temp, true
		}
	}
	return "", false
}
