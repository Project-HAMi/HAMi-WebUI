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
package ascend

import (
	"testing"

	"vgpu/internal/provider/util"
)

func TestAscendVariantRegistration(t *testing.T) {
	wantAnnos := []string{
		"hami.io/node-register-Ascend910B",
		"hami.io/node-register-Ascend310P",
		"hami.io/node-register-Ascend910A",
		"hami.io/node-register-Ascend910B2",
		"hami.io/node-register-Ascend910B3",
		"hami.io/node-register-Ascend910B4",
		"hami.io/node-register-Ascend910B4-1",
		"hami.io/node-register-Ascend910C",
	}
	got := map[string]bool{}
	for _, a := range AscendNodeRegisterAnnos {
		got[a] = true
	}
	for _, want := range wantAnnos {
		if !got[want] {
			t.Errorf("AscendNodeRegisterAnnos missing %s", want)
		}
	}

	for _, cw := range ascendCommonWords {
		if util.SupportDevices[cw] != "hami.io/"+cw+"-devices-allocated" {
			t.Errorf("SupportDevices[%s] = %q", cw, util.SupportDevices[cw])
		}
		if util.InRequestDevices[cw] != "hami.io/"+cw+"-devices-to-allocate" {
			t.Errorf("InRequestDevices[%s] = %q", cw, util.InRequestDevices[cw])
		}
	}
}
