package exporter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	pb "vgpu/api/v1"
	"vgpu/internal/biz"
	"vgpu/internal/conf"
	"vgpu/internal/data/prom"
	"vgpu/internal/provider/metax"
	providerutil "vgpu/internal/provider/util"
	"vgpu/internal/service"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

// ProviderSet is service providers.
var ProviderSet = wire.NewSet(
	NewMetricsGenerator,
)

const (
	defaultGenerateInterval = 30 * time.Second
	defaultGenerateTimeout  = 60 * time.Second
	bytesPerKiB             = 1024
	bytesPerMiB             = bytesPerKiB * 1024
)

var (
	errNoMetricData                 = errors.New("prometheus query returned no data")
	errInvalidCoreCapacity          = errors.New("allocated core capacity must be greater than zero")
	errWorkloadTelemetryUnsupported = errors.New("workload telemetry is not supported by provider")
	errFanSpeedTelemetryUnsupported = errors.New("fan speed telemetry is not supported by provider")
)

type instantQuerier interface {
	QueryInstant(context.Context, *pb.QueryInstantRequest) (*pb.InstantResponse, error)
}

// MetricsGenerator owns the lifecycle of the background /metrics collector.
//
// It satisfies kratos transport.Server: Start spins up a single refresh goroutine that
// periodically re-fans out PromQL into the in-memory Prometheus registry; Stop waits
// for the goroutine to drain.
//
// The HTTP /metrics handler does NOT call GenerateMetrics directly anymore: scrapes
// only serialize whatever the registry currently holds, which is what makes the
// endpoint scrape-timeout safe even at customer-scale fanout (~1800 PromQL/cycle).
type MetricsGenerator struct {
	promClient     *prom.Client
	nodeUsecase    *biz.NodeUsecase
	podUsecase     *biz.PodUseCase
	monitorService instantQuerier

	interval time.Duration
	timeout  time.Duration

	log *log.Helper

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	done      chan struct{}

	runMu      sync.Mutex
	generating bool

	// Diff-based cell tracking. See cells.go for the full rationale.
	// current holds tuples written during the in-progress cycle; prev holds the
	// last successfully-committed cycle so we know which tuples to delete when a
	// device or container disappears.
	cellMu  sync.Mutex
	current map[cellKey]cell
	prev    map[cellKey]cell

	cacheTime time.Time
}

// roundToTwoDecimal rounds value to two decimal places.
func roundToTwoDecimal(value float64) float64 {
	return float64(math.Round(value*100) / 100)
}

// roundToOneDecimal rounds value to one decimal place.
func roundToOneDecimal(value float64) float64 {
	return math.Round(value*10) / 10
}

func NewMetricsGenerator(
	bc *conf.Bootstrap,
	promClient *prom.Client,
	nodeUsecase *biz.NodeUsecase,
	podUsecase *biz.PodUseCase,
	monitorService *service.MonitorService,
	logger log.Logger,
) *MetricsGenerator {
	interval, timeout := resolveCollectorIntervals(bc)
	return &MetricsGenerator{
		promClient:     promClient,
		nodeUsecase:    nodeUsecase,
		podUsecase:     podUsecase,
		monitorService: monitorService,
		interval:       interval,
		timeout:        timeout,
		log:            log.NewHelper(log.With(logger, "module", "exporter")),
	}
}

func resolveCollectorIntervals(bc *conf.Bootstrap) (interval, timeout time.Duration) {
	interval, timeout = defaultGenerateInterval, defaultGenerateTimeout
	if bc == nil || bc.Exporter == nil {
		return
	}
	if d := bc.Exporter.Interval.AsDuration(); d > 0 {
		interval = d
	}
	if d := bc.Exporter.Timeout.AsDuration(); d > 0 {
		timeout = d
	}
	return
}

// Start launches the background collector loop. It is invoked once by the kratos
// app lifecycle. The loop deliberately runs against context.Background() so a
// graceful shutdown is the only thing that can cancel it; per-cycle ctx is bounded
// by s.timeout.
func (s *MetricsGenerator) Start(_ context.Context) error {
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.done = make(chan struct{})
		s.log.Infow("msg", "starting metrics collector", "interval", s.interval, "timeout", s.timeout)
		go s.loop(ctx)
	})
	return nil
}

