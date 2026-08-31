package ascend

import (
	"testing"

	"vgpu/internal/provider/util"
)

func TestLegacyAscendAnnotationsRemainRegistered(t *testing.T) {
	if got := util.SupportDevices[AscendDevice]; got != "hami.io/Ascend910B-devices-allocated" {
		t.Fatalf("legacy Ascend910B allocation annotation = %q", got)
	}
	if got := util.SupportDevices[Ascend310PDevice]; got != "hami.io/Ascend310P-devices-allocated" {
		t.Fatalf("legacy Ascend310P allocation annotation = %q", got)
	}
}
