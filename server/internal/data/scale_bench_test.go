package data

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"vgpu/internal/podfacts"
	"vgpu/internal/resourceview"
)

func buildRealisticScalePods(podCount, nodeCount int) []*corev1.Pod {
	lastApplied := strings.Repeat(`{"apiVersion":"v1","kind":"Pod","metadata":{"labels":{"app":"train"}}}`, 32)
	pods := make([]*corev1.Pod, podCount)
	now := metav1.Now()
	for i := range podCount {
		nodeName := fmt.Sprintf("gpu-node-%03d", i%nodeCount)
		podUID := types.UID(fmt.Sprintf("pod-uid-%05d", i))
		env := make([]corev1.EnvVar, 12)
		for e := range env {
			env[e] = corev1.EnvVar{
				Name:  fmt.Sprintf("ENV_KEY_%02d", e),
				Value: "0123456789abcdef0123456789abcdef",
			}
		}
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       "ml-training",
				Name:            fmt.Sprintf("trainer-%05d", i),
				UID:             podUID,
				ResourceVersion: fmt.Sprintf("%d", 10000+i),
				CreationTimestamp: now,
				Labels: map[string]string{
					"app.kubernetes.io/name": "trainer",
					"team":                   "ai-platform",
				},
				Annotations: map[string]string{
					lastAppliedConfigAnnotation:        lastApplied,
					"hami.io/vgpu-devices-allocated":   fmt.Sprintf("GPU-%03d,NVIDIA,4096,25:;", (i%nodeCount)*8+(i%8)),
					"hami.io/vgpu-time":                "1700000000",
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1",
					Kind:       "StatefulSet",
					Name:       "trainer",
					UID:        types.UID("owner-uid-1"),
				}},
				ManagedFields: []metav1.ManagedFieldsEntry{{
					Manager:   "kube-scheduler",
					Operation: metav1.ManagedFieldsOperationUpdate,
					FieldsV1:  &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:nodeName":{}}}`)},
				}},
			},
			Spec: corev1.PodSpec{
				NodeName:      nodeName,
				SchedulerName: "hami-scheduler",
				Volumes: []corev1.Volume{{
					Name: "shm",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
					},
				}},
				Containers: []corev1.Container{{
					Name:  "worker",
					Image: "registry.example.com/ml/pytorch:2.5.1-cuda12.4",
					Env:   env,
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "shm",
						MountPath: "/dev/shm",
					}},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							"nvidia.com/gpu":    resource.MustParse("1"),
							"nvidia.com/gpumem": resource.MustParse("4096"),
						},
					},
				}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: fmt.Sprintf("10.244.%d.%d", (i/250)%250, i%250+1),
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:        "worker",
					ContainerID: fmt.Sprintf("containerd://%064d", i),
					Ready:       true,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{StartedAt: now},
					},
				}},
			},
		}
	}
	return pods
}

