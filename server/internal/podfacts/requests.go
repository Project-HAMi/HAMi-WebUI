package podfacts

import (
	"math"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"vgpu/internal/resourceview"
)

func resourceRequests(resources corev1.ResourceRequirements, options Options) []Request {
	result := []Request{}
	for _, item := range []struct {
		field  string
		values corev1.ResourceList
	}{{"requests", resources.Requests}, {"limits", resources.Limits}} {
		names := make([]string, 0, len(item.values))
		for name := range item.values {
			names = append(names, string(name))
		}
		slices.Sort(names)
		for _, name := range names {
			if !recognizes(corev1.ResourceName(name), options) {
				continue
			}
			value := item.values[corev1.ResourceName(name)]
			state := known("pod_spec", "container")
			if value.Sign() < 0 {
				state = unknown("pod_spec", "container")
			}
			result = append(result, Request{Resource: name, Field: item.field, Raw: value.String(), Source: "pod_spec", State: state, Positive: value.Sign() > 0})
		}
	}
	return result
}
func claimReferences(pod *corev1.Pod, container corev1.Container) []ClaimReference {
	result := []ClaimReference{}
	for _, ref := range container.Resources.Claims {
		value := ClaimReference{LocalName: ref.Name, Source: "pod_spec", State: unknown("dra_claim", "container")}
		for _, declaration := range pod.Spec.ResourceClaims {
			if declaration.Name == ref.Name {
				if declaration.Source.ResourceClaimName != nil {
					value.ClaimName = *declaration.Source.ResourceClaimName
				}
				if declaration.Source.ResourceClaimTemplateName != nil {
					value.TemplateName = *declaration.Source.ResourceClaimTemplateName
				}
			}
		}
		// status resolves a template-generated name; no API call or speculative name.
		if value.TemplateName != "" && value.ClaimName == "" {
			for _, status := range pod.Status.ResourceClaimStatuses {
				if status.Name == ref.Name && status.ResourceClaimName != nil {
					value.ClaimName = *status.ResourceClaimName
				}
			}
		}
		result = append(result, value)
	}
	return result
}
func (f *Facts) readAnnotations(pod *corev1.Pod) {
	for _, key := range []string{"gpu-fraction", "gpu-memory"} {
		raw, exists := pod.Annotations[key]
		if !exists {
			continue
		}
		valid, positive := false, false
		if key == "gpu-fraction" {
			v, err := strconv.ParseFloat(raw, 64)
			valid = err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
			positive = valid && v > 0
		} else {
			v, err := strconv.ParseInt(raw, 10, 64)
			valid = err == nil && v >= 0
			positive = valid && v > 0
		}
		state := known("kai_annotation", "pod")
		if !valid {
			state = unknown("kai_annotation", "pod")
		}
		request := Request{Field: key, Raw: raw, Source: "kai_annotation", State: state, Positive: positive}
		// Preserve the Pod scope. Different admission/isolator versions have different
		// target defaults; do not silently select the first container here.
		f.Requests = append(f.Requests, request)
		f.consider(request)
	}
	keys := make([]string, 0, len(pod.Annotations))
	for key := range pod.Annotations {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		const prefix = "nvidia.com/container."
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		target := ""
		for _, suffix := range []string{".gpu-memory.request", ".gpu-memory.limit"} {
			if strings.HasSuffix(key, suffix) {
				target = strings.TrimSuffix(strings.TrimPrefix(key, prefix), suffix)
				break
			}
		}
		if target == "" {
			continue
		}
		var container *Container
		for i := range f.Containers {
			if f.Containers[i].Key.Name == target && f.Containers[i].Kind != Ephemeral {
				container = &f.Containers[i]
				break
			}
		}
		raw := pod.Annotations[key]
		state := known("kai_annotation", "container")
		value, err := resource.ParseQuantity(raw)
		if err != nil || value.Sign() < 0 {
			state = unknown("kai_annotation", "container")
		}
		request := Request{Field: key, Raw: raw, Source: "kai_annotation", State: state, Positive: state.State == resourceview.Known && value.Sign() > 0}
		if container == nil {
			request.State = unknown("kai_annotation", "pod")
			request.State.Params = map[string]string{"container": target}
			request.Positive = false
			f.Requests = append(f.Requests, request)
			f.Issues = append(f.Issues, Issue{"annotations." + key, "unknown_or_ephemeral_annotation_target"})
		} else {
			container.Requests = append(container.Requests, request)
		}
		f.consider(request)
	}
}
