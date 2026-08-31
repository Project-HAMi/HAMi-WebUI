package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	// Device metrics.
	prometheus.MustRegister(HamiVCoreScaling)
	prometheus.MustRegister(HamiVMemoryScaling)
	prometheus.MustRegister(HamiVgpuCount)
	prometheus.MustRegister(HamiVmemorySize)
	prometheus.MustRegister(HamiVcoreSize)
	prometheus.MustRegister(HamiMemorySize)
	prometheus.MustRegister(HamiMemoryUsed)
	prometheus.MustRegister(HamiMemoryUtil)
	prometheus.MustRegister(HamiCoreSize)
	prometheus.MustRegister(HamiCoreUsed)
	prometheus.MustRegister(HamiCoreUtil)
	prometheus.MustRegister(HamiCoreUsedAvg)
	prometheus.MustRegister(HamiCoreUtilAvg)
	prometheus.MustRegister(HamiDeviceTemperature)
	prometheus.MustRegister(HamiDeviceMemoryTemperature)
	prometheus.MustRegister(HamiDevicePower)
	prometheus.MustRegister(HamiDeviceFanSpeedP)
	prometheus.MustRegister(HamiDeviceFanSpeedR)
	prometheus.MustRegister(HamiDeviceLastXIDErrorCode)

	// Container metrics.
	prometheus.MustRegister(HamiContainerVgpuAllocated)
	prometheus.MustRegister(HamiContainerVmemoryAllocated)
	prometheus.MustRegister(HamiContainerVcoreAllocated)
	prometheus.MustRegister(HamiContainerVcoreAllocationKnown)
	prometheus.MustRegister(HamiContainerMemoryUsed)
	prometheus.MustRegister(HamiContainerMemoryUtil)
	prometheus.MustRegister(HamiContainerCoreUsed)
	prometheus.MustRegister(HamiContainerCoreUtil)

	// Resource pool metrics.
	prometheus.MustRegister(HamiPoolVcoreSize)
	prometheus.MustRegister(HamiPoolVgpuCount)
	prometheus.MustRegister(HamiPoolVmemorySize)

	prometheus.MustRegister(HamiSystemComponentHealth)
}

var (
	HamiVCoreScaling = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vcore_scaling",
		Help: "GPU virtual core Scaling",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiVMemoryScaling = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vmemory_scaling",
		Help: "HAMi-registered schedulable memory divided by vendor-reported physical memory",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiVgpuCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vgpu_count",
		Help: "Total vGPU count",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiVmemorySize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vmemory_size",
		Help: "HAMi-registered schedulable device memory, unit is 'MiB'",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiVcoreSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vcore_size",
		Help: "Total vCore size",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiMemoryUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_used",
		Help: "Vendor-reported used physical device memory, unit is 'MiB'",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiMemorySize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_size",
		Help: "Vendor-reported total physical device memory, unit is 'MiB'",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiMemoryUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_util",
		Help: "Vendor-reported used physical memory divided by total physical memory, percent 0-100",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiCoreSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_size",
		Help: "Actual core size",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiCoreUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_used",
		Help: "Actual Core Used",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiCoreUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_util",
		Help: "Actual Core Util percent 0-100",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiCoreUsedAvg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_used_avg",
		Help: "Actual Core Used period avg",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiCoreUtilAvg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_util_avg",
		Help: "Actual Core Util percent 0-100 period avg",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDeviceTemperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_temperature",
		Help: "gpu temperature",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDeviceMemoryTemperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_memory_temperature",
		Help: "gpu memory temperature",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDevicePower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_power",
		Help: "gpu power",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDeviceLastXIDErrorCode = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_last_xid_error_code",
		Help: "Last NVIDIA XID error code reported by DCGM; 0 means no error and non-zero values are diagnostic codes, not a health status",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDeviceFanSpeedP = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_fan_speed_p",
		Help: "gpu fan speed percent 0-100",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiDeviceFanSpeedR = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_fan_speed_r",
		Help: "gpu fan speed rpm",
	}, []string{"node", "provider", "device_type", "device_uuid", "driver_version", "device_no"})

	HamiContainerVgpuAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vgpu_allocated",
		Help: "task allocated vGPU count",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerVmemoryAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vmemory_allocated",
		Help: "task allocated vMemory size",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerVcoreAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vcore_allocated",
		Help: "task allocated vCore size",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerVcoreAllocationKnown = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vcore_allocation_known",
		Help: "whether the task's allocated vCore size is known (1) or unavailable (0)",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerMemoryUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_memory_used",
		Help: "task used memory unit MB",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerMemoryUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_memory_util",
		Help: "task memory util percent 0-100",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerCoreUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_core_used",
		Help: "task compute usage in provider-specific core units; NVIDIA estimates active allocated vCore percentage points and excludes elastic borrowing",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerCoreUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_core_util",
		Help: "task compute utilization percent; NVIDIA reports allocated-compute activity 0-100, not physical-card utilization",
	}, []string{"node", "provider", "device_type", "device_uuid", "pod_name", "container_name", "namespace_name"})

	HamiPoolVgpuCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_pool_vgpu_count",
		Help: "Pool total vGPU count",
	}, []string{"pool"})

	HamiPoolVmemorySize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_pool_vmemory_size",
		Help: "Pool total vMemory size",
	}, []string{"pool"})

	HamiPoolVcoreSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_pool_vcore_size",
		Help: "Pool total vCore size",
	}, []string{"pool"})

	HamiSystemComponentHealth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_system_component_health",
		Help: "system component health",
	}, []string{"component"})

	Reset = true
)
