package ascend

import (
	"os"
	"reflect"
	"testing"

	"vgpu/internal/devicecatalog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func loadedCatalog(t *testing.T, fixture string) *devicecatalog.Snapshot {
	t.Helper()
	data, err := os.ReadFile("../../devicecatalog/testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := devicecatalog.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return &devicecatalog.Snapshot{State: devicecatalog.StateLoaded, Ascend: parsed.Ascend, HamiVnpuCore: parsed.HamiVnpuCore}
}

func TestResolveModeFollowsTheDevicePlugin(t *testing.T) {
	tests := []struct {
		podMode, nodeHamiCore, policy string
		mode, reason                  string
	}{
		{VNPUModeHamiCore, "false", PolicyUnknown, ModeSoft, ""},
		{"template", "true", PolicyNode, ModeTemplate, ""},
		{"", "false", PolicyNode, ModeTemplate, ""},
		{"", "true", PolicyUnknown, ModeUnknown, ReasonModeAmbiguous},
		{"", "true", "", ModeUnknown, ReasonModeAmbiguous},
		{"", "true", PolicyNode, ModeSoft, ""},
		{"", "true", PolicyTemplate, ModeTemplate, ""},
		{"", "", PolicyNode, ModeUnknown, ReasonNodeModeUnknown},
	}
	for _, tt := range tests {
		if mode, reason := ResolveMode(tt.podMode, tt.nodeHamiCore, tt.policy); mode != tt.mode || reason != tt.reason {
			t.Errorf("ResolveMode(%q, %q, %q) = %s %s, want %s %s", tt.podMode, tt.nodeHamiCore, tt.policy, mode, reason, tt.mode, tt.reason)
		}
	}
}

func TestInterpretMatchesHAMiShippedTemplates(t *testing.T) {
	catalog := loadedCatalog(t, "hami-v2.10.0.yaml")
	tests := []struct {
		word     string
		memory   int64
		template string
		cores    int32
	}{
		{"Ascend910A", 2184, "vir02", 7},
		{"Ascend910B2", 8192, "vir03_1c_8g", 13},
		{"Ascend910B4", 8192, "vir05_1c_8g", 25},
		{"Ascend910B4-1", 32768, "vir10_3c_32g", 50},
		{"Ascend310P", 6144, "vir02", 25},
	}
	for _, tt := range tests {
		got := Interpret(catalog, Facts{CommonWord: tt.word, Memory: tt.memory, Mode: ModeTemplate}, PolicyUnknown)
		if want := (Result{Shape: ShapeTemplate, Template: tt.template, Cores: tt.cores, Known: true}); got != want {
			t.Errorf("%s %d = %+v, want %+v", tt.word, tt.memory, got, want)
		}
	}
}

func TestInterpretShapesAndReasons(t *testing.T) {
	catalog := loadedCatalog(t, "hami-v2.10.0.yaml")
	unloaded := &devicecatalog.Snapshot{State: devicecatalog.StateForbidden}
	custom := loadedCatalog(t, "hami-v2.10.0.yaml")
	model := custom.Ascend["Ascend910B3"]
	model.AICore = 0
	custom.Ascend = map[string]devicecatalog.AscendModel{"Ascend910B3": model}
	tests := []struct {
		name     string
		snapshot *devicecatalog.Snapshot
		facts    Facts
		policy   string
		want     Result
	}{
		{"whole card from the catalog", catalog, Facts{CommonWord: "Ascend910C", Memory: 65536, Mode: ModeTemplate}, "", Result{Shape: ShapeWhole, Cores: 100, Known: true}},
		{"whole card from the registration without a catalog", nil, Facts{CommonWord: "Ascend910B3", Memory: 65536, CardMemory: 65536, Mode: ModeTemplate}, "", Result{Shape: ShapeWhole, Cores: 100, Known: true}},
		{"soft split keeps the requested share", catalog, Facts{CommonWord: "Ascend910B3", Memory: 28672, AnnotatedCore: 40, Mode: ModeSoft}, "", Result{Shape: ShapeSoft, Cores: 40, Known: true}},
		{"soft split without reservation", nil, Facts{CommonWord: "Ascend910B3", Memory: 16384, Mode: ModeSoft}, "", Result{Shape: ShapeSoft, Known: true}},
		{"operator template missing from the catalog", catalog, Facts{CommonWord: "Ascend910B4", Memory: 4096, Mode: ModeTemplate}, "", Result{Shape: ShapeTemplate, Reason: ReasonTemplateNotConfigured}},
		{"unreadable catalog", unloaded, Facts{CommonWord: "Ascend910B3", Memory: 16384, Template: "vir05_1c_16g", Mode: ModeTemplate}, "", Result{Shape: ShapeTemplate, Template: "vir05_1c_16g", Reason: ReasonCatalogUnavailable}},
		{"model missing from the catalog", catalog, Facts{CommonWord: "Ascend910X", Memory: 16384, Mode: ModeTemplate}, "", Result{Shape: ShapeTemplate, Reason: ReasonModelNotConfigured}},
		{"template without AI Cores", custom, Facts{CommonWord: "Ascend910B3", Memory: 16384, Mode: ModeTemplate}, "", Result{Shape: ShapeTemplate, Template: "vir05_1c_16g", Reason: ReasonComputeNotConfigured}},
		{"annotation-less Pod on a soft node", catalog, Facts{CommonWord: "Ascend910B3", Memory: 16384, Mode: ModeUnknown, ModeReason: ReasonModeAmbiguous}, "", Result{Shape: ShapeUnknown, Reason: ReasonModeAmbiguous}},
		{"a node without the annotation follows the configured default", catalog, Facts{CommonWord: "Ascend910B3", Memory: 16384, NodeRead: true, Mode: ModeUnknown, ModeReason: ReasonNodeModeUnknown}, "", Result{Shape: ShapeTemplate, Template: "vir05_1c_16g", Cores: 25, Known: true}},
		{"an unread node is not guessed", catalog, Facts{CommonWord: "Ascend910B3", Memory: 16384, Mode: ModeUnknown, ModeReason: ReasonNodeModeUnknown}, "", Result{Shape: ShapeUnknown, Reason: ReasonNodeModeUnknown}},
		{"a node without the annotation and no catalog", nil, Facts{CommonWord: "Ascend910B3", Memory: 16384, NodeRead: true, Mode: ModeUnknown, ModeReason: ReasonNodeModeUnknown}, "", Result{Shape: ShapeUnknown, Reason: ReasonNodeModeUnknown}},
		{"HAMi's record without a template is a whole card despite registration drift", catalog, Facts{CommonWord: "Ascend910B3", Memory: 61440, CardMemory: 65536, Recorded: true, Mode: ModeTemplate}, "", Result{Shape: ShapeWhole, Cores: 100, Known: true}},
		{"without HAMi's record, the configured allocatable memory is a whole card", catalog, Facts{CommonWord: "Ascend910B3", Memory: 65536, CardMemory: 61440, Mode: ModeTemplate}, "", Result{Shape: ShapeWhole, Cores: 100, Known: true}},
		{"the applied template name wins over memory", catalog, Facts{CommonWord: "Ascend910B3", Memory: 20000, Template: "vir10_3c_32g", Mode: ModeTemplate}, "", Result{Shape: ShapeTemplate, Template: "vir10_3c_32g", Cores: 50, Known: true}},
	}
	for _, tt := range tests {
		if got := Interpret(tt.snapshot, tt.facts, tt.policy); got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func ascendPod(annotations map[string]string, initContainers, containers int) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
	for i := 0; i < initContainers; i++ {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{})
	}
	for i := 0; i < containers; i++ {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{})
	}
	return pod
}