// Stop signals the loop to exit and waits for it to drain or for the caller's
// ctx to expire.
func (s *MetricsGenerator) Stop(ctx context.Context) error {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.done != nil {
			select {
			case <-s.done:
			case <-ctx.Done():
				s.log.Warn("msg", "metrics collector did not stop before shutdown deadline")
			}
		}
	})
	return nil
}

func (s *MetricsGenerator) loop(ctx context.Context) {
	defer close(s.done)
	s.runOnce(ctx)
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce executes a single refresh cycle with a bounded context. It refuses to
// overlap cycles: a cycle that runs longer than s.interval simply skips the next
// tick instead of piling up parallel PromQL fanouts onto vmselect.
func (s *MetricsGenerator) runOnce(parent context.Context) {
	s.runMu.Lock()
	if s.generating {
		s.runMu.Unlock()
		s.log.Warnw("msg", "skip metrics refresh: previous cycle still running")
		return
	}
	s.generating = true
	s.runMu.Unlock()
	defer func() {
		s.runMu.Lock()
		s.generating = false
		s.runMu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	start := time.Now()
	if err := s.GenerateMetrics(ctx); err != nil {
		// Partial cycle (ctx canceled, upstream error, etc.): keep last known good
		// snapshot intact so the registry doesn't briefly lose cells that simply
		// could not be re-queried this round.
		s.dropCurrentCycle()
		s.log.Errorw("msg", "metrics refresh failed (partial cycle, last-known cells retained)", "err", err, "elapsed", time.Since(start))
		return
	}
	s.commitCycle()
	s.log.Debugw("msg", "metrics refresh complete", "elapsed", time.Since(start))
}

// GenerateMetrics runs one full fanout cycle (device + container dimensions) into
// the in-memory Prometheus registry. It is exported so it can be unit-tested and
// triggered manually if needed, but on the request path it must never be called
// synchronously: the registry is the only thing /metrics serves.
//
// The function intentionally does NOT clear existing gauges. Each Set goes
// through s.set, which both writes the value and records the (gauge, labels)
// tuple. runOnce decides whether to commit the new snapshot (delete tuples that
// disappeared) or drop it (keep the previous snapshot intact). See cells.go.
func (s *MetricsGenerator) GenerateMetrics(ctx context.Context) error {
	s.cellMu.Lock()
	s.current = make(map[cellKey]cell)
	s.cellMu.Unlock()

	s.GenerateDeviceMetrics(ctx)
	s.GenerateContainerMetrics(ctx)

	// Surface ctx cancellation so runOnce treats this as a partial cycle and
	// preserves the prior snapshot. Without this guard, a mid-cycle timeout
	// would silently commit an incomplete map and erase real series.
	if err := ctx.Err(); err != nil {
		return err
	}
	s.cacheTime = time.Now()
	return nil
}

// GenerateDeviceMetrics refreshes device-level metrics from inventory and provider telemetry.
func (s *MetricsGenerator) GenerateDeviceMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	for _, device := range deviceInfos {
		provider := device.Provider
		deviceAdditional, err := s.queryDeviceAdditional(ctx, provider, device.Id)
		var driver, deviceNo = "", ""
		if err == nil && deviceAdditional != nil {
			driver = deviceAdditional.DriverVersion
			deviceNo = deviceAdditional.DeviceNo
		}

		s.set(HamiVgpuCount, float64(device.Count), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		s.set(HamiVmemorySize, float64(device.Devmem), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		s.set(HamiVcoreSize, float64(device.Devcore), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		s.set(HamiVCoreScaling, float64(device.Devcore)/100, device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		s.set(HamiCoreSize, float64(biz.PhysicalCoreBaselinePerDevice), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		deviceMemUsed, memoryUsedErr := s.deviceMemUsed(ctx, provider, device.Id)
		if memoryUsedErr == nil {
			s.set(HamiMemoryUsed, float64(deviceMemUsed), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		deviceMemSize, memorySizeErr := s.deviceMemTotal(ctx, provider, device.Id)
		if memorySizeErr == nil && deviceMemSize > 0 {
			s.set(HamiMemorySize, float64(deviceMemSize), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			s.set(HamiVMemoryScaling, roundToOneDecimal(float64(float32(device.Devmem)/deviceMemSize)), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			if memoryUsedErr == nil {
				s.set(HamiMemoryUtil, roundToOneDecimal(100*float64(deviceMemUsed/deviceMemSize)), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			}
		}
		actualCoreUtil, err := s.deviceCoreUtil(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiCoreUsed, float64(actualCoreUtil), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			s.set(HamiCoreUtil, float64(actualCoreUtil), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		actualCoreUtilAvg, err := s.deviceCoreUtil(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiCoreUsedAvg, float64(actualCoreUtilAvg), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			s.set(HamiCoreUtilAvg, float64(actualCoreUtilAvg), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		gpuTemperature, err := s.gpuTemperature(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiDeviceTemperature, float64(gpuTemperature), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		memoryTemperature, err := s.memoryTemperature(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiDeviceMemoryTemperature, float64(memoryTemperature), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		gpuPower, err := s.gpuPower(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiDevicePower, float64(gpuPower), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
		fanSpeed, err := s.fanSpeed(ctx, provider, device.Id)
		if err == nil {
			switch provider {
			case biz.NvidiaGPUDevice:
				s.set(HamiDeviceFanSpeedP, float64(fanSpeed), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			case biz.CambriconGPUDevice:
				s.set(HamiDeviceFanSpeedR, float64(fanSpeed), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
			}
		}
		lastXIDErrorCode, err := s.lastXIDErrorCode(ctx, provider, device.Id)
		if err == nil {
			s.set(HamiDeviceLastXIDErrorCode, float64(lastXIDErrorCode), device.NodeName, provider, device.Type, device.Id, driver, deviceNo)
		}
	}
	return nil
}

func (s *MetricsGenerator) generateMetricsForMetaxGPU(containers []*biz.Container) {
	for _, c := range containers {
		if len(c.ContainerDevices) == 0 {
			continue
		}
		if c.ContainerDevices[0].Type != metax.MetaxGPUDevice {
			continue
		}
		// MetaX allocation UUIDs cannot currently be correlated with telemetry UUIDs,
		// so this path pairs the two lists by index and emits telemetry identities.
		var core []int32
		var memory []int32
		for _, cd := range c.ContainerDevices {
			core = append(core, cd.Usedcores)
			memory = append(memory, cd.Usedmem)
		}

		query := fmt.Sprintf("mx_memory_used{exported_namespace=\"%s\", exported_pod=\"%s\", exported_container=\"%s\", type=\"vram\"}",
			c.Namespace, c.PodName, c.Name)

		res, err := s.monitorService.QueryInstant(context.TODO(), &pb.QueryInstantRequest{
			Query: query,
		})
		if err != nil {
			continue
		}
		reportLen := min(len(res.Data), len(c.ContainerDevices))
		for i := range reportLen {
			deviceUUID := res.Data[i].Metric["uuid"]
			deviceType := res.Data[i].Metric["modelName"]
			nodeName := res.Data[i].Metric["Hostname"]

			podUIDLabel := fmt.Sprintf("%s:%s", c.Name, c.PodUID)
			s.set(HamiContainerVgpuAllocated, float64(1), nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace, podUIDLabel)
			s.set(HamiContainerVmemoryAllocated, float64(memory[i]), nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace, podUIDLabel)
			s.set(HamiContainerVcoreAllocated, float64(core[i]), nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace, podUIDLabel)

			taskMemoryUsed := float32(res.Data[i].Value) // KB
			s.set(HamiContainerMemoryUsed, float64(taskMemoryUsed/1024), nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace)
			s.set(HamiContainerMemoryUtil, roundToOneDecimal(100*float64(taskMemoryUsed/1024)/float64(memory[i])), nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace)

			taskCoreUsed, err := s.taskCoreUsed(context.TODO(), metax.MetaxGPUDevice, c.Namespace, c.PodName, c.Name, c.PodUID, deviceUUID, nodeName, -1)
			if err == nil {
				used := float64(taskCoreUsed)
				util := roundToOneDecimal(100 * float64(taskCoreUsed) / float64(core[0]))
				cardCoreUtil, err := s.deviceCoreUtil(context.TODO(), metax.MetaxGPUDevice, deviceUUID)
				if err == nil && used != 0 && cardCoreUtil > 95 {
					used = float64(cardCoreUtil) / 100 * float64(core[i])
					util = float64(cardCoreUtil)
				}
				s.set(HamiContainerCoreUsed, used, nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace)
				s.set(HamiContainerCoreUtil, util, nodeName, metax.MetaxGPUDevice, deviceType, deviceUUID, c.PodName, c.Name, c.Namespace)
			}
		}
	}
}

// GenerateContainerMetrics refreshes workload-level allocation and usage metrics.
func (s *MetricsGenerator) GenerateContainerMetrics(ctx context.Context) error {
	deviceInfos, err := s.nodeUsecase.ListAllDevices(ctx)
	if err != nil {
		return err
	}
	containers, err := s.podUsecase.ListAll(ctx)
	if err != nil {
		return err
	}

	s.generateMetricsForMetaxGPU(containers)
	for _, device := range deviceInfos {
		for _, c := range containers {
			var vGPU int32 = 0
			var core int32 = 0
			var memory int32 = 0
			var provider string = ""
			coreAllocationKnown := true
			for _, cd := range c.ContainerDevices {
				if device.AliasId != "" && !strings.HasPrefix(cd.UUID, device.AliasId) {
					continue
				}
				vGPU = vGPU + 1
				core = core + cd.Usedcores
				memory = memory + cd.Usedmem
				provider = providerutil.VendorOf(cd.Type)
				if provider == biz.AscendGPUDevice && !cd.CoreAllocationKnown {
					coreAllocationKnown = false
				}
			}
			if provider == "" || provider == metax.MetaxGPUDevice {
				continue
			}
			podUIDLabel := fmt.Sprintf("%s:%s", c.Name, c.PodUID)
			s.set(HamiContainerVgpuAllocated, float64(vGPU), device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, podUIDLabel)
			s.set(HamiContainerVmemoryAllocated, float64(memory), device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, podUIDLabel)
			allocationKnown := float64(1)
			if !coreAllocationKnown {
				allocationKnown = 0
			}
			s.set(HamiContainerVcoreAllocationKnown, allocationKnown, device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, podUIDLabel)
			if coreAllocationKnown {
				s.set(HamiContainerVcoreAllocated, float64(core), device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace, podUIDLabel)
				used, util, err := s.containerCoreMetrics(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id, device.NodeName, device.Index, core)
				if err == nil {
					s.set(HamiContainerCoreUsed, used, device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace)
					s.set(HamiContainerCoreUtil, util, device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace)
				}
			}
			taskMemoryUsed, err := s.taskMemoryUsed(ctx, provider, c.Namespace, c.PodName, c.Name, c.PodUID, device.Id, device.NodeName, device.Index)
			if err == nil {
				switch provider {
				case biz.CambriconGPUDevice:
					taskMemoryUsed = float32((taskMemoryUsed/100)*float32(memory)) * 1024 * 1024
				case metax.MetaxSGPUDevice:
					taskMemoryUsed = float32(taskMemoryUsed) * 1024 // KB->Byte
				default:
				}
				s.set(HamiContainerMemoryUsed, float64(taskMemoryUsed/1024/1024), device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace)
				s.set(HamiContainerMemoryUtil, roundToOneDecimal(100*float64(taskMemoryUsed/1024/1024)/float64(memory)), device.NodeName, provider, device.Type, device.Id, c.PodName, c.Name, c.Namespace)
			}
		}
	}
	return nil
}

func (s *MetricsGenerator) queryInstantVal(ctx context.Context, query string) (float32, error) {
	val, _, err := s.queryInstantValWithPresence(ctx, query)
	return val, err
}

func (s *MetricsGenerator) queryInstantValWithPresence(ctx context.Context, query string) (float32, bool, error) {
	res, err := s.monitorService.QueryInstant(ctx, &pb.QueryInstantRequest{
		Query: query,
	})
	if err != nil {
		return 0, false, err
	}
	if len(res.Data) > 0 {
		return res.Data[0].Value, true, nil
	}
	return 0, false, nil
}

func (s *MetricsGenerator) queryRequiredInstantVal(ctx context.Context, query string) (float32, error) {
	val, present, err := s.queryInstantValWithPresence(ctx, query)
	if err != nil {
		return 0, err
	}
	if !present || math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
		return 0, errNoMetricData
	}
	return val, nil
}

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
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_memory_used{uuid=\"%s\", type=\"vram\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryRequiredInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	return deviceMemoryToMiB(provider, val), nil
}

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
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_memory_total{uuid=\"%s\", type=\"vram\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	val, err := s.queryRequiredInstantVal(ctx, query)
	if err != nil {
		return val, err
	}
	return deviceMemoryToMiB(provider, val), nil
}

func deviceMemoryToMiB(provider string, value float32) float32 {
	switch provider {
	case biz.CambriconGPUDevice, biz.HygonGPUDevice:
		return value / bytesPerMiB
	case biz.MetaxGPUDevice:
		return value / bytesPerKiB
	default:
		return value
	}
}

func (s *MetricsGenerator) deviceCoreUtil(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("DCGM_FI_DEV_GPU_UTIL{UUID=\"%s\"}", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization{uuid=\"%s\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_utilization{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_utilizationrate{device_id=\"%s\"})", deviceUUID)
	case biz.MetaxGPUDevice, metax.MetaxGPUDevice, metax.MetaxSGPUDevice:
		query = fmt.Sprintf("avg(mx_gpu_usage{uuid=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryRequiredInstantVal(ctx, query)
}

func (s *MetricsGenerator) taskCoreUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID, hostname string, deviceIndex int) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = nvidiaTaskCoreUsedQuery(deviceUUID, namespace, pod, container)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vcore\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		return 0, errWorkloadTelemetryUnsupported
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(vdcu_percent{pod_uuid=\"%s\", container_name=\"%s\"})", podUUID, container)
	case biz.MetaxGPUDevice, metax.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_gpu_usage{uuid=\"%s\", exported_namespace=\"%s\", exported_pod=\"%s\", exported_container=\"%s\"})", deviceUUID, namespace, pod, container)
	case metax.MetaxSGPUDevice:
		query = fmt.Sprintf("avg(mx_sgpu_usage{Hostname=\"%s\", deviceId=\"%d\", exported_namespace=\"%s\", exported_pod=\"%s\", exported_container=\"%s\"})",
			hostname, deviceIndex, namespace, pod, container)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryRequiredInstantVal(ctx, query)
}

func nvidiaTaskCoreUsedQuery(deviceUUID, namespace, pod, container string) string {
	selector := fmt.Sprintf("hami_container_device_utilization_ratio{device_uuid=\"%s\", namespace=\"%s\", pod=\"%s\", container=\"%s\"}", deviceUUID, namespace, pod, container)
	return fmt.Sprintf("avg(avg_over_time(%s[1m]))", selector)
}

// containerCoreMetrics preserves each provider's existing task-compute conversion.
// For NVIDIA, used estimates active allocated compute in vCore percentage points
// and excludes elastic borrowing; util is allocated-compute activity (0-100), not
// physical-card utilization.
func (s *MetricsGenerator) containerCoreMetrics(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID, hostname string, deviceIndex int, allocatedCore int32) (float64, float64, error) {
	if allocatedCore <= 0 {
		return 0, 0, errInvalidCoreCapacity
	}

	taskCoreUsed, err := s.taskCoreUsed(ctx, provider, namespace, pod, container, podUUID, deviceUUID, hostname, deviceIndex)
	if err != nil {
		return 0, 0, err
	}

	used := float64(0)
	util := float64(0)
	switch provider {
	case biz.NvidiaGPUDevice:
		rawActivity := float64(taskCoreUsed)
		if math.IsNaN(rawActivity) || math.IsInf(rawActivity, 0) {
			return 0, 0, errNoMetricData
		}
		activity := math.Max(0, math.Min(100, rawActivity))
		used = roundToTwoDecimal(math.Min(100, activity*float64(allocatedCore)/100))
		util = roundToOneDecimal(activity)
		return used, util, nil
	case biz.CambriconGPUDevice:
		used = float64(taskCoreUsed) / 100 * float64(allocatedCore)
		util = float64(taskCoreUsed)
	case biz.HygonGPUDevice, metax.MetaxSGPUDevice:
		used = float64(taskCoreUsed)
		util = roundToOneDecimal(100 * float64(taskCoreUsed) / float64(allocatedCore))
	}

	// Keep the legacy fallback for providers whose task-level metrics still need it.
	// It must not be used for NVIDIA: physical-card activity cannot be attributed to
	// one container when several workloads share the same GPU.
	cardCoreUtil, err := s.deviceCoreUtil(ctx, provider, deviceUUID)
	if err == nil && used != 0 && cardCoreUtil > 95 {
		used = float64(cardCoreUtil) / 100 * float64(allocatedCore)
		util = float64(cardCoreUtil)
	}
	return used, util, nil
}

func (s *MetricsGenerator) taskMemoryUsed(ctx context.Context, provider, namespace, pod, container, podUUID, deviceUUID, hostname string, deviceIndex int) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(hami_vgpu_memory_used_bytes{device_uuid=\"%s\", namespace=\"%s\", pod=\"%s\", container=\"%s\"})", deviceUUID, namespace, pod, container)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_utilization * on(uuid) group_right mlu_container{namespace=\"%s\",pod=\"%s\",container=\"%s\",type=\"mlu370.smlu.vmemory\"})", namespace, pod, container)
	case biz.AscendGPUDevice:
		return 0, errWorkloadTelemetryUnsupported
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(vdcu_usage_memory_size{pod_uuid=\"%s\", container_name=\"%s\"})", podUUID, container)
	case metax.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_memory_used{uuid=\"%s\", exported_namespace=\"%s\", exported_pod=\"%s\", exported_container=\"%s\", type=\"vram\"})", deviceUUID, namespace, pod, container)
	case metax.MetaxSGPUDevice:
		query = fmt.Sprintf("avg(mx_sgpu_used_memory{Hostname=\"%s\", deviceId=\"%d\", exported_namespace=\"%s\", exported_pod=\"%s\", exported_container=\"%s\"})",
			hostname, deviceIndex, namespace, pod, container)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryRequiredInstantVal(ctx, query)
}

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
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_chip_hotspot_temp{uuid=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryRequiredInstantVal(ctx, query)
}

func (s *MetricsGenerator) memoryTemperature(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_MEMORY_TEMP{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_memory_temperature{uuid=\"%s\",memory_die=\"\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_hbm_temperature{vdie_id=\"%s\"})", deviceUUID)
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_chip_hbm_temp{uuid=\"%s\"})", deviceUUID)
	default:
		return 0, errors.New("provider not exists")
	}
	return s.queryRequiredInstantVal(ctx, query)
}

func (s *MetricsGenerator) gpuPower(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_power_usage{uuid=\"%s\",vf=\"\"})", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("avg(npu_chip_info_power{vdie_id=\"%s\"})", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("avg(dcu_power_usage{device_id=\"%s\"})", deviceUUID)
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("avg(mx_board_power{uuid=\"%s\"})", deviceUUID) // mW
	default:
		return 0, errors.New("provider not exists")
	}

	power, err := s.queryRequiredInstantVal(ctx, query)
	if err == nil && provider == biz.MetaxGPUDevice {
		power = power / 1000 // mW -> W
	}
	return power, err
}

func (s *MetricsGenerator) lastXIDErrorCode(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_XID_ERRORS{UUID=\"%s\"})", deviceUUID)
	default:
		return 0, fmt.Errorf("last XID error code is not supported by provider %q", provider)
	}
	return s.queryRequiredInstantVal(ctx, query)
}

func (s *MetricsGenerator) fanSpeed(ctx context.Context, provider, deviceUUID string) (float32, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("avg(DCGM_FI_DEV_FAN_SPEED{UUID=\"%s\"})", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("avg(mlu_fan_speed{uuid=\"%s\"})", deviceUUID)
	default:
		return 0, errFanSpeedTelemetryUnsupported
	}
	fanSpeed, err := s.queryRequiredInstantVal(ctx, query)
	if err != nil {
		return 0, err
	}
	if provider == biz.CambriconGPUDevice && fanSpeed == -1 {
		return 0, errNoMetricData
	}
	return fanSpeed, nil
}

type DeviceAdditionalInfo struct {
	DriverVersion string
	DeviceNo      string
}

func (s *MetricsGenerator) queryDeviceAdditional(ctx context.Context, provider, deviceUUID string) (*DeviceAdditionalInfo, error) {
	query := ""
	switch provider {
	case biz.NvidiaGPUDevice:
		query = fmt.Sprintf("DCGM_FI_DEV_POWER_USAGE{UUID=\"%s\"}", deviceUUID)
	case biz.AscendGPUDevice:
		query = fmt.Sprintf("npu_chip_info_power{vdie_id=\"%s\"}", deviceUUID)
	case biz.CambriconGPUDevice:
		query = fmt.Sprintf("mlu_power_usage{uuid=\"%s\",vf=\"\"}", deviceUUID)
	case biz.HygonGPUDevice:
		query = fmt.Sprintf("dcu_power_usage{device_id=\"%s\"}", deviceUUID)
	case biz.MetaxGPUDevice:
		query = fmt.Sprintf("mx_board_power{uuid=\"%s\"}", deviceUUID)
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
		case biz.AscendGPUDevice:
			info.DeviceNo = "ascend-" + metric["id"]
		case biz.HygonGPUDevice:
			info.DeviceNo = "dcu-" + metric["minor_number"]
		case biz.MetaxGPUDevice:
			info.DriverVersion = metric["driver_version"]
			info.DeviceNo = metric["deviceId"]
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
