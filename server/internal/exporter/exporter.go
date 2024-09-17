package exporter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/mlu"
	"vgpu/internal/service"

	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewMetricsGenerator,
)

type MetricsGenerator struct {
	promClient     *prom.Client
	nodeUsecase    *biz.NodeUsecase
	podUsecase     *biz.PodUseCase
	monitorService *service.MonitorService
	cacheTime      time.Time
}

// roundToTwoDecimal 将浮点数保留两位小数
func roundToTwoDecimal(value float64) float64 {
	return float64(math.Round(value*100) / 100)
}

// roundToOneDecimal 将浮点数保留一位小数并进行四舍五入
func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func NewMetricsGenerator(
	promClient *prom.Client,
	nodeUsecase *biz.NodeUsecase,
	podUsecase *biz.PodUseCase,
	monitorService *service.MonitorService,
) *MetricsGenerator {
	return &MetricsGenerator{
		promClient:     promClient,
		nodeUsecase:    nodeUsecase,
		podUsecase:     podUsecase,
		monitorService: monitorService,
	}
}
func (s *MetricsGenerator) generatorCache() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, now.Location())
}

func (s *MetricsGenerator) cacheIsValidate() bool {
	if s.cacheTime == s.generatorCache() {
		return true
	}
	return false
}

func (s *MetricsGenerator) GenerateMetrics(ctx context.Context) error {
	//if s.cacheIsValidate() {
	//	return nil
	//}
	reset()                         // 重置所有指标缓存值
	s.GenerateDeviceMetrics(ctx)    // 卡维度指标
	s.GenerateContainerMetrics(ctx) // 任务维度指标
	s.cacheTime = s.generatorCache()
	return nil
}

// 卡维度指标
func (s *MetricsGenerator) GenerateDeviceMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	for _, device := range deviceInfos {
		provider := device.Provider
		// 查询device的驱动版本以及设备号
		deviceAdditional, err := s.queryDeviceAdditional(ctx, provider, device.Id)
		var driver, deviceNo = "", ""
		if err == nil && deviceAdditional != nil {
			driver = deviceAdditional.DriverVersion
			deviceNo = deviceAdditional.DeviceNo
		}

		// 分配率指标
		HamiVgpuCount.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Count))
		HamiVmemorySize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devmem))
		HamiVcoreSize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devcore))
		// 超配比指标
		HamiVCoreScaling.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(device.Devcore) / 100)
		HamiCoreSize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(100)
		deviceMemUsed, err := s.deviceMemUsed(ctx, provider, device.Id)
		if err == nil {
			HamiMemoryUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(deviceMemUsed))
		}
		deviceMemSize, err := s.deviceMemTotal(ctx, provider, device.Id)
		if err == nil && deviceMemSize > 0 {
			HamiMemorySize.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(deviceMemSize))
			HamiMemoryUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(roundToOneDecimal(100 * float64(deviceMemUsed/deviceMemSize)))
			HamiVMemoryScaling.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(roundToOneDecimal(float64(float32(device.Devmem) / deviceMemSize)))
		}
		actualCoreUtil, err := s.deviceCoreUtil(ctx, provider, device.Id)
		if err == nil {
			HamiCoreUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(actualCoreUtil))
			HamiCoreUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(actualCoreUtil))
		}
		actualCoreUtilAvg, err := s.deviceCoreUtil(ctx, provider, device.Id)
		if err == nil {
			HamiCoreUsedAvg.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(actualCoreUtilAvg))
			HamiCoreUtilAvg.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(actualCoreUtilAvg))
		}
		gpuTemperature, err := s.gpuTemperature(ctx, provider, device.Id)
		if err == nil {
			HamiDeviceTemperature.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(gpuTemperature))
		}
		memoryTemperature, err := s.memoryTemperature(ctx, provider, device.Id)
		if err == nil {
			HamiDeviceMemoryTemperature.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(memoryTemperature))
		}
		gpuPower, err := s.gpuPower(ctx, provider, device.Id)
		if err == nil {
			HamiDevicePower.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(gpuPower))
		}
		fanSpeed, err := s.fanSpeed(ctx, provider, device.Id)
		if err == nil {
			switch provider {
			case biz.NvidiaGPUDevice:
				HamiDeviceFanSpeedP.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(fanSpeed))
			case biz.CambriconGPUDevice:
				HamiDeviceFanSpeedR.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(fanSpeed))
			}
		}
		gpuHardwareHealth, err := s.gpuHardwareHealth(ctx, provider, device.Id)
		if err == nil {
			HamiDeviceHardwareHealth.WithLabelValues(device.NodeName, provider, device.Type, device.Id, driver, deviceNo).Set(float64(gpuHardwareHealth))
		}
	}
	return nil
}

