package devicecatalog

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func parseFixture(t *testing.T, name string) Parsed {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func sharesOf(model AscendModel) map[int64]int32 {
	shares := map[int64]int32{model.MemoryAllocatable: 100}
	for _, template := range model.Templates {
		if share, ok := model.ComputeShare(template); ok {
			shares[template.Memory] = share
		}
	}
	return shares
}

// The shares WebUI hard-coded before reading HAMi's configuration.
var previousTable = map[string]map[int64]int32{
	"Ascend910A":    {2184: 7, 4369: 13, 8738: 27, 17476: 53, 32768: 100},
	"Ascend910B2":   {8192: 13, 16384: 25, 32768: 50, 65536: 100},
	"Ascend910B3":   {16384: 25, 32768: 50, 65536: 100},
	"Ascend910B4":   {8192: 25, 16384: 50, 32768: 100},
	"Ascend910B4-1": {16384: 25, 32768: 50, 65536: 100},
	"Ascend910C":    {16384: 25, 32768: 50, 65536: 100},
	"Ascend310P":    {3072: 13, 6144: 25, 12288: 50, 21527: 100},
}

func TestShippedConfigurationReproducesThePreviousTable(t *testing.T) {
	parsed := parseFixture(t, "hami-v2.10.0.yaml")
	if len(parsed.Issues) != 0 {
		t.Fatalf("issues = %v", parsed.Issues)
	}
	if len(parsed.Ascend) != len(previousTable) {
		t.Fatalf("models = %d, want %d", len(parsed.Ascend), len(previousTable))
	}
	for word, want := range previousTable {
		if got := sharesOf(parsed.Ascend[word]); !reflect.DeepEqual(got, want) {
			t.Errorf("%s shares = %v, want %v", word, got, want)
		}
	}
}

func TestV290ConfigurationHasNoTemplatesFor910C(t *testing.T) {
	parsed := parseFixture(t, "hami-v2.9.0.yaml")
	for word, want := range previousTable {
		if word == "Ascend910C" {
			want = map[int64]int32{65536: 100}
		}
		if got := sharesOf(parsed.Ascend[word]); !reflect.DeepEqual(got, want) {
			t.Errorf("%s shares = %v, want %v", word, got, want)
		}
	}
}

func TestModelFieldsFollowHAMi(t *testing.T) {
	model := parseFixture(t, "hami-v2.10.0.yaml").Ascend["Ascend310P"]
	if model.ChipName != "310P3" || model.ResourceName != "huawei.com/Ascend310P" || model.ResourceMemoryName != "huawei.com/Ascend310P-memory" ||
		model.MemoryAllocatable != 21527 || model.MemoryCapacity != 24576 || model.AICore != 8 || model.AICPU != 7 {
		t.Fatalf("310P = %+v", model)
	}
	if names := []string{model.Templates[0].Name, model.Templates[1].Name, model.Templates[2].Name}; !reflect.DeepEqual(names, []string{"vir01", "vir02", "vir04"}) {
		t.Fatalf("templates = %v", names)
	}
	if !parseFixture(t, "hami-v2.10.0.yaml").Ascend["Ascend910C"].SuperPod {
		t.Fatal("910C superPod was not read")
	}
}

func TestRoundFollowsTrimMemory(t *testing.T) {
	parsed := parseFixture(t, "hami-v2.10.0.yaml")
	tests := []struct {
		model    string
		memory   int64
		rounded  int64
		template string
		ok       bool
	}{
		{"Ascend910B3", 1, 16384, "vir05_1c_16g", true},
		{"Ascend910B3", 16384, 16384, "vir05_1c_16g", true},
		{"Ascend910B3", 16385, 32768, "vir10_3c_32g", true},
		{"Ascend910B3", 65536, 65536, "", true},
		{"Ascend910B3", 65537, 0, "", false},
		{"Ascend310P", 12289, 21527, "", true},
		{"Ascend310P", 24576, 21527, "", true},
		{"Ascend310P", 24577, 0, "", false},
	}
	for _, tt := range tests {
		rounded, template, ok := parsed.Ascend[tt.model].Round(tt.memory)
		if rounded != tt.rounded || template != tt.template || ok != tt.ok {
			t.Errorf("%s Round(%d) = %d %q %v, want %d %q %v", tt.model, tt.memory, rounded, template, ok, tt.rounded, tt.template, tt.ok)
		}
	}
}

func TestParseAcceptsBothLayoutsAndReportsInvalidEntries(t *testing.T) {
	legacy, err := Parse([]byte(`
vnpus:
- commonWord: Custom910
  chipName: 910X
  memoryAllocatable: 1000
  aiCore: 10
  templates:
  - {name: big, memory: 800, aiCore: 8}
  - {name: small, memory: 200, aiCore: 2}
`))
	if err != nil || legacy.Ascend["Custom910"].Templates[0].Name != "small" {
		t.Fatalf("legacy layout = %+v, %v", legacy, err)
	}
	current, err := Parse([]byte(`
vnpus:
  hamiVnpuCore: true
  configs:
  - commonWord: A
    memoryAllocatable: 100
    templates: [{name: t, memory: 50, aiCore: 1}]
  - commonWord: A
    memoryAllocatable: 200
  - chipName: nameless
  - commonWord: B
    memoryAllocatable: 0
    aiCore: 4
    templates: [{name: "", memory: 1}]
`))
	if err != nil || !current.HamiVnpuCore || current.Ascend["A"].MemoryAllocatable != 100 || len(current.Ascend) != 2 {
		t.Fatalf("current layout = %+v, %v", current, err)
	}
	joined := strings.Join(current.Issues, "\n")
	for _, want := range []string{"more than once", "no commonWord", "no positive aiCore", "without a name", "no positive memoryAllocatable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues %q lack %q", joined, want)
		}
	}
	if _, ok := current.Ascend["A"].ComputeShare(current.Ascend["A"].Templates[0]); ok {
		t.Fatal("a share was invented without aiCore")
	}
}

func TestParseIsCaseSensitiveLikeHAMiAndRejectsMalformedYAML(t *testing.T) {
	parsed, err := Parse([]byte("vnpus:\n  configs:\n  - CommonWord: Wrong\n"))
	if err != nil || len(parsed.Ascend) != 0 || len(parsed.Issues) != 1 {
		t.Fatalf("differently cased key was accepted: %+v, %v", parsed, err)
	}
	if _, err := Parse([]byte("vnpus: [")); err == nil {
		t.Fatal("malformed YAML was accepted")
	}
	empty, err := Parse([]byte("nvidia:\n  resourceCountName: nvidia.com/gpu\n"))
	if err != nil || len(empty.Ascend) != 0 {
		t.Fatalf("configuration without vnpus = %+v, %v", empty, err)
	}
}
