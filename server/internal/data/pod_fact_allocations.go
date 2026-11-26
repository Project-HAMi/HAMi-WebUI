package data

import (
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/podfacts"
	"vgpu/internal/provider"
	"vgpu/internal/resourceview"
)

func (r *podRepo) hamiFactEvidence(pod *corev1.Pod, node *corev1.Node, catalog *devicecatalog.Snapshot) (podfacts.Evidence, podfacts.Options) {
	result := podfacts.Evidence{}
	options := podfacts.Options{}
	for name := range r.schedulingResourceNames {
		options.ResourceNames = append(options.ResourceNames, name)
	}
	if catalog.Loaded() {
		for _, names := range catalog.ResourceNames {
			for _, name := range []string{names.Count, names.Memory, names.Compute} {
				if name != "" {
					options.ResourceNames = append(options.ResourceNames, corev1.ResourceName(name))
				}
			}
		}
	}
	containerNames := make([]string, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, c := range pod.Spec.InitContainers {
		containerNames = append(containerNames, c.Name)
	}
	for _, c := range pod.Spec.Containers {
		containerNames = append(containerNames, c.Name)
	}

	dialects := r.factAllocationDialects()
	ctx := provider.PodAllocationContext{
		Pod:            pod,
		Node:           node,
		Catalog:        catalog,
		ContainerNames: containerNames,
		Logger:         r.log,
	}
	recognizedKeys := map[string]bool{}
	for _, dialect := range dialects {
		options.ResourceNames = append(options.ResourceNames, dialect.CatalogResourceNames(catalog)...)
		out := dialect.ExtractPodAllocations(ctx)
		result.Allocations = append(result.Allocations, out.Allocations...)
		result.UnreadableAllocations = append(result.UnreadableAllocations, out.UnreadableAllocations...)
		options.ResourceNames = append(options.ResourceNames, out.ResourceNames...)
		for _, key := range out.RecognizedAnnotationKeys {
			recognizedKeys[key] = true
		}
	}

	// A nonempty allocator annotation outside the supported decoder set is
	// evidence of an unrecognized allocation, never proof of a CPU-only Pod.
	for key, raw := range pod.Annotations {
		if recognizedKeys[key] || !strings.HasPrefix(key, "hami.io/") || !strings.HasSuffix(key, "-devices-allocated") || strings.Trim(raw, ";:") == "" {
			continue
		}
		kind := strings.TrimSuffix(strings.TrimPrefix(key, "hami.io/"), "-devices-allocated")
		source := provider.NewFactAllocationSnapshot(pod, kind)
		source.Source.State, source.Source.Reason = resourceview.Unknown, "version_unsupported"
		source.HasAllocationEvidence = true
		result.Allocations = append(result.Allocations, source)
	}
	sort.Slice(result.Allocations, func(i, j int) bool {
		return result.Allocations[i].Source.Source < result.Allocations[j].Source.Source
	})
	return result, options
}
