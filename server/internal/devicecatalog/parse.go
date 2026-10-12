package devicecatalog

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v2"
)

// yaml.v2 and HAMi's tags match keys exactly as the scheduler does (pkg/device/ascend/vnpu.go).
type rawTemplate struct {
	Name   string `yaml:"name"`
	Memory int64  `yaml:"memory"`
	AICore int32  `yaml:"aiCore,omitempty"`
	AICPU  int32  `yaml:"aiCPU,omitempty"`
}

type rawVNPUConfig struct {
	CommonWord         string        `yaml:"commonWord"`
	ChipName           string        `yaml:"chipName"`
	ResourceName       string        `yaml:"resourceName"`
	ResourceMemoryName string        `yaml:"resourceMemoryName"`
	ResourceCoreName   string        `yaml:"resourceCoreName"`
	MemoryAllocatable  int64         `yaml:"memoryAllocatable"`
	MemoryCapacity     int64         `yaml:"memoryCapacity"`
	MemoryFactor       int32         `yaml:"memoryFactor"`
	AICore             int32         `yaml:"aiCore"`
	AICPU              int32         `yaml:"aiCPU"`
	Templates          []rawTemplate `yaml:"templates"`
	SuperPod           bool          `yaml:"superPod"`
}

type rawVNPUs struct {
	HamiVnpuCore bool
	Configs      []rawVNPUConfig
}

// HAMi v2.9 moved vnpus from a list to {hamiVnpuCore, configs}.
func (v *rawVNPUs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var list []rawVNPUConfig
	if err := unmarshal(&list); err == nil {
		v.Configs = list
		return nil
	}
	var current struct {
		HamiVnpuCore bool            `yaml:"hamiVnpuCore"`
		Configs      []rawVNPUConfig `yaml:"configs"`
	}
	if err := unmarshal(&current); err != nil {
		return err
	}
	v.HamiVnpuCore, v.Configs = current.HamiVnpuCore, current.Configs
	return nil
}

type Parsed struct {
	HamiVnpuCore bool
	Ascend       map[string]AscendModel
	Issues       []string
}

// Parse skips invalid entries and reports them as issues; only malformed YAML is an error.
func Parse(data []byte) (Parsed, error) {
	var document struct {
		VNPUs rawVNPUs `yaml:"vnpus"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Parsed{}, fmt.Errorf("parse device-config.yaml: %w", err)
	}
	parsed := Parsed{HamiVnpuCore: document.VNPUs.HamiVnpuCore, Ascend: map[string]AscendModel{}}
	for i, raw := range document.VNPUs.Configs {
		if raw.CommonWord == "" {
			parsed.Issues = append(parsed.Issues, fmt.Sprintf("vnpus entry %d has no commonWord", i))
			continue
		}
		if _, duplicate := parsed.Ascend[raw.CommonWord]; duplicate {
			parsed.Issues = append(parsed.Issues, fmt.Sprintf("vnpus commonWord %q is defined more than once; the first entry is used", raw.CommonWord))
			continue
		}
		model := AscendModel{
			CommonWord:         raw.CommonWord,
			ChipName:           raw.ChipName,
			ResourceName:       raw.ResourceName,
			ResourceMemoryName: raw.ResourceMemoryName,
			ResourceCoreName:   raw.ResourceCoreName,
			MemoryAllocatable:  raw.MemoryAllocatable,
			MemoryCapacity:     raw.MemoryCapacity,
			MemoryFactor:       raw.MemoryFactor,
			AICore:             raw.AICore,
			AICPU:              raw.AICPU,
			SuperPod:           raw.SuperPod,
		}
		for _, template := range raw.Templates {
			if template.Name == "" || template.Memory <= 0 {
				parsed.Issues = append(parsed.Issues, fmt.Sprintf("vnpus %q has a template without a name or positive memory", raw.CommonWord))
				continue
			}
			model.Templates = append(model.Templates, AscendTemplate(template))
		}
		sort.SliceStable(model.Templates, func(a, b int) bool { return model.Templates[a].Memory < model.Templates[b].Memory })
		if model.MemoryAllocatable <= 0 {
			parsed.Issues = append(parsed.Issues, fmt.Sprintf("vnpus %q has no positive memoryAllocatable", raw.CommonWord))
		}
		if model.AICore <= 0 && len(model.Templates) > 0 {
			parsed.Issues = append(parsed.Issues, fmt.Sprintf("vnpus %q has no positive aiCore, so template compute shares are unknown", raw.CommonWord))
		}
		parsed.Ascend[raw.CommonWord] = model
	}
	return parsed, nil
}