// 任务维度指标
func (s *MetricsGenerator) GenerateContainerMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	containers, err := s.podUsecase.ListAll(ctx)
	if err != nil {
		return err
	}
	for _, device := range deviceInfos {
		for _, c := range containers {
			var vGPU int32 = 0
			var core int32 = 0
			var memory int32 = 0
			var provider string = ""
			for _, cd := range c.ContainerDevices {
				if device.AliasId != "" && device.AliasId != cd.UUID {
					continue
				}
				vGPU = vGPU + 1
				core = core + cd.Usedcores
				memory = memory + cd.Usedmem
				provider = cd.Type
			}
			if provider == "" {
				continue
			}
			HamiContainerVgpuAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(vGPU))
			HamiContainerVmemoryAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(memory))
			HamiContainerVcoreAllocated.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, fmt.Sprintf("%s:%s", c.Name, c.PodUID)).Set(float64(core))
			// 查询任务在当前设备下的算力利用率
			taskCoreUsed, err := s.taskCoreUsed(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id)
			if err == nil {
				used := float64(0)
				util := float64(0)
				switch provider {
				case biz.NvidiaGPUDevice:
					used = float64(taskCoreUsed)
					util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core))
				case biz.CambriconGPUDevice:
					used = float64(taskCoreUsed) / 100 * float64(core)
					util = float64(taskCoreUsed)
				case biz.HygonGPUDevice:
					used = float64(taskCoreUsed)
					util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core))
				default:
				}
				cardCoreUtil, err := s.deviceCoreUtil(ctx, provider, device.Id)
				if err == nil && used != 0 && cardCoreUtil > 95 {
					used = float64(cardCoreUtil) / 100 * float64(core)
					util = float64(cardCoreUtil)
				}
				HamiContainerCoreUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(used)
				HamiContainerCoreUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(util)
			}
			taskMemoryUsed, err := s.taskMemoryUsed(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id)
			if err == nil {
				switch provider {
				case biz.CambriconGPUDevice:
					taskMemoryUsed = float32((taskMemoryUsed/100)*float32(memory)) * 1024 * 1024
				default:
				}
				HamiContainerMemoryUsed.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(float64(taskMemoryUsed / 1024 / 1024))
				HamiContainerMemoryUtil.WithLabelValues(device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace).Set(roundToOneDecimal(100 * float64(taskMemoryUsed/1024/1024) / float64(memory)))
			}
		}
	}
	return nil
}

func (s *MetricsGenerator) queryInstantVal(ctx context.Context, query string) (float32, error) {
	res, err := s.monitorService.QueryInstant(context.TODO(), &pb.QueryInstantRequest{
		Query: query,
	})
	if err != nil {
		return 0, err
	}
	if len(res.Data) > 0 {
		return res.Data[0].Value, nil
	}
	return 0, nil
}

