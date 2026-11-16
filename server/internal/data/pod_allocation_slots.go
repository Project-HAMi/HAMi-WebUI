package data

import (
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"vgpu/internal/devicecatalog"
	"vgpu/internal/provider/util"
)

// requestedSlots uses the same container order and count resource as HAMi Filter.
func requestedSlots(pod *corev1.Pod, countResource string) []int {
	if countResource == "" {
		return nil
	}
	key := corev1.ResourceName(countResource)
	var slots []int
	add := func(i int, c *corev1.Container) {
		q, ok := c.Resources.Limits[key]
		if !ok {
			q = c.Resources.Requests[key]
		}
		if q.Sign() > 0 {
			slots = append(slots, i)
		}
	}
	for i := range pod.Spec.InitContainers {
		add(i, &pod.Spec.InitContainers[i])
	}
	for i := range pod.Spec.Containers {
		add(i+len(pod.Spec.InitContainers), &pod.Spec.Containers[i])
	}
	return slots
}

// HAMi v2.10 can omit every CPU-only slot for a model-specific NVIDIA
// registration. Restore those positions only when the configured GPU requests
// account for every encoded row. The informer object stays untouched.
func alignNvidiaAllocation(pod *corev1.Pod, catalog *devicecatalog.Snapshot) *corev1.Pod {
	key := util.SupportDevices[util.NvidiaGPUDevice]
	raw := pod.Annotations[key]
	n := len(pod.Spec.InitContainers) + len(pod.Spec.Containers)
	if !catalog.Loaded() || n < 2 || raw == "" {
		return pod
	}
	raw, terminated := strings.CutSuffix(raw, ";")
	if !terminated || strings.Count(raw, ";")+1 >= n {
		return pod
	}
	rows := strings.Split(raw, ";")
	slots := requestedSlots(pod, catalog.ResourceNames[util.NvidiaGPUDevice].Count)
	if len(rows) != len(slots) || slices.Contains(rows, "") {
		return pod
	}
	aligned := make([]string, n)
	for i, slot := range slots {
		aligned[slot] = rows[i]
	}
	copy := *pod
	copy.Annotations = maps.Clone(pod.Annotations)
	copy.Annotations[key] = strings.Join(aligned, ";") + ";"
	return &copy
}
