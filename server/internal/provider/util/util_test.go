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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var inRequestDevices map[string]string

func init() {
	inRequestDevices = make(map[string]string)
	inRequestDevices["NVIDIA"] = "hami.io/vgpu-devices-to-allocate"
}

//func TestEmptyContainerDevicesCoding(t *testing.T) {
//	cd1 := ContainerDevices{}
//	s := EncodeContainerDevices(cd1)
//	fmt.Println(s)
//	cd2, _ := DecodeContainerDevices(s)
//	assert.DeepEqual(t, cd1, cd2)
//}

//func TestEmptyPodDeviceCoding(t *testing.T) {
//	pd1 := PodDevices{}
//	s := EncodePodDevices(inRequestDevices, pd1)
//	fmt.Println(s)
//	pd2, _ := DecodePodDevices(inRequestDevices, s)
//	assert.DeepEqual(t, pd1, pd2)
//}

//func TestPodDevicesCoding(t *testing.T) {
//	tests := []struct {
//		name string
//		args PodDevices
//	}{
//		{
//			name: "one pod one container use zero device",
//			args: PodDevices{
//				"NVIDIA": PodSingleDevice{},
//			},
//		},
//		{
//			name: "one pod one container use one device",
//			args: PodDevices{
//				"NVIDIA": PodSingleDevice{
//					ContainerDevices{
//						ContainerDevice{0, "UUID1", "Type1", 1000, 30},
//					},
//				},
//			},
//		},
//		{
//			name: "one pod two container, every container use one device",
//			args: PodDevices{
//				"NVIDIA": PodSingleDevice{
//					ContainerDevices{
//						ContainerDevice{0, "UUID1", "Type1", 1000, 30},
//					},
//					ContainerDevices{
//						ContainerDevice{0, "UUID1", "Type1", 1000, 30},
//					},
//				},
//			},
//		},
//		{
//			name: "one pod one container use two devices",
//			args: PodDevices{
//				"NVIDIA": PodSingleDevice{
//					ContainerDevices{
//						ContainerDevice{0, "UUID1", "Type1", 1000, 30},
//						ContainerDevice{0, "UUID2", "Type1", 1000, 30},
//					},
//				},
//			},
//		},
//	}
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//			//s := EncodePodDevices(inRequestDevices, test.args)
//			//fmt.Println(s)
//			//got, _ := DecodePodDevices(inRequestDevices, s)
//			//assert.DeepEqual(t, test.args, got)
//		})
//	}
//}

func Test_test(t *testing.T) {
	var deviceInfos PodDevices
	log.Info("deviceInfos is ", deviceInfos == nil)

}

