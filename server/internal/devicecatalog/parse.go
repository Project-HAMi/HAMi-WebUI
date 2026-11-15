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
	ResourceNames map[string]DeviceResourceNames
	HamiVnpuCore  bool
	Cambricon     *CambriconConfig
	Ascend        map[string]AscendModel
	Issues        []string

	NodeResourceNames map[string]bool
}

// Parse skips invalid entries and reports them as issues; only malformed YAML is an error.
func Parse(data []byte) (Parsed, error) {
	var document struct {
		Mthreads *DeviceResourceNames `yaml:"mthreads"`
		Nvidia   *DeviceResourceNames `yaml:"nvidia"`
		Hygon    *DeviceResourceNames `yaml:"hygon"`
		HCU      *DeviceResourceNames `yaml:"hcu"`
		Metax    *struct {
			Count    string `yaml:"resourceCountName"`
			VCount   string `yaml:"resourceVCountName"`
			VMemory  string `yaml:"resourceVMemoryName"`
			VCompute string `yaml:"resourceVCoreName"`
		} `yaml:"metax"`
		VNPUs     rawVNPUs         `yaml:"vnpus"`
		Cambricon *CambriconConfig `yaml:"cambricon"`
		Other     map[string]any   `yaml:",inline"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Parsed{}, fmt.Errorf("parse device-config.yaml: %w", err)
	}
	parsed := Parsed{Cambricon: document.Cambricon, HamiVnpuCore: document.VNPUs.HamiVnpuCore, Ascend: map[string]AscendModel{}}
	parsed.NodeResourceNames = nodeResourceNames(document.Other)
	parsed.ResourceNames = map[string]DeviceResourceNames{}
	for kind, names := range map[string]*DeviceResourceNames{"NVIDIA": document.Nvidia, "DCU": document.Hygon, "HCU": document.HCU, "Mthreads": document.Mthreads} {
		if names != nil {
			parsed.ResourceNames[kind] = *names
		}
	}
	if document.Metax != nil {
		parsed.ResourceNames["Metax-GPU"] = DeviceResourceNames{Count: document.Metax.Count}
		parsed.ResourceNames["Metax-SGPU"] = DeviceResourceNames{Count: document.Metax.VCount, Memory: document.Metax.VMemory, Compute: document.Metax.VCompute}
	}
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

// These HAMi config fields name accelerator resources but do not identify
// devices. Keep them separate from the provider mappings used for Pod accounting.
func nodeResourceNames(sections map[string]any) map[string]bool {
	names := map[string]bool{}
	add := func(section any, fields ...string) {
		config, _ := section.(map[any]any)
		for _, field := range fields {
			if name, ok := config[field].(string); ok && name != "" {
				names[name] = true
			}
		}
	}
	chips, _ := sections["iluvatars"].([]any)
	for _, chip := range chips {
		add(chip, "resourceCountName", "resourceMemoryName", "resourceCoreName")
	}
	add(sections["enflame"], "resourceNameGCU", "resourceNameDRSGCU", "resourceNameGCUMemory", "resourceNameGCUCore", "resourceNameVGCU", "resourceNameVGCUPercentage")
	add(sections["kunlun"], "resourceCountName", "resourceVCountName", "resourceVMemoryName")
	add(sections["awsneuron"], "resourceCountName", "resourceCoreName")
	add(sections["amd"], "resourceCountName", "resourceCoreName", "resourceMemoryName", "resourceMemoryPercentageName")
	add(sections["vastai"], "resourceCountName")
	add(sections["biren"], "resourceCountName")
	return names
}
