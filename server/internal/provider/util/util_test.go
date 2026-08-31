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
	"reflect"
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

	got, err := DecodePodDevices(pod, logger, AscendAllocationModeUnknown)
	if err != nil {
		t.Fatalf("DecodePodDevices() error = %v", err)
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

func Test_DecodePodDevices_Ascend(t *testing.T) {
	// Ensure Ascend device keys are registered in SupportDevices.
	setSupportDeviceForTest(t, "Ascend", "hami.io/Ascend910B-devices-allocated")
	setSupportDeviceForTest(t, "Ascend310P", "hami.io/Ascend310P-devices-allocated")

	logger := log.NewHelper(log.DefaultLogger)

	tests := []struct {
		name    string
		pod     *corev1.Pod
		want    PodDevices
		wantErr bool
	}{
		{
			name: "Ascend310P single container single device",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"hami.io/Ascend310P-devices-allocated": "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
			want: PodDevices{
				"Ascend310P": {
					{
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Ascend310P two containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"hami.io/Ascend310P-devices-allocated": "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:;D7E96E64-214123F1-E8E618E4-AED8030A-E3003039,Ascend310P,6144,100:",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ctr1"},
						{Name: "ctr2"},
					},
				},
			},
			want: PodDevices{
				"Ascend310P": {
					{
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
					{
						{Idx: 0, UUID: "D7E96E64-214123F1-E8E618E4-AED8030A-E3003039", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Ascend310P annotation segments exceed container count - should truncate",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						// Two device segments but only one container in spec.
						"hami.io/Ascend310P-devices-allocated": "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:;D7E96E64-214123F1-E8E618E4-AED8030A-E3003039,Ascend310P,6144,100:",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
			want: PodDevices{
				"Ascend310P": {
					{
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Ascend310P empty segment in middle should produce empty ContainerDevices placeholder",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"hami.io/Ascend310P-devices-allocated": "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:;;D7E96E64-214123F1-E8E618E4-AED8030A-E3003039,Ascend310P,6144,100:",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "ctr1"},
						{Name: "ctr2"},
						{Name: "ctr3"},
					},
				},
			},
			want: PodDevices{
				"Ascend310P": {
					{
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
					{}, // empty segment placeholder
					{
						{Idx: 0, UUID: "D7E96E64-214123F1-E8E618E4-AED8030A-E3003039", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Ascend310P init container placeholder keeps main container aligned",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"hami.io/Ascend310P-devices-allocated": ";E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019,Ascend310P,6144,100:",
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Name: "init"}},
					Containers:     []corev1.Container{{Name: "main"}},
				},
			},
			want: PodDevices{
				"Ascend310P": {
					{},
					{
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25, CoreAllocationKnown: true},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Ascend310P malformed device should produce empty device set",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"hami.io/Ascend310P-devices-allocated": "bad-format-string",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
			want:    PodDevices{"Ascend310P": {}},
			wantErr: false,
		},
		{
			name: "empty annotations should return empty PodDevices",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "main"}},
				},
			},
			want:    PodDevices{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePodDevices(tt.pod, logger, AscendAllocationModeTemplate)
			if (err != nil) != tt.wantErr {
				t.Errorf("DecodePodDevices() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("DecodePodDevices() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeNpuContainerDevicesUsesModeSpecificCoreContract(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		mode      AscendAllocationMode
		wantCore  int32
		wantKnown bool
	}{
		{name: "B3 hami-core uses annotation percentage", input: "B3-soft,Ascend910B3,28672,40:", mode: AscendAllocationModeHamiCore, wantCore: 40, wantKnown: true},
		{name: "B3 hami-core zero remains zero", input: "B3-soft-zero,Ascend910B3,16384,0:", mode: AscendAllocationModeHamiCore, wantCore: 0, wantKnown: true},
		{name: "910A hard template", input: "A-hard,Ascend910A,2184,0:", mode: AscendAllocationModeTemplate, wantCore: 7, wantKnown: true},
		{name: "B2 hard template", input: "B2-hard,Ascend910B2,8192,0:", mode: AscendAllocationModeTemplate, wantCore: 13, wantKnown: true},
		{name: "B4 hard template", input: "B4-hard,Ascend910B4,8192,0:", mode: AscendAllocationModeTemplate, wantCore: 25, wantKnown: true},
		{name: "B4-1 hard template", input: "B4-1-hard,Ascend910B4-1,32768,0:", mode: AscendAllocationModeTemplate, wantCore: 50, wantKnown: true},
		{name: "C hard full card", input: "C-hard,Ascend910C,65536,0:", mode: AscendAllocationModeTemplate, wantCore: 100, wantKnown: true},
		{name: "unknown hard template is unavailable", input: "B4-custom,Ascend910B4,4096,0:", mode: AscendAllocationModeTemplate, wantCore: 0, wantKnown: false},
		{name: "zero with unresolved mode is unavailable", input: "B4-ambiguous,Ascend910B4,8192,0:", mode: AscendAllocationModeUnknown, wantCore: 0, wantKnown: false},
		{name: "positive core with unresolved mode is unavailable", input: "B3-positive,Ascend910B3,28672,40:", mode: AscendAllocationModeUnknown, wantCore: 0, wantKnown: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices, err := DecodeNpuContainerDevices(tt.input, tt.mode)
			if err != nil {
				t.Fatalf("DecodeNpuContainerDevices() error = %v", err)
			}
			if len(devices) != 1 {
				t.Fatalf("device count = %d, want 1", len(devices))
			}
			if devices[0].Usedcores != tt.wantCore || devices[0].CoreAllocationKnown != tt.wantKnown {
				t.Fatalf("core allocation = (%d, known=%t), want (%d, known=%t)", devices[0].Usedcores, devices[0].CoreAllocationKnown, tt.wantCore, tt.wantKnown)
			}
		})
	}
}

func TestDecodePodDevicesKeepsAscendInitContainerSlot(t *testing.T) {
	const annotation = "hami.io/Ascend910B3-devices-allocated"
	setSupportDeviceForTest(t, "Ascend910B3", annotation)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			annotation: ";B3-main,Ascend910B3,28672,40:;",
		}},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{{Name: "init"}},
			Containers:     []corev1.Container{{Name: "worker"}},
		},
	}

	got, err := DecodePodDevices(pod, log.NewHelper(log.DefaultLogger), AscendAllocationModeHamiCore)
	if err != nil {
		t.Fatalf("DecodePodDevices() error = %v", err)
	}
	slots := got["Ascend910B3"]
	if len(slots) != 2 || len(slots[0]) != 0 || len(slots[1]) != 1 {
		t.Fatalf("Ascend slots = %#v, want empty init slot and one worker device", slots)
	}
	if slots[1][0].UUID != "B3-main" || slots[1][0].Usedcores != 40 {
		t.Fatalf("worker allocation = %#v", slots[1][0])
	}
}

func TestDecodePodDevicesDiscoversFutureAscendCommonWord(t *testing.T) {
	const annotation = "hami.io/AscendFuture-devices-allocated"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			annotation: "future-0,AscendFuture,12345,0:",
		}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
	}

	got, err := DecodePodDevices(pod, log.NewHelper(log.DefaultLogger), AscendAllocationModeTemplate)
	if err != nil {
		t.Fatalf("DecodePodDevices() error = %v", err)
	}
	devices := got["AscendFuture"]
	if len(devices) != 1 || len(devices[0]) != 1 {
		t.Fatalf("future Ascend devices = %#v", devices)
	}
	if devices[0][0].Usedmem != 12345 || devices[0][0].CoreAllocationKnown {
		t.Fatalf("future Ascend allocation = %#v, want memory preserved and core unavailable", devices[0][0])
	}
}

func TestDecodePodDevicesRejectsAscendCommonWordMismatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			"hami.io/Ascend910B3-devices-allocated": "device-0,Ascend910B4,16384,0:",
		}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "worker"}}},
	}

	if _, err := DecodePodDevices(pod, log.NewHelper(log.DefaultLogger), AscendAllocationModeTemplate); err == nil {
		t.Fatal("DecodePodDevices() accepted a device type that did not match the annotation commonWord")
	}
}
