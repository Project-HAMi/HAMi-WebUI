package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

func init() {
	// 卡维度指标
	prometheus.MustRegister(HamiVCoreScaling)            // 算力超分倍数
	prometheus.MustRegister(HamiVMemoryScaling)          // 显存超分倍数
	prometheus.MustRegister(HamiVgpuCount)               // 虚拟vgpu设备数
	prometheus.MustRegister(HamiVmemorySize)             // 虚拟显存大小(MB)
	prometheus.MustRegister(HamiVcoreSize)               // 虚拟算力大小
	prometheus.MustRegister(HamiMemorySize)              // 真实显存总量(MB)
	prometheus.MustRegister(HamiMemoryUsed)              // 真实显存已使用(MB)
	prometheus.MustRegister(HamiMemoryUtil)              // 真实显存利用率
	prometheus.MustRegister(HamiCoreSize)                // 真实算力总量
	prometheus.MustRegister(HamiCoreUsed)                // 真实算力已使用
	prometheus.MustRegister(HamiCoreUtil)                // 真实算力利用率
	prometheus.MustRegister(HamiCoreUsedAvg)             // 真实算力已使用周期平均
	prometheus.MustRegister(HamiCoreUtilAvg)             // 真实算力利用率周期平均
	prometheus.MustRegister(HamiDeviceTemperature)       // 显卡温度
	prometheus.MustRegister(HamiDeviceMemoryTemperature) // 显存温度
	prometheus.MustRegister(HamiDevicePower)             // 显卡功耗
	prometheus.MustRegister(HamiDeviceFanSpeedP)         // 风扇转速（百分比）
	prometheus.MustRegister(HamiDeviceFanSpeedR)         // 风扇转速（每分钟转速）
	prometheus.MustRegister(HamiDeviceHardwareHealth)    // 显卡健康状态

	// 任务维度指标
	prometheus.MustRegister(HamiContainerVgpuAllocated)    // 任务申请的vgpu设备数
	prometheus.MustRegister(HamiContainerVmemoryAllocated) // 任务申请的vmemory
	prometheus.MustRegister(HamiContainerVcoreAllocated)   // 任务申请的vcore
	prometheus.MustRegister(HamiContainerMemoryUsed)       // 任务实际使用的显存大小（MB）
	prometheus.MustRegister(HamiContainerMemoryUtil)       // 任务实际使用的显存占任务申请的比例
	prometheus.MustRegister(HamiContainerCoreUsed)         // 任务实际使用的算力占卡的百分比
	prometheus.MustRegister(HamiContainerCoreUtil)         // 任务实际使用的算力占任务申请的比例

	// 资源池维度指标
	prometheus.MustRegister(HamiPoolVcoreSize)   // 资源池总算力大小
	prometheus.MustRegister(HamiPoolVgpuCount)   // 资源池总vgpu设备数
	prometheus.MustRegister(HamiPoolVmemorySize) // 资源池总显存大小

	prometheus.MustRegister(HamiSystemComponentHealth) // 系统组件健康状态
}

func reset() {
	HamiVCoreScaling.Reset()
	HamiVMemoryScaling.Reset()
	HamiVgpuCount.Reset()
	HamiVmemorySize.Reset()
	HamiVcoreSize.Reset()
	HamiMemoryUsed.Reset()
	HamiMemorySize.Reset()
	HamiMemoryUtil.Reset()
	HamiCoreSize.Reset()
	HamiCoreUsed.Reset()
	HamiCoreUtil.Reset()
	HamiCoreUsedAvg.Reset()
	HamiCoreUtilAvg.Reset()
	HamiDeviceTemperature.Reset()
	HamiDeviceMemoryTemperature.Reset()
	HamiDevicePower.Reset()
	HamiDeviceFanSpeedP.Reset()
	HamiDeviceFanSpeedR.Reset()

	HamiContainerVgpuAllocated.Reset()
	HamiContainerVmemoryAllocated.Reset()
	HamiContainerVcoreAllocated.Reset()
	HamiContainerMemoryUsed.Reset()
	HamiContainerMemoryUtil.Reset()
	HamiContainerCoreUsed.Reset()
	HamiContainerCoreUtil.Reset()

	HamiPoolVgpuCount.Reset()
	HamiPoolVmemorySize.Reset()
	HamiPoolVcoreSize.Reset()
}

var (
	HamiVCoreScaling = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vcore_scaling",
		Help: "GPU virtual core Scaling",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiVMemoryScaling = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vmemory_scaling",
		Help: "GPU virtual memory Scaling",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiVgpuCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vgpu_count",
		Help: "Total vGPU count",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiVmemorySize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vmemory_size",
		Help: "Total vMemory size",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiVcoreSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_vcore_size",
		Help: "Total vCore size",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiMemoryUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_used",
		Help: "Actual memory usage, unit is 'MB' ",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiMemorySize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_size",
		Help: "Actual memory size, unit is 'MB' ",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiMemoryUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_memory_util",
		Help: "Actual Memory Util percent 0-100",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiCoreSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_size",
		Help: "Actual core size",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiCoreUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_used",
		Help: "Actual Core Used",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiCoreUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_util",
		Help: "Actual Core Util percent 0-100",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiCoreUsedAvg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_used_avg",
		Help: "Actual Core Used period avg",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiCoreUtilAvg = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_core_util_avg",
		Help: "Actual Core Util percent 0-100 period avg",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDeviceTemperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_temperature",
		Help: "gpu temperature",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDeviceMemoryTemperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_memory_temperature",
		Help: "gpu memory temperature",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDevicePower = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_power",
		Help: "gpu power",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDeviceHardwareHealth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_hardware_health",
		Help: "gpu hardware health",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDeviceFanSpeedP = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_fan_speed_p",
		Help: "gpu fan speed percent 0-100",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiDeviceFanSpeedR = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_device_fan_speed_r",
		Help: "gpu fan speed rpm",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "driver_version", "device_no"})

	HamiContainerVgpuAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vgpu_allocated",
		Help: "task allocated vGPU count",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerVmemoryAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vmemory_allocated",
		Help: "task allocated vMemory size",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerVcoreAllocated = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_vcore_allocated",
		Help: "task allocated vCore size",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name", "container_pod_uuid"})

	HamiContainerMemoryUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_memory_used",
		Help: "task used memory unit MB",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerMemoryUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_memory_util",
		Help: "task memory util percent 0-100",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerCoreUsed = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_core_used",
		Help: "task used core ",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name"})

	HamiContainerCoreUtil = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hami_container_core_util",
		Help: "task core util percent 0-100",
	}, []string{"node", "provider", "devicetype", "deviceuuid", "pod_name", "container_name", "namespace_name"})

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