func slotShape(slots [][]Device) []int {
	shape := make([]int, len(slots))
	for i, slot := range slots {
		shape[i] = len(slot)
	}
	return shape
}

func TestDecodeKeepsContainerSlots(t *testing.T) {
	const key = "hami.io/Ascend310P-devices-allocated"
	const a, b = "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:", "D7E96E64-214123F1-E8E618E4-AED8030A-E3003039,Ascend310P,6144,100:"
	tests := []struct {
		name       string
		value      string
		init, main int
		want       []int
	}{
		{"one container", a, 0, 1, []int{1}},
		{"two containers", a + ";" + b, 0, 2, []int{1, 1}},
		{"segments beyond the containers", a + ";" + b, 0, 1, []int{1}},
		{"empty middle segment", a + ";;" + b, 0, 3, []int{1, 0, 1}},
		{"init container placeholder", ";" + a, 1, 1, []int{0, 1}},
		{"malformed device", "bad-format-string", 0, 1, []int{0}},
	}
	decoder := Decoder{}
	for _, tt := range tests {
		got, err := decoder.Decode(ascendPod(map[string]string{key: tt.value}, tt.init, tt.main), nil)
		if err != nil || !reflect.DeepEqual(slotShape(got["Ascend310P"]), tt.want) {
			t.Errorf("%s: slots %v, %v; want %v", tt.name, slotShape(got["Ascend310P"]), err, tt.want)
		}
	}
	got, _ := decoder.Decode(ascendPod(map[string]string{key: ";" + a}, 1, 1), nil)
	if device := got["Ascend310P"][1][0]; device.UUID != "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019" || device.Facts.Memory != 6144 || device.Facts.AnnotatedCore != 100 {
		t.Fatalf("device = %+v", device)
	}
}

