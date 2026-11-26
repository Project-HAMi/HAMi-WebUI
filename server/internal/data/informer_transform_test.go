package data

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"vgpu/internal/biz"
	"vgpu/internal/podfacts"
)

func sampleMessyPod() *corev1.Pod {
	controller := true
	always := corev1.ContainerRestartPolicyAlways
	claimName := "claim-1"
	templateName := "tmpl-1"
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mlu-exporter-abc",
			Namespace:       "kube-system",
			UID:             types.UID("pod-uid-1"),
			ResourceVersion: "4201",
			Generation:      2,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: appsv1.SchemeGroupVersion.String(),
				Kind:       "DaemonSet",
				Name:       "cambricon-mlu-exporter",
				UID:        types.UID("ds-uid-1"),
				Controller: &controller,
			}},
			Labels: map[string]string{
				"app.kubernetes.io/component": "mlu-exporter",
				"custom-workload":             "train",
			},
			Annotations: map[string]string{
				"hami.io/vgpu-node":              "gpu-node-1",
				"hami.io/node-scheduler-policy":  "binpack",
				"gpu-fraction":                   "0.5",
				"nvidia.com/container.ctr.gpu-memory.request": "4Gi",
				lastAppliedConfigAnnotation:      `{"very":"large-json-blob"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:   "kube-controller-manager",
				Operation: metav1.ManagedFieldsOperationApply,
				FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{"f:metadata":{"f:labels":{}}}`)},
			}},
		},
		Spec: corev1.PodSpec{
			NodeName:          "gpu-node-1",
			RestartPolicy:     corev1.RestartPolicyAlways,
			SchedulerName:     "hami-scheduler",
			PriorityClassName: "high-priority",
			SchedulingGates:   []corev1.PodSchedulingGate{{Name: "gate-1"}},
			NodeSelector:      map[string]string{"gpu": "true"},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{
							MatchExpressions: []corev1.NodeSelectorRequirement{{
								Key: "zone", Operator: corev1.NodeSelectorOpIn, Values: []string{"us-a"},
							}},
						}},
					},
				},
			},
			Tolerations: []corev1.Toleration{{Key: "nvidia.com/gpu", Operator: corev1.TolerationOpExists}},
			TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
				MaxSkew: 1, TopologyKey: "kubernetes.io/hostname", WhenUnsatisfiable: corev1.DoNotSchedule,
			}},
			ResourceClaims: []corev1.PodResourceClaim{
				{Name: "gpu-claim", Source: corev1.ClaimSource{ResourceClaimName: &claimName}},
				{Name: "tmpl-claim", Source: corev1.ClaimSource{ResourceClaimTemplateName: &templateName}},
			},
			Volumes: []corev1.Volume{{Name: "secret-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
			InitContainers: []corev1.Container{{
				Name:          "init-prep",
				Image:         "busybox:1.36",
				RestartPolicy: nil,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
					Limits:   corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
				},
			}, {
				Name:          "sidecar",
				Image:         "envoy:1.30",
				RestartPolicy: &always,
			}},
			Containers: []corev1.Container{{
				Name:    "ctr",
				Image:   "cambricon/cambricon-mlu-exporter:v1.0",
				Command: []string{"/usr/bin/mlu-exporter"},
				Args:    []string{"--strategy", "hami"},
				Env: []corev1.EnvVar{
					{Name: "RESOURCE_REGISTER_STRATEGY", Value: "hami"},
					{Name: "SECRET_TOKEN", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "token"}}},
				},
				EnvFrom: []corev1.EnvFromSource{{Prefix: "APP_"}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
					Limits:   corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")},
					Claims:   []corev1.ResourceClaim{{Name: "gpu-claim"}},
				},
				VolumeMounts: []corev1.VolumeMount{{Name: "secret-vol", MountPath: "/var/run/secret"}},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromInt32(9402),
						},
					},
					PeriodSeconds: 10,
				},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{Command: []string{"true"}},
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase:   corev1.PodRunning,
			Reason:  "Started",
			Message: "running",
			PodIP:   "10.1.2.3",
			PodIPs:  []corev1.PodIP{{IP: "10.1.2.3"}},
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodReady, Status: corev1.ConditionTrue,
			}, {
				Type: corev1.PodScheduled, Status: corev1.ConditionTrue, Message: "scheduled",
			}},
			ResourceClaimStatuses: []corev1.PodResourceClaimStatus{{Name: "tmpl-claim", ResourceClaimName: &claimName}},
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name: "init-prep",
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
				},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:         "ctr",
				ContainerID:  "containerd://abc",
				Ready:        true,
				RestartCount: 2,
				State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
}

