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
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVendorOf(t *testing.T) {
	cases := map[string]string{
		"Ascend":        AscendGPUDevice,
		"Ascend310P":    AscendGPUDevice,
		"Ascend910B":    AscendGPUDevice,
		"Ascend910B4":   AscendGPUDevice,
		"Ascend910B4-1": AscendGPUDevice,
		NvidiaGPUDevice: NvidiaGPUDevice,
		HygonGPUDevice:  HygonGPUDevice,
		MetaxGPUDevice:  MetaxGPUDevice,
		MetaxSGPUDevice: MetaxSGPUDevice,
	}
	for in, want := range cases {
		if got := VendorOf(in); got != want {
			t.Errorf("VendorOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDecodePodDevicesAscend910B4(t *testing.T) {
	SupportDevices["Ascend910B4"] = "hami.io/Ascend910B4-devices-allocated"
	defer delete(SupportDevices, "Ascend910B4")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"hami.io/Ascend910B4-devices-allocated": "Ascend910B4-0,Ascend910B4,32768,100:;Ascend910B4-1,Ascend910B4,8192,0:;",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c0"}, {Name: "c1"}},
		},
	}

	pd, err := DecodePodDevices(pod, log.NewHelper(log.DefaultLogger))
	if err != nil {
		t.Fatalf("DecodePodDevices: %v", err)
	}
	devs, ok := pd["Ascend910B4"]
	if !ok {
		t.Fatalf("pod devices missing Ascend910B4 key, got %v", pd)
	}
	if len(devs) != 3 {
		t.Fatalf("expected 3 container positions, got %d", len(devs))
	}
	full := devs[0][0]
	if full.UUID != "Ascend910B4-0" || full.Type != "Ascend910B4" || full.Usedmem != 32768 || full.Usedcores != 100 {
		t.Errorf("whole-card device decoded wrong: %+v", full)
	}
	slice := devs[1][0]
	if slice.Usedmem != 8192 || slice.Usedcores != 25 {
		t.Errorf("vNPU template 8192 should map to 25 cores, got %+v", slice)
	}
	if len(devs[2]) != 0 {
		t.Errorf("trailing empty container position was not preserved: %+v", devs[2])
	}
}
