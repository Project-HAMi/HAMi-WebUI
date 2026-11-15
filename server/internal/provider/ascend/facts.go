package ascend

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/util"
)

// RegistrationFacts preserves per-field evidence from Kubernetes. Unlike the
// old telemetry-enriched provider it does not invent UUIDs or observation health.
func RegistrationFacts(node *corev1.Node, catalog *devicecatalog.Snapshot) ([]util.RegistrationDevice, error) {
	keys := map[string]bool{}
	for _, word := range catalog.AscendCommonWords() {
		if _, exists := node.Annotations[registerAnnotationPrefix+word]; exists {
			keys[registerAnnotationPrefix+word] = true
		}
	}
	for key := range node.Annotations {
		if strings.HasPrefix(key, registerAnnotationPrefix+legacyCommonWordPrefix) {
			keys[key] = true
		}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	result := []util.RegistrationDevice{}
	var errs []error
	counts := map[string]int{}
	mode := NodeSplitMode(node)
	for _, key := range ordered {
		word := strings.TrimPrefix(key, registerAnnotationPrefix)
		facts, err := util.DecodeRegistrationFacts(node.Annotations[key])
		if err != nil {
			errs = append(errs, err)
		}
		for _, fact := range facts {
			if fact.Device.Type != word {
				errs = append(errs, fmt.Errorf("Ascend registration type differs from source"))
				continue
			}
			fact.Device.Mode = mode
			fact.Known["mode"] = mode != ""
			counts[fact.Device.ID]++
			result = append(result, fact)
		}
	}
	filtered := result[:0]
	for _, fact := range result {
		if counts[fact.Device.ID] != 1 {
			errs = append(errs, fmt.Errorf("conflicting Ascend registration identity"))
			continue
		}
		filtered = append(filtered, fact)
	}
	return filtered, errors.Join(errs...)
}
