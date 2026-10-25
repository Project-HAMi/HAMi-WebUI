/*
Copyright 2024 The HAMi Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package util

import (
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setSupportDeviceForTest(t *testing.T, deviceType, annotation string) {
	t.Helper()
	previous, existed := SupportDevices[deviceType]
	SupportDevices[deviceType] = annotation
	t.Cleanup(func() {
		if existed {
			SupportDevices[deviceType] = previous
			return
		}
		delete(SupportDevices, deviceType)
	})
}

func TestDecodePodDevicesWithInitContainers(t *testing.T) {
	setSupportDeviceForTest(t, "NVIDIA", "hami.io/vgpu-devices-allocated")
	logger := log.NewHelper(log.DefaultLogger)

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: "init"},
			},
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceName(NVIDIAPriority): resource.MustParse("1"),
						},
					},
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				SupportDevices["NVIDIA"]: ";GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,6144,30:;",
			},
		},
	}

	got, failures := DecodePodDevices(pod, logger)
	if len(failures) != 0 {
		t.Fatalf("DecodePodDevices() failures = %v", failures)
	}
	nvidiaDevices, ok := got["NVIDIA"]
	if !ok {
		t.Fatal("expected NVIDIA devices")
	}
	if len(nvidiaDevices) != 2 {
		t.Fatalf("expected 2 container slots, got %d", len(nvidiaDevices))
	}
	if len(nvidiaDevices[0]) != 0 {
		t.Fatalf("expected empty init container devices, got %+v", nvidiaDevices[0])
	}
	if len(nvidiaDevices[1]) != 1 {
		t.Fatalf("expected 1 device on main container, got %+v", nvidiaDevices[1])
	}
	if nvidiaDevices[1][0].UUID != "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d" {
		t.Fatalf("unexpected device UUID: %s", nvidiaDevices[1][0].UUID)
	}
	if nvidiaDevices[1][0].Priority != "1" {
		t.Fatalf("unexpected priority: %s", nvidiaDevices[1][0].Priority)
	}
}

func TestDecodePodDevicesRejectsMalformedAllocations(t *testing.T) {
	setSupportDeviceForTest(t, "NVIDIA", "hami.io/vgpu-devices-allocated")
	setSupportDeviceForTest(t, "DCU", "hami.io/dcu-devices-allocated")
	setSupportDeviceForTest(t, "Metax-GPU", "metax-tech.com/gpu-devices-allocated")
	setSupportDeviceForTest(t, "MLU", "hami.io/cambricon-mlu-devices-allocated")
	logger := log.NewHelper(log.NewStdLogger(io.Discard))
	for _, tc := range []struct {
		name, devType string
		annotations   map[string]string
	}{
		{"NVIDIA segment without fields", "NVIDIA", map[string]string{"hami.io/vgpu-devices-allocated": "not-an-allocation"}},
		{"NVIDIA non-numeric values", "NVIDIA", map[string]string{"hami.io/vgpu-devices-allocated": "GPU-1,NVIDIA,nan,nan:"}},
		{"NVIDIA negative memory", "NVIDIA", map[string]string{"hami.io/vgpu-devices-allocated": "GPU-1,NVIDIA,-1,30:"}},
		{"NVIDIA memory that overflows", "NVIDIA", map[string]string{"hami.io/vgpu-devices-allocated": "GPU-1,NVIDIA,99999999999,30:"}},
		{"NVIDIA without a device id", "NVIDIA", map[string]string{"hami.io/vgpu-devices-allocated": ",NVIDIA,1024,30:"}},
		{"DCU id without an index", "DCU", map[string]string{"hami.io/dcu-devices-allocated": "DCU-,DCU,1024,50:"}},
		{"MetaX segment without fields", "Metax-GPU", map[string]string{"metax-tech.com/gpu-devices-allocated": "not-an-allocation"}},
		{"Cambricon segment without fields", "MLU", map[string]string{"hami.io/cambricon-mlu-devices-allocated": "node-a-cambricon-mlu-0,MLU"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			annotations := map[string]string{AssignedNodeAnnotations: "node-a"}
			for k, v := range tc.annotations {
				annotations[k] = v
			}
			pod := &corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}, ObjectMeta: metav1.ObjectMeta{Annotations: annotations}}
			got, failures := DecodePodDevices(pod, logger)
			if _, ok := got[tc.devType]; ok {
				t.Fatalf("devices = %+v, want %s left out", got[tc.devType], tc.devType)
			}
			if len(failures) != 1 || failures[0].DeviceType != tc.devType {
				t.Fatalf("failures = %v, want one invalid %s", failures, tc.devType)
			}
		})
	}
}

// Valid shapes keep decoding as before: a trailing separator, an optional fifth field and a zero core request.
func TestDecodeContainerDevicesKeepsValidShapes(t *testing.T) {
	got, err := DecodeContainerDevices("GPU-1,NVIDIA,1024,0,2:GPU-2,NVIDIA,2048,30:", "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].UUID != "GPU-1" || got[0].Usedcores != 100 || got[1].Usedmem != 2048 || got[1].Usedcores != 30 || got[1].Priority != "1" {
		t.Fatalf("devices = %+v", got)
	}
}

func TestDecodeDCUContainerDevicesRejectsIDWithoutIndex(t *testing.T) {
	if _, err := DecodeDCUContainerDevices("DCU,DCU,1024,50", "", "node-a"); err == nil {
		t.Fatal("expected an error for a device id without an index")
	}
}

// One vendor's unreadable annotation must neither crash nor hide the pod's other allocations.
func TestDecodePodDevicesKeepsOtherDeviceTypesWhenOneFails(t *testing.T) {
	setSupportDeviceForTest(t, "NVIDIA", "hami.io/vgpu-devices-allocated")
	setSupportDeviceForTest(t, "DCU", "hami.io/dcu-devices-allocated")
	setSupportDeviceForTest(t, "MLU", "hami.io/cambricon-mlu-devices-allocated")
	logger := log.NewHelper(log.DefaultLogger)
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			AssignedNodeAnnotations:                   "node-a",
			"hami.io/vgpu-devices-allocated":          "GPU-1,NVIDIA,6144,30:;",
			"hami.io/dcu-devices-allocated":           "DCU,DCU,1024,50:;",
			"hami.io/cambricon-mlu-devices-allocated": "node-a-cambricon-mlu-0,MLU,2048,50:;",
		}},
	}

	got, failures := DecodePodDevices(pod, logger)
	if len(got["NVIDIA"]) != 1 || len(got["NVIDIA"][0]) != 1 || got["NVIDIA"][0][0].UUID != "GPU-1" {
		t.Fatalf("NVIDIA devices = %+v, want GPU-1 kept", got["NVIDIA"])
	}
	if _, ok := got["DCU"]; ok {
		t.Fatalf("DCU devices = %+v, want the unreadable type left out", got["DCU"])
	}
	if len(got["MLU"]) != 1 {
		t.Fatalf("MLU devices = %+v, want the Cambricon allocation kept", got["MLU"])
	}
	if len(failures) != 1 || failures[0].DeviceType != "DCU" {
		t.Fatalf("failures = %v, want only DCU", failures)
	}
}