func Test_DecodePodDevices(t *testing.T) {
	//DecodePodDevices(checklist map[string]string, annos map[string]string) (PodDevices, error)
	//InRequestDevices["NVIDIA"] = "hami.io/vgpu-devices-to-allocate"
	//SupportDevices["DCU"] = "hami.io/vgpu-devices-allocated"
	SupportDevices["NVIDIA"] = "hami.io/vgpu-devices-allocated"
	tests := []struct {
		name string
		args struct {
			checklist map[string]string
			annos     map[string]string
		}
		want    PodDevices
		wantErr error
	}{
		{
			name: "annos len is 0",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: map[string]string{},
				annos:     make(map[string]string),
			},
			want:    PodDevices{},
			wantErr: nil,
		},
		{
			name: "annos having two device",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: InRequestDevices,
				annos: map[string]string{
					InRequestDevices["NVIDIA"]: ";",
					SupportDevices["NVIDIA"]:   "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,1000,10:;GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,1000,10:;",
				},
			},
			want:    PodDevices{},
			wantErr: nil,
		},
		{
			name: "annos having two device",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: SupportDevices,
				annos: map[string]string{
					InRequestDevices["NVIDIA"]: ";",
					SupportDevices["NVIDIA"]:   "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,1000,10:;GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,2000,20:;",
				},
			},
			want: PodDevices{
				"NVIDIA": {
					{
						{
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d",
							Type:      "NVIDIA",
							Usedmem:   1000,
							Usedcores: 10,
						},
					},
					{
						{
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d",
							Type:      "NVIDIA",
							Usedmem:   2000,
							Usedcores: 20,
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "annos having three device",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: SupportDevices,
				annos: map[string]string{
					InRequestDevices["NVIDIA"]: ";,,0,0:;",
					SupportDevices["NVIDIA"]:   "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,1000,10:;GPU-962d9630-a4ef-dc16-a50d-b2effb90239d,NVIDIA,2000,20:;,,0,0:;",
				},
			},
			want: PodDevices{
				"NVIDIA": {
					{
						{
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d",
							Type:      "NVIDIA",
							Usedmem:   1000,
							Usedcores: 10,
						},
					},
					{
						{
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-b2effb90239d",
							Type:      "NVIDIA",
							Usedmem:   2000,
							Usedcores: 20,
						},
					},
					{{}},
				},
			},
			wantErr: nil,
		},
		{
			name: "annos having 5 dcu device ",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: SupportDevices,
				annos: map[string]string{
					InRequestDevices["DCU"]: ";,,0,0:;",
					SupportDevices["DCU"]:   "GPU-962d9630-a4ef-dc16-a50d-111111111111,DCU,1000,10:GPU-962d9630-a4ef-dc16-a50d-222222222222,DCU,2000,20:;GPU-962d9630-a4ef-dc16-a50d-222222222222,DCU,3000,30:GPU-962d9630-a4ef-dc16-a50d-222222222222,DCU,3000,30:GPU-962d9630-a4ef-dc16-a50d-333333333333,DCU,3000,30:;,,0,0:;",
				},
			},
			want: PodDevices{
				"DCU": {
					{
						{
							Idx:       0,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-111111111111",
							Type:      "DCU",
							Usedmem:   1000,
							Usedcores: 10,
						},
						{
							Idx:       1,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "DCU",
							Usedmem:   2000,
							Usedcores: 20,
						},
					},
					{
						{
							Idx:       0,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "DCU",
							Usedmem:   3000,
							Usedcores: 30,
						},
						{
							Idx:       1,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "DCU",
							Usedmem:   3000,
							Usedcores: 30,
						},
						{
							Idx:       2,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-333333333333",
							Type:      "DCU",
							Usedmem:   3000,
							Usedcores: 30,
						},
					},
					{{}},
				},
			},
			wantErr: nil,
		},
		{
			name: "annos having 5 mixed device ",
			args: struct {
				checklist map[string]string
				annos     map[string]string
			}{
				checklist: SupportDevices,
				annos: map[string]string{
					InRequestDevices["DCU"]: ";,,0,0:;",
					// 这两行最终只有一个生效, 因为key的名字一样
					SupportDevices["DCU"]:    "GPU-962d9630-a4ef-dc16-a50d-111111111111,DCU,1000,10:GPU-962d9630-a4ef-dc16-a50d-222222222222,DCU,2000,20:;GPU-962d9630-a4ef-dc16-a50d-222222222222,NVIDIA,3000,30:GPU-962d9630-a4ef-dc16-a50d-222222222222,NVIDIA,3000,30:GPU-962d9630-a4ef-dc16-a50d-333333333333,NVIDIA,3000,30:;,,0,0:;",
					SupportDevices["NVIDIA"]: "GPU-962d9630-a4ef-dc16-a50d-111111111111,DCU,1000,10:GPU-962d9630-a4ef-dc16-a50d-222222222222,DCU,2000,20:;GPU-962d9630-a4ef-dc16-a50d-222222222222,NVIDIA,3000,30:GPU-962d9630-a4ef-dc16-a50d-222222222222,NVIDIA,3000,30:GPU-962d9630-a4ef-dc16-a50d-333333333333,NVIDIA,3000,30:;,,0,0:;",
				},
			},
			want: PodDevices{
				"DCU": {
					{
						{
							Idx:       0,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-111111111111",
							Type:      "DCU",
							Usedmem:   1000,
							Usedcores: 10,
						},
						{
							Idx:       1,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "DCU",
							Usedmem:   2000,
							Usedcores: 20,
						},
					},
				},
				"NVIDIA": {
					{
						{
							Idx:       0,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "NVIDIA",
							Usedmem:   3000,
							Usedcores: 30,
						},
						{
							Idx:       1,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-222222222222",
							Type:      "NVIDIA",
							Usedmem:   3000,
							Usedcores: 30,
						},
						{
							Idx:       2,
							UUID:      "GPU-962d9630-a4ef-dc16-a50d-333333333333",
							Type:      "NVIDIA",
							Usedmem:   3000,
							Usedcores: 30,
						},
					},
					{{}},
				},
			},
			wantErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			//got, gotErr := DecodePodDevices(test.args.checklist, test.args.annos, )
			//assert.DeepEqual(t, test.wantErr, gotErr)
			//assert.DeepEqual(t, test.want, got)
		})
	}
}

func TestDecodePodDevicesWithInitContainers(t *testing.T) {
	SupportDevices["NVIDIA"] = "hami.io/vgpu-devices-allocated"
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

	got, err := DecodePodDevices(pod, logger)
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