func sampleMessyNode() *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "gpu-node-1",
			UID:               types.UID("node-uid-1"),
			ResourceVersion:   "991",
			CreationTimestamp: metav1.NewTime(time.Unix(1700000000, 0).UTC()),
			Labels:            map[string]string{"gpu": "on"},
			Annotations: map[string]string{
				"hami.io/node-nvidia-register": "GPU-1:...",
				lastAppliedConfigAnnotation:    `{"huge":"node-annotation"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{{
				Manager:  "kubelet",
				FieldsV1: &metav1.FieldsV1{Raw: []byte(`{"f:status":{}}`)},
			}},
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true,
			PodCIDR:       "10.244.1.0/24",
			ProviderID:    "gce://proj/zone/gpu-node-1",
			Taints:        []corev1.Taint{{Key: "gpu", Effect: corev1.TaintEffectNoSchedule}},
		},
		Status: corev1.NodeStatus{
			NodeInfo:    corev1.NodeSystemInfo{OSImage: "Ubuntu 22.04", Architecture: "amd64", KubeletVersion: "v1.30.1"},
			Addresses:   []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.8"}},
			Conditions:  []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Capacity:    corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("4")},
			Allocatable: corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("4")},
			Images: []corev1.ContainerImage{
				{Names: []string{"ghcr.io/large/image:v1"}, SizeBytes: 1024 * 1024 * 1024},
				{Names: []string{"ghcr.io/large/image:v2"}, SizeBytes: 2048 * 1024 * 1024},
			},
			VolumesAttached: []corev1.AttachedVolume{{Name: "vol-1", DevicePath: "/dev/sdb"}},
		},
	}
}

func TestTrimInformerObjectIsIdempotentNonMutatingAndTombstoneSafe(t *testing.T) {
	origPod := sampleMessyPod()
	origSnapshot := origPod.DeepCopy()

	trimmedOnceObj, err := trimInformerObject(origPod)
	if err != nil {
		t.Fatalf("trim pod: %v", err)
	}
	if !reflect.DeepEqual(origPod, origSnapshot) {
		t.Fatal("trimInformerObject mutated original Pod")
	}
	trimmedPod := trimmedOnceObj.(*corev1.Pod)
	if trimmedPod == origPod {
		t.Fatal("trimmed Pod must be a detached copy")
	}
	if trimmedPod.ResourceVersion != "4201" || len(trimmedPod.ManagedFields) != 0 || len(trimmedPod.Spec.Volumes) != 0 {
		t.Fatalf("unexpected pod trim result: rv=%q managed=%d vols=%d", trimmedPod.ResourceVersion, len(trimmedPod.ManagedFields), len(trimmedPod.Spec.Volumes))
	}
	if _, exists := trimmedPod.Annotations[lastAppliedConfigAnnotation]; exists {
		t.Fatal("last-applied-configuration should be stripped")
	}
	if trimmedPod.Spec.Containers[0].Env[1].ValueFrom != nil || len(trimmedPod.Spec.Containers[0].VolumeMounts) != 0 {
		t.Fatal("Env.ValueFrom or VolumeMounts should be stripped")
	}
	if trimmedPod.Spec.Containers[0].ReadinessProbe == nil || trimmedPod.Spec.Containers[0].ReadinessProbe.HTTPGet == nil {
		t.Fatal("ReadinessProbe.HTTPGet must be preserved for MLU component discovery")
	}
	if trimmedPod.Spec.Containers[0].LivenessProbe != nil {
		t.Fatal("LivenessProbe should be stripped")
	}

	trimmedTwiceObj, err := trimInformerObject(trimmedPod)
	if err != nil {
		t.Fatalf("second trim pod: %v", err)
	}
	if !reflect.DeepEqual(trimmedPod, trimmedTwiceObj.(*corev1.Pod)) {
		t.Fatal("trimInformerObject is not idempotent for Pod")
	}

	origNode := sampleMessyNode()
	origNodeSnap := origNode.DeepCopy()
	nodeObj, err := trimInformerObject(origNode)
	if err != nil {
		t.Fatalf("trim node: %v", err)
	}
	if !reflect.DeepEqual(origNode, origNodeSnap) {
		t.Fatal("trimInformerObject mutated original Node")
	}
	trimmedNode := nodeObj.(*corev1.Node)
	if trimmedNode.ResourceVersion != "991" || len(trimmedNode.Status.Images) != 0 || len(trimmedNode.ManagedFields) != 0 || trimmedNode.Spec.PodCIDR != "" {
		t.Fatalf("unexpected node trim result: rv=%q imgs=%d", trimmedNode.ResourceVersion, len(trimmedNode.Status.Images))
	}
	nodeTwiceObj, err := trimInformerObject(trimmedNode)
	if err != nil {
		t.Fatalf("second trim node: %v", err)
	}
	if !reflect.DeepEqual(trimmedNode, nodeTwiceObj.(*corev1.Node)) {
		t.Fatal("trimInformerObject is not idempotent for Node")
	}

	tombstone := cache.DeletedFinalStateUnknown{Key: "kube-system/mlu-exporter-abc", Obj: trimmedPod}
	gotTombstone, err := trimInformerObject(tombstone)
	if err != nil || !reflect.DeepEqual(gotTombstone, tombstone) {
		t.Fatalf("tombstone passthrough failed: %v %+v", err, gotTombstone)
	}
}

func TestTrimInformerObjectConcurrentResyncSafe(t *testing.T) {
	orig := sampleMessyPod()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				out, err := trimInformerObject(orig)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				p := out.(*corev1.Pod)
				p.Annotations["mutated-by-caller"] = "yes"
			}
		}()
	}
	wg.Wait()
	if _, leaked := orig.Annotations["mutated-by-caller"]; leaked {
		t.Fatal("caller mutation leaked back to original cached Pod")
	}
}

func TestTrimmedObjectsPreserveClusterPodFactsAndSchedulingSemantics(t *testing.T) {
	rawPod := sampleMessyPod()
	trimmedObj, err := trimInformerObject(rawPod)
	if err != nil {
		t.Fatal(err)
	}
	trimmedPod := trimmedObj.(*corev1.Pod)

	// 1. podfacts equivalence
	opts := podfacts.Options{ResourceNames: []corev1.ResourceName{"nvidia.com/gpu"}}
	rawFacts, err := podfacts.Extract(rawPod, podfacts.Evidence{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	trimmedFacts, err := podfacts.Extract(trimmedPod, podfacts.Evidence{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rawFacts, trimmedFacts) {
		t.Fatalf("podfacts mismatch after trim:\nraw=%+v\ntrimmed=%+v", rawFacts, trimmedFacts)
	}

	// 2. cluster.Components equivalence
	compFor := func(p *corev1.Pod) biz.ClusterComponents {
		idx := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		if err := idx.Add(p); err != nil {
			t.Fatal(err)
		}
		repo := &clusterRepo{pods: &podRepo{podLister: listerscorev1.NewPodLister(idx)}}
		comp, err := repo.Components(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return comp
	}
	if !reflect.DeepEqual(compFor(rawPod), compFor(trimmedPod)) {
		t.Fatal("cluster.Components result diverged after SetTransform")
	}

	// 3. schedulingPodView equivalence
	resNames, _ := schedulingResources(nil)
	schedRepo := &podRepo{schedulingResourceNames: resNames}
	if !reflect.DeepEqual(schedRepo.schedulingPod(rawPod), schedRepo.schedulingPod(trimmedPod)) {
		t.Fatal("schedulingPodView diverged after SetTransform")
	}
}

func TestTrimInformerObject5kPodMemoryReduction(t *testing.T) {
	basePod := sampleMessyPod()
	// Add realistic heavy volumes & managedFields padding (~2 KiB stripped per Pod)
	for i := 0; i < 6; i++ {
		basePod.Spec.Volumes = append(basePod.Spec.Volumes, corev1.Volume{
			Name: "data-" + string(rune('a'+i)),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "very-long-configmap-name-padding-0123456789"}},
			},
		})
		basePod.Spec.Containers[0].VolumeMounts = append(basePod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name: "data-" + string(rune('a'+i)), MountPath: "/mnt/data/long/path/" + string(rune('a'+i)),
		})
	}
	basePod.Annotations[lastAppliedConfigAnnotation] = string(make([]byte, 2048))

	rawBytesTotal := 0
	trimmedBytesTotal := 0
	rawSample, _ := json.Marshal(basePod)
	trimmedSampleObj, _ := trimInformerObject(basePod)
	trimmedSample, _ := json.Marshal(trimmedSampleObj)

	const count = 5000
	rawBytesTotal = len(rawSample) * count
	trimmedBytesTotal = len(trimmedSample) * count

	if trimmedBytesTotal*100 >= rawBytesTotal*65 {
		t.Fatalf("expected >35%% reduction across %d pods, raw=%d trimmed=%d", count, rawBytesTotal, trimmedBytesTotal)
	}
	t.Logf("5000 Pods serialized memory: raw=%.2f MiB -> trimmed=%.2f MiB (%.1f%% reduction)",
		float64(rawBytesTotal)/(1024*1024),
		float64(trimmedBytesTotal)/(1024*1024),
		100.0*(1.0-float64(trimmedBytesTotal)/float64(rawBytesTotal)),
	)
}