func buildScaleCaptureInputs(podCount, nodeCount, devicesPerNode int) (resourceview.Snapshot, podfacts.Inventory) {
	nodes := make([]resourceview.Node, nodeCount)
	devices := make([]resourceview.Device, 0, nodeCount*devicesPerNode)
	for n := range nodeCount {
		nodeName := fmt.Sprintf("gpu-node-%03d", n)
		nodeUID := types.UID(fmt.Sprintf("node-uid-%03d", n))
		nodes[n] = resourceview.Node{
			UID:  nodeUID,
			Name: nodeName,
		}
		for d := range devicesPerNode {
			devices = append(devices, resourceview.Device{
				Identity: resourceview.DeviceIdentity{
					Provider: "NVIDIA",
					Node:     nodeName,
					NodeUID:  nodeUID,
					DeviceID: fmt.Sprintf("GPU-%03d-%d", n, d),
				},
				Vendor: "NVIDIA",
				Model:  "A100",
			})
		}
	}
	entries := make([]podfacts.Entry, podCount)
	for p := range podCount {
		podUID := types.UID(fmt.Sprintf("pod-uid-%05d", p))
		entries[p] = podfacts.Entry{
			Facts: &podfacts.Facts{
				Metadata: metav1.ObjectMeta{
					Namespace:       "ml-training",
					Name:            fmt.Sprintf("trainer-%05d", p),
					UID:             podUID,
					ResourceVersion: fmt.Sprintf("%d", 10000+p),
				},
				NodeName: fmt.Sprintf("gpu-node-%03d", p%nodeCount),
				Containers: []podfacts.Container{{
					Key:  podfacts.ContainerKey{PodUID: podUID, Name: "worker"},
					Kind: podfacts.Regular,
				}},
			},
		}
	}
	snap := resourceview.Snapshot{
		At:      time.Unix(1700000000, 0).UTC(),
		Nodes:   nodes,
		Devices: devices,
	}
	inv := podfacts.Inventory{Entries: entries}
	return snap, inv
}

func BenchmarkInformerTransformPods(b *testing.B) {
	for _, tc := range []struct {
		name      string
		podCount  int
		nodeCount int
	}{
		{name: "5000_pods", podCount: 5000, nodeCount: 100},
		{name: "10000_pods", podCount: 10000, nodeCount: 200},
	} {
		b.Run(tc.name, func(b *testing.B) {
			rawPods := buildRealisticScalePods(tc.podCount, tc.nodeCount)
			trimmed := make([]*corev1.Pod, len(rawPods))

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				for i, p := range rawPods {
					trimmed[i] = trimInformerPod(p)
				}
			}
		})
	}
}

func BenchmarkClusterCapturePublish(b *testing.B) {
	for _, tc := range []struct {
		name      string
		podCount  int
		nodeCount int
	}{
		{name: "5000_pods_100_nodes", podCount: 5000, nodeCount: 100},
		{name: "10000_pods_200_nodes", podCount: 10000, nodeCount: 200},
	} {
		b.Run(tc.name, func(b *testing.B) {
			snap, inv := buildScaleCaptureInputs(tc.podCount, tc.nodeCount, 8)
			store := NewCaptureStore()
			ts := time.Unix(1700000001, 0).UTC()

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				capSnap := store.Publish("rv-bench", snap, inv, ts)
				runtime.KeepAlive(capSnap)
			}
		})
	}
}

func BenchmarkClusterCaptureConcurrentLookup(b *testing.B) {
	const (
		podCount       = 10000
		nodeCount      = 200
		devicesPerNode = 8
	)
	snap, inv := buildScaleCaptureInputs(podCount, nodeCount, devicesPerNode)
	store := NewCaptureStore()
	store.Publish("rv-bench", snap, inv, time.Unix(1700000001, 0).UTC())

	probeDeviceID, err := resourceview.DeviceID("NVIDIA", "gpu-node-042", "GPU-042-3")
	if err != nil {
		b.Fatal(err)
	}
	probeNodeUID := types.UID("node-uid-042")
	probePodUID := types.UID("pod-uid-04242")
	probeCtrKey := podfacts.ContainerKey{PodUID: probePodUID, Name: "worker"}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			capture := store.Load()
			if _, ok := capture.LookupDevice(probeDeviceID); !ok {
				b.Fatal("LookupDevice miss")
			}
			if _, ok := capture.LookupNode(probeNodeUID); !ok {
				b.Fatal("LookupNode miss")
			}
			if _, ok := capture.LookupNodeByName("gpu-node-042"); !ok {
				b.Fatal("LookupNodeByName miss")
			}
			if _, ok := capture.LookupPod(probePodUID); !ok {
				b.Fatal("LookupPod miss")
			}
			if _, ok := capture.LookupContainer(probeCtrKey); !ok {
				b.Fatal("LookupContainer miss")
			}
		}
	})
}