// 卡显存已使用量
func (s *MetricsGenerator) deviceMemUsed(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_used{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_hbm_used_memory{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_usedmemory_bytes{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	if provider == biz.CambriconGPUDevice {
		val = val / mlu.CambriconMemUnit
	} else if provider == biz.HygonGPUDevice {
		val = val / 1024 / 1024
	}
	return val, err
}

// 卡显存总量
func (s *MetricsGenerator) deviceMemTotal(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FB_FREE{UUID=\"%s\"})+avg(DCGM_FI_DEV_FB_USED{UUID=\"%s\"})", deviceUUID, deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_total{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_hbm_total_memory{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_memorycap_bytes{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	if provider == biz.CambriconGPUDevice {
		val = val / 1024
	} else if provider == biz.HygonGPUDevice {
		val = val / 1024 / 1024
	}
	return val, err
}

// 卡算力利用率
func (s *MetricsGenerator) deviceCoreUtil(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		//query = fmt.Sprintf("avg(avg_over_time(DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}[1m]))", deviceUUID)
		query = fmt.Sprintf("DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}", deviceUUID)
		//query = fmt.Sprintf("(%s * (sum_over_time(%s[5m:]) / count_over_time(( %s !=0)[5m:])) / %s) > 0 or %s", queryTemplate, queryTemplate, queryTemplate, queryTemplate, queryTemplate)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_utilizationrate{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 任务算力利用率
func (s *MetricsGenerator) taskCoreUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		//query = fmt.Sprintf("avg(Device_utilization_desc_of_container{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"})", deviceUUID, namespace, pod, container)
		//		queryTemplate := `last_over_time((Device_utilization_desc_of_container{deviceuuid="%s", podnamespace="%s", podname="%s", ctrname="%s"} != 0)[1m:])
		//or
		//last_over_time(Device_utilization_desc_of_container{deviceuuid="%s", podnamespace="%s", podname="%s", ctrname="%s"}[1m:])`
		//		query = fmt.Sprintf(queryTemplate, deviceUUID, namespace, pod, container, deviceUUID, namespace, pod, container)
		queryTemplate := fmt.Sprintf("Device_utilization_desc_of_container{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"}", deviceUUID, namespace, pod, container)
		query = fmt.Sprintf("sum_over_time(%s[1m]) == 0 or (sum_over_time(%s[10m:]) / count_over_time(( %s !=0)[10m:])) ", queryTemplate, queryTemplate, queryTemplate)
		//query = queryTemplate
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vcore\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		return 0, nil
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(vdcu_percent{pod_uuid=\"%s\", container_name=\"%s\"})", podUUID, container)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 任务显存使用量
func (s *MetricsGenerator) taskMemoryUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(vGPU_device_memory_usage_in_bytes{deviceuuid=\"%s\", podnamespace=\"%s\", podname=\"%s\", ctrname=\"%s\"})", deviceUUID, namespace, pod, container)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vmemory\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		return 0, nil
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(vdcu_usage_memory_size{pod_uuid=\"%s\", container_name=\"%s\"})", podUUID, container)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// GPU温度
func (s *MetricsGenerator) gpuTemperature(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_GPU_TEMP{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_temperature{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_temp{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 显存温度
func (s *MetricsGenerator) memoryTemperature(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_MEMORY_TEMP{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_temperature{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_temperature{vdie_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 功耗
func (s *MetricsGenerator) gpuPower(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_power_usage{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_power{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_power_usage{device_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

// 硬件健康
func (s *MetricsGenerator) gpuHardwareHealth(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_XID_ERRORS{UUID=\"%s\"})", deviceUUID)
	default:
		return 0, nil
	}
	return s.queryInstantVal(ctx, query)
}

// 风扇转速
func (s *MetricsGenerator) fanSpeed(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FAN_SPEED{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_fan_speed{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_link_speed{vdie_id=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryInstantVal(ctx, query)
}

type DeviceAdditionalInfo struct {
	DriverVersion string // 驱动版本
	DeviceNo      string // 设备号
}

// 设备的驱动版本、设备号等信息
func (s *MetricsGenerator) queryDeviceAdditional(ctx context.Context, provider, deviceUUID string) (*DeviceAdditionalInfo, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"}", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("mlu_power_usage{uuid=\"%s\"}", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("dcu_power_usage{device_id=\"%s\"}", deviceUUID)
	default:
		return nil, errors.New("provider not exists")
	}
	res, err := s.monitorService.QueryInstant(context.TODO(), &pb.QueryInstantRequest{
		Query: query,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Data) > 0 {
		sample := res.Data[0]
		metric := sample.Metric
		info := &DeviceAdditionalInfo{}
		switch provider {
		case biz.NvidiaGPUDevice:
			info.DriverVersion = metric["DCGM_FI_DRIVER_VERSION"]
			info.DeviceNo = metric["device"]
		case biz.CambriconGPUDevice:
			info.DriverVersion = metric["driver"]
			info.DeviceNo = metric["sn"]
		case biz.HygonGPUDevice:
			info.DriverVersion = "暂无"
			info.DeviceNo = "dcu-" + metric["minor_number"]
		}
		return info, nil
	}
	return nil, fmt.Errorf("unknown error")
}

func (s *MetricsGenerator) systemComponentHealth(ctx context.Context, componentType, componentName string) (float32, error) {
	query := ""
	switch componentType {
	case biz.ComponentTypeDeployment:
		query = fmt.Sprintf("ALERTS{alertname=\"KubeDeploymentReplicasMismatch\",deployment=\"%s\"} ", componentName)
	case biz.ComponentTypeStatefulSet:
		query = fmt.Sprintf("ALERTS{alertname=\"KubeStatefulSetReplicasMismatch\", statefulset=\"%s\"}", componentName)
	case biz.ComponentTypeDaemonSet:
		query = fmt.Sprintf("(kube_daemonset_status_desired_number_scheduled{daemonset=\"%s\"} > kube_daemonset_status_number_available{daemonset=\"%s\"})", componentName, componentName)
	default:
		return 0, errors.New("componentType not exists")
	}
	return s.queryInstantVal(ctx, query)
}
