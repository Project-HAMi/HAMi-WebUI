package podfacts

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"vgpu/internal/resourceview"
)

func fixture() *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "work", UID: "pod-uid", ResourceVersion: "10", CreationTimestamp: metav1.NewTime(time.Date(2026, 11, 13, 1, 2, 3, 0, time.UTC))}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "fixture/app:v1"}}}}
}
func extract(t *testing.T, pod *corev1.Pod, evidence Evidence, options Options) *Facts {
	t.Helper()
	facts, err := Extract(pod, evidence, options)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}
func quantity(value string) *resource.Quantity { result := resource.MustParse(value); return &result }
func textPointer(value string) *string         { return &value }
func container(f *Facts, name string) *Container {
	for i := range f.Containers {
		if f.Containers[i].Key.Name == name {
			return &f.Containers[i]
		}
	}
	return nil
}
func TestEveryPodAndContainerKindRetainsStableIdentity(t *testing.T) {
	p := fixture()
	always := corev1.ContainerRestartPolicyAlways
	p.Spec.InitContainers = []corev1.Container{{Name: "sidecar", RestartPolicy: &always}, {Name: "initialize"}}
	p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug"}, TargetContainerName: "app"}}
	p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", RestartCount: 3, ContainerID: "containerd://run-3", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.NewTime(time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC))}}}}
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{{Name: "sidecar", RestartCount: 1}}
	p.Status.EphemeralContainerStatuses = []corev1.ContainerStatus{{Name: "debug", ContainerID: "containerd://debug"}}
	before := p.DeepCopy()
	f := extract(t, p, Evidence{}, Options{})
	if f.Candidate != NoAcceleratorEvidence || len(f.Containers) != 4 {
		t.Fatalf("CPU Pod was discarded or misclassified: %+v", f)
	}
	for i, kind := range []ContainerKind{NativeSidecar, Init, Regular, Ephemeral} {
		if f.Containers[i].Kind != kind || f.Containers[i].Key.PodUID != p.UID {
			t.Fatal("kind or UID lost")
		}
	}
	if f.Containers[1].Index != 1 || container(f, "debug").States["resources"].State != resourceview.NotApplicable {
		t.Fatal("container ordering or ephemeral semantics lost")
	}
	app := container(f, "app")
	p.Status.ContainerStatuses[0].ContainerID = "containerd://run-4"
	p.Status.ContainerStatuses[0].RestartCount = 4
	restarted := extract(t, p, Evidence{}, Options{})
	if container(restarted, "app").Key != app.Key {
		t.Fatal("restart changed logical identity")
	}
	p.UID = "replacement-uid"
	replacement := extract(t, p, Evidence{}, Options{})
	if container(replacement, "app").Key == app.Key {
		t.Fatal("same-name replacement reused old UID")
	}
	f.Metadata.Name = "changed"
	if p.Name != "worker" || p.Status.ContainerStatuses[0].RestartCount != 4 {
		t.Fatal("facts alias source Pod")
	}
	if !reflect.DeepEqual(before.Spec, p.Spec) {
		t.Fatal("extraction changed Pod spec")
	}
}
func TestOnlyAllocationMetadataIsRetained(t *testing.T) {
	p := fixture()
	p.Annotations = map[string]string{"hami.io/vgpu-node": "node-a", "example.com/unrelated": "ignored"}
	p.Labels = map[string]string{"example.com/app": "worker"}
	f := extract(t, p, Evidence{}, Options{})
	if !reflect.DeepEqual(f.Metadata.Annotations, map[string]string{"hami.io/vgpu-node": "node-a"}) || len(f.Metadata.Labels) != 0 {
		t.Fatal("ledger must retain only evidence used in allocation matching")
	}
	f.Metadata.Annotations["hami.io/vgpu-node"] = "mutated"
	if p.Annotations["hami.io/vgpu-node"] != "node-a" {
		t.Fatal("retained metadata aliases informer")
	}
}
func TestCandidateRequestsDoNotBecomeAllocationsOrSources(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		candidate   Candidate
		configured  bool
	}{
		{"nvidia.com/gpu", "1", CandidateAccelerator, false}, {"amd.com/gpu-cpx-nps1", "1", CandidateAccelerator, false},
		{"nvidia.com/mig-1g.10gb", "1", CandidateAccelerator, false}, {"custom.example/accelerator", "2", CandidateAccelerator, true},
		{"rdma/hca_shared_devices_a", "1", NoAcceleratorEvidence, false}, {"example.com/my-gpu", "1", NoAcceleratorEvidence, false},
		{"nvidia.com/gpu", "0", NoAcceleratorEvidence, false}, {"nvidia.com/gpu", "-1", CandidateUnresolved, false},
	} {
		t.Run(tc.name+tc.value, func(t *testing.T) {
			p := fixture()
			p.Spec.NodeName = "node-a"
			p.Spec.SchedulerName = "hami-scheduler"
			p.Spec.Containers[0].Resources.Limits = corev1.ResourceList{corev1.ResourceName(tc.name): resource.MustParse(tc.value)}
			options := Options{}
			if tc.configured {
				options.ResourceNames = []corev1.ResourceName{corev1.ResourceName(tc.name)}
			}
			f := extract(t, p, Evidence{}, options)
			if f.Candidate != tc.candidate {
				t.Fatalf("candidate %s", f.Candidate)
			}
			if len(container(f, "app").Allocations) != 0 || container(f, "app").States["allocations"].State != resourceview.Unknown {
				t.Fatal("request/binding/scheduler fabricated allocation or system identity")
			}
			if tc.name == "nvidia.com/gpu" && tc.value == "0" {
				r := container(f, "app").Requests[0]
				if r.Raw != "0" || r.State.State != resourceview.Known || r.Positive {
					t.Fatal("known zero was not preserved")
				}
			}
		})
	}
}
func TestKAIAnnotationScopeAndMalformedValues(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		candidate  Candidate
		container  bool
	}{
		{"gpu-fraction", "0.5", CandidateAccelerator, false}, {"gpu-memory", "4096", CandidateAccelerator, false},
		{"gpu-fraction", "NaN", CandidateUnresolved, false}, {"gpu-fraction", "2", CandidateUnresolved, false},
		{"nvidia.com/container.app.gpu-memory.request", "4Gi", CandidateAccelerator, true},
		{"nvidia.com/container.app.gpu-memory.limit", "bad", CandidateUnresolved, true},
	} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			p := fixture()
			p.Annotations = map[string]string{tc.key: tc.value}
			f := extract(t, p, Evidence{}, Options{})
			if f.Candidate != tc.candidate {
				t.Fatal(f.Candidate)
			}
			if tc.container {
				if len(f.Requests) != 0 || len(container(f, "app").Requests) != 1 {
					t.Fatal("container scope lost")
				}
			} else if len(f.Requests) != 1 || len(container(f, "app").Requests) != 0 {
				t.Fatal("Pod fraction copied to a container")
			}
			if len(container(f, "app").Allocations) != 0 {
				t.Fatal("annotation request turned into actual allocation")
			}
		})
	}
}
func TestUsedClaimsRemainUnresolved(t *testing.T) {
	p := fixture()
	p.Spec.Containers[0].Resources.Claims = []corev1.ResourceClaim{{Name: "gpu"}}
	p.Spec.ResourceClaims = []corev1.PodResourceClaim{{Name: "gpu", Source: corev1.ClaimSource{ResourceClaimName: textPointer("claim-a")}}}
	f := extract(t, p, Evidence{}, Options{})
	claims := container(f, "app").Claims
	if f.Candidate != CandidateUnresolved || len(claims) != 1 || claims[0].ClaimName != "claim-a" || claims[0].State.State != resourceview.Unknown {
		t.Fatal("claim reference must not prove allocation", claims)
	}
	p.Spec.Containers[0].Resources.Claims = nil
	if extract(t, p, Evidence{}, Options{}).Candidate != NoAcceleratorEvidence {
		t.Fatal("unused claim classified as a consumer")
	}
}
func TestEphemeralCannotInventIndependentRequests(t *testing.T) {
	p := fixture()
	p.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", Resources: corev1.ResourceRequirements{Limits: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}}}, TargetContainerName: "app"}}
	f := extract(t, p, Evidence{}, Options{})
	debug := container(f, "debug")
	if f.Candidate != NoAcceleratorEvidence || len(f.Issues) != 1 || len(debug.Requests) != 0 {
		t.Fatal("unsupported ephemeral resource field accepted as a declaration")
	}
}
func TestKAIUnknownContainerReferenceRemainsVisibleButUnresolved(t *testing.T) {
	p := fixture()
	p.Annotations = map[string]string{"nvidia.com/container.missing.gpu-memory.request": "2Gi"}
	f := extract(t, p, Evidence{}, Options{})
	if f.Candidate != CandidateUnresolved || len(f.Requests) != 1 || f.Requests[0].State.Params["container"] != "missing" || len(container(f, "app").Requests) != 0 {
		t.Fatal("unresolved annotation was hidden or assigned to another container")
	}
}
