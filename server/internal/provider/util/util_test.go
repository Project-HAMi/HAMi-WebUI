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
	"os"
	"reflect"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
	corev1 "k8s.io/api/core/v1"
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

func Test_DecodePodDevices_Ascend(t *testing.T) {
	// Ensure Ascend device keys are registered in SupportDevices.
	SupportDevices["Ascend"] = "hami.io/Ascend910B-devices-allocated"
	SupportDevices["Ascend310P"] = "hami.io/Ascend310P-devices-allocated"
	t.Cleanup(func() {
		delete(SupportDevices, "Ascend")
		delete(SupportDevices, "Ascend310P")
	})

	logger := log.NewHelper(log.NewStdLogger(os.Stdout))

	tests := []struct {
		name        string
		pod         *corev1.Pod
		want        PodDevices
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
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
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
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
					},
					{
						{Idx: 0, UUID: "D7E96E64-214123F1-E8E618E4-AED8030A-E3003039", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
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
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
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
						{Idx: 0, UUID: "E0766E64-20C0AB59-CC9AB1A4-3778030A-83003019", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
					},
					{}, // empty segment placeholder
					{
						{Idx: 0, UUID: "D7E96E64-214123F1-E8E618E4-AED8030A-E3003039", Type: "Ascend310P", Usedmem: 6144, Usedcores: 25},
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
			got, err := DecodePodDevices(tt.pod, logger)
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