func TestDecodeDropsOnlyTheMalformedModel(t *testing.T) {
	pod := ascendPod(map[string]string{
		"hami.io/Ascend910B3-devices-allocated": "device-0,Ascend910B4,16384,0:",
		"hami.io/Ascend310P-devices-allocated":  "device-1,Ascend310P,6144,0:",
	}, 0, 1)
	got, err := (Decoder{}).Decode(pod, nil)
	if err == nil {
		t.Fatal("a device type that contradicts its annotation was accepted")
	}
	if _, ok := got["Ascend910B3"]; ok || len(got["Ascend310P"]) != 1 {
		t.Fatalf("decoded = %v", got)
	}
}

func TestDecodeFindsModelsByConfigurationAndRecordsFacts(t *testing.T) {
	catalog := loadedCatalog(t, "hami-v2.10.0.yaml")
	catalog.Ascend["Custom910"] = devicecatalog.AscendModel{CommonWord: "Custom910", MemoryAllocatable: 1000}
	pod := ascendPod(map[string]string{
		"hami.io/Custom910-devices-allocated":    "C-0,Custom910,500,0:",
		"hami.io/Ascend910B3-devices-allocated":  ";B3-0,Ascend910B3,16384,0:B3-1,Ascend910B3,32768,0:B3-2,Ascend910B3,65536,0:",
		"huawei.com/Ascend910B3":                 `[{"UUID":"B3-0","temp":"vir05_1c_16g","memory":16384},{"UUID":"B3-1","temp":"vir10_3c_32g","memory":32768},{"UUID":"B3-2","memory":65536}]`,
		"hami.io/AscendFuture-devices-allocated": "F-0,AscendFuture,1,0:",
		"hami.io/vgpu-devices-allocated":         "GPU-0,NVIDIA,1024,10:",
	}, 1, 1)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
		NodeHamiCoreAnnotation:              "false",
		"hami.io/node-register-Ascend910B3": `[{"id":"B3-0","count":4,"devmem":65536,"devcore":20,"type":"Ascend910B3","health":true}]`,
	}}}
	decoder := Decoder{Catalog: devicecatalog.Static{Current: catalog}}
	if words := decoder.CommonWords(pod.Annotations); !reflect.DeepEqual(words, []string{"Ascend910B3", "AscendFuture", "Custom910"}) {
		t.Fatalf("commonWords = %v", words)
	}
	got, err := decoder.Decode(pod, node)
	if err != nil {
		t.Fatal(err)
	}
	first, second, whole := got["Ascend910B3"][1][0].Facts, got["Ascend910B3"][1][1].Facts, got["Ascend910B3"][1][2].Facts
	if first.Template != "vir05_1c_16g" || !first.Recorded || !first.NodeRead || first.CardMemory != 65536 || first.Mode != ModeTemplate || second.Template != "vir10_3c_32g" || second.CardMemory != 0 {
		t.Fatalf("facts = %+v %+v", first, second)
	}
	if whole.Template != "" || !whole.Recorded || Interpret(catalog, whole, "").Shape != ShapeWhole {
		t.Fatalf("whole-card facts = %+v", whole)
	}
	if len(got["Custom910"]) != 1 || len(got["AscendFuture"]) != 1 {
		t.Fatalf("decoded models = %v", got)
	}
}
