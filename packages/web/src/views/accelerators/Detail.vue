<template>
  <div>
    <page-header
      :title="dt('card.detail.title')"
      :name="headerName"
      :status="headerStatusDisplay.text"
      :status-icon="headerStatusDisplay.icon"
    />
    <detail-page-state :status="detailStatus" @retry="retryDetail">
      <template #loading>
        <block-box class="node-block" :title="$t('card.detail.detailInfo')" variant="detail">
          <DeviceFacts loading :readings="sensorReadings" />
        </block-box>
        <block-box class="resource-overview-block" :title="$t('card.detail.resourceOverview')" variant="detail">
          <div class="resource-overview-layout">
            <ul class="resource-overview-cards">
              <li v-for="title in [$t('dashboard.computePowerTotal'), dt('dashboard.memoryTotal')]" :key="title" class="resource-overview-card">
                <div class="resource-card">
                  <div class="resource-card-header">
                    <div class="resource-card-icon"><LoadingValue :height="24" width="24px" /></div>
                    <div class="resource-card-header-info">
                      <div class="resource-card-value resource-card-value--compute"><LoadingValue :height="22" width="100px" /></div>
                      <div class="resource-card-sub-title">{{ title }}</div>
                    </div>
                  </div>
                  <div class="resource-card-footer">
                    <div v-for="label in [$t('dashboard.allocated'), $t('dashboard.used')]" :key="label" class="resource-card-rate-wrap">
                      <div class="resource-card-footer-item">
                        <div class="resource-card-footer-title">{{ label }}</div>
                        <div class="resource-card-footer-value"><LoadingValue :height="24" width="120px" /></div>
                      </div>
                    </div>
                  </div>
                </div>
              </li>
            </ul>
          </div>
        </block-box>
        <TrendTimeFilter :model-value="times" loading class="card-trend-filter" />
        <div class="line-box">
          <ResourceTrend kind="compute" />
          <ResourceTrend kind="memory" />
          <block-box v-for="item in lineToolsView" :key="item.titleKey" :title="item.title">
            <MetricChart :option="item.option" />
          </block-box>
        </div>
      </template>
    <block-box class="node-block" :title="$t('card.detail.detailInfo')" variant="detail">
      <DeviceFacts
        :device="detail"
        :node-location="nodeDetailLocation"
        :mode="splitModeText"
        :mode-icon="splitModeIcon"
        :readings="sensorReadings"
      />
    </block-box>

    <block-box class="resource-overview-block" :title="$t('card.detail.resourceOverview')" variant="detail">
      <div class="resource-overview-layout" :class="{ 'has-slot-summary': allocationSlots }">
        <div v-if="allocationSlots" class="resource-slot-card">
          <div class="resource-slot-gauge">
            <workload-semi-progress :percent="allocationSlots.percent" />
            <div class="resource-slot-summary">
              <div class="resource-slot-value">
                <b>{{ allocatedSlotsText }}</b><span>/ {{ slotLimitText }}</span>
              </div>
              <div class="resource-slot-label">
                <span>{{ $t('card.detail.allocatedSlots') }}</span>
                <metric-help
                  :description="$t('card.detail.allocatedSlotsTip')"
                  :help-label="$t('dashboard.metricHelpLabel', { metric: $t('card.detail.allocatedSlots') })"
                />
              </div>
            </div>
          </div>
        </div>
        <ul class="resource-overview-cards">
          <li class="resource-overview-card">
            <div class="resource-card">
              <div class="resource-card-header">
                <div class="resource-card-icon">
                  <svg-icon icon="vgpu-core" />
                </div>
                <div class="resource-card-header-info">
                  <div class="resource-card-value resource-card-value--compute">
                    {{ computeTotalText }}
                  </div>
                  <div class="resource-card-sub-title">
                    {{ $t('dashboard.computePowerTotal') }}
                  </div>
                </div>
              </div>

              <div class="resource-card-footer">
                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">
                      <span class="resource-card-footer-label">
                        {{ $t('dashboard.allocated') }}
                      </span>
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-reading">
                        <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ computeAllocUsedText }}</span>&nbsp;
                        <t-tooltip v-if="computeAllocNote" :content="computeAllocNote">
                          <span>(<span class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>)</span>
                        </t-tooltip>
                        <span v-else>(<span class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>)</span>
                      </span>
                      <span v-if="computeAllocNote" class="resource-card-sr-only">{{ computeAllocNote }}</span>
                      <t-progress
                        v-if="computeAllocPercentProgress !== undefined"
                        theme="circle"
                        :percentage="computeAllocPercentProgressRounded"
                        size="24"
                        :color="getResourceColor(computeAllocPercentProgress)"
                        :label="false"
                      />
                    </div>
                  </div>
                </div>

                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">
                      <span class="resource-card-footer-label">
                        {{ $t('dashboard.used') }}
                      </span>
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-reading">
                        <span class="resource-card-footer-metric">{{ computeUsageUsedText }}</span>
                        (<span class="resource-card-footer-percent">{{ computeUsagePercentText }}</span>)
                      </span>
                      <t-progress
                        v-if="computeUsagePercentProgress !== undefined"
                        theme="circle"
                        :percentage="computeUsagePercentProgressRounded"
                        size="24"
                        :color="getResourceColor(computeUsagePercentProgress)"
                        :label="false"
                      />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </li>
          <li class="resource-overview-card">
            <div class="resource-card">
              <div class="resource-card-header">
                <div class="resource-card-icon">
                  <svg-icon icon="node-memory-total" />
                </div>
                <div class="resource-card-header-info">
                  <div class="resource-card-value resource-card-value--compute">{{ memoryTotalText }}</div>
                  <div class="resource-card-sub-title">
                    {{ dt('dashboard.memoryTotal') }}
                  </div>
                </div>
              </div>

              <div class="resource-card-footer">
                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">
                      <span class="resource-card-footer-label">
                        {{ $t('dashboard.allocated') }}
                      </span>
                      <metric-help
                        :description="$t('dashboard.memAllocRateDescription')"
                        :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memAllocRate') })"
                      />
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-reading">
                        <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ memoryAllocUsedText }}</span>
                        (<span class="resource-card-footer-percent">{{ memoryAllocPercentText }}</span>)
                      </span>
                      <t-progress
                        v-if="memoryAllocPercentProgress !== undefined"
                        theme="circle"
                        :percentage="memoryAllocPercentProgressRounded"
                        size="24"
                        :color="getResourceColor(memoryAllocPercentProgress)"
                        :label="false"
                      />
                    </div>
                  </div>
                </div>

                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">
                      <span class="resource-card-footer-label">
                        {{ migInstanceMemory ? $t('dashboard.migInstancesUsed') : $t('dashboard.used') }}
                      </span>
                      <metric-help
                        :description="memoryUsageHelp"
                        :help-label="$t('dashboard.metricHelpLabel', { metric: $t(migInstanceMemory ? 'dashboard.migInstancesUsed' : 'dashboard.memUsageRate') })"
                      />
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-reading">
                        <span class="resource-card-footer-metric">{{ memoryUsageUsedText }}</span>
                        <span v-if="!migInstanceMemory || migInstanceMemory.percent !== undefined">
                          (<span class="resource-card-footer-percent">{{ memoryUsagePercentText }}</span>)
                        </span>
                      </span>
                      <t-progress
                        v-if="memoryUsagePercentProgress !== undefined"
                        theme="circle"
                        :percentage="memoryUsagePercentProgressRounded"
                        size="24"
                        :color="getResourceColor(memoryUsagePercentProgress)"
                        :label="false"
                      />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </li>
        </ul>
      </div>
    </block-box>

    <block-box v-if="splitVisible" class="device-split-block" :title="$t('card.split.title')" variant="detail">
      <template v-if="detail.mode === 'mig'" #extra>
        <MetricHelp multiline :description="$t('card.split.help')" :help-label="$t('card.split.title')" />
      </template>
      <DeviceSplit
        :device="detail"
        :containers="cardContainers"
        :total-containers="splitTotal"
        :truncated="splitTruncated"
        :status="splitStatus"
        :show-shared-count="false"
        @retry="loadSplit"
      />
    </block-box>

    <block-box v-if="npuSpecVisible" class="npu-spec-block" :title="$t('card.deviceConfig.title')" variant="detail">
      <p v-if="deviceConfigStateText" class="npu-spec-note">{{ deviceConfigStateText }}</p>
      <template v-else>
        <div class="npu-spec-facts">
          <div v-for="fact in ascendModelFacts" :key="fact.key" class="npu-spec-fact">
            <span class="npu-spec-fact-icon"><svg-icon :icon="fact.icon" aria-hidden="true" /></span>
            <span>
              <span class="npu-spec-fact-value">{{ fact.value }}</span>
              <span class="npu-spec-fact-label">{{ fact.label }}</span>
            </span>
          </div>
        </div>
        <p v-if="ascendModel.superPod" class="npu-spec-note npu-spec-super-pod">{{ $t('card.deviceConfig.superPod') }}</p>
        <!-- HAMi-core allocates what is requested; templates never apply to this card. -->
        <template v-if="detail.mode !== 'hami-core'">
          <div class="npu-spec-subtitle">
            {{ $t('card.deviceConfig.templates') }}
            <MetricHelp
              v-if="ascendModel.templates.length"
              multiline
              :description="$t('card.deviceConfig.roundingHint')"
              :help-label="$t('card.deviceConfig.roundingHintLabel')"
            />
          </div>
          <p v-if="!ascendModel.templates.length" class="npu-spec-note">{{ $t('card.deviceConfig.noTemplates') }}</p>
          <div class="npu-spec-options">
            <NpuAllocationOption v-for="option in allocationOptions" :key="option.whole ? '' : option.name" :option="option" />
          </div>
        </template>
      </template>
    </block-box>

    <TrendTimeFilter
      v-model="times"
      :loading="trendLoading"
      :min-range-ms="DEVICE_TREND_MIN_RANGE_MS"
      :max-range-ms="DEVICE_TREND_MAX_RANGE_MS"
      class="card-trend-filter"
    />
    <div class="line-box">
      <ResourceTrend kind="compute" :series="computeTrend" :vendor="detail.vendor" />
      <ResourceTrend kind="memory" :series="memoryTrend" :vendor="detail.vendor" />

      <block-box
        v-for="item in lineToolsView"
        :key="item.titleKey"
        :title="item.title"
      >
        <MetricChart
          :status="item.status"
          :option="item.option"
          :state-text="item.stateText"
          :refreshing="item.refreshing"
          :refresh-error="Boolean(item.refreshError)"
        />
      </block-box>
    </div>
    </detail-page-state>
  </div>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import TrendTimeFilter from '@/components/TrendTimeFilter.vue';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import DetailPageState from '@/components/DetailPageState.vue';
import LoadingValue from '@/components/LoadingValue.vue';
import MetricHelp from '@/components/MetricHelp.vue';
import { ref, watch, computed, onUnmounted } from 'vue';
import {
  isLowerBound,
  isNothingCounted,
  lowerBoundMessage,
  nothingCountedMessage,
} from '@/metrics/uncounted.mjs';
import useDetailResource from '@/hooks/useDetailResource.js';
import { classifyDetailPayload } from '@/hooks/detail-resource-state.mjs';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import apiClient, { ApiClientError } from '@/api/client.mjs';
import { useTelemetryTrends } from '@/hooks/useTelemetryTrends.mjs';
import { getDetailHealth } from '@/api/physical-inventory.mjs';
import WorkloadSemiProgress from './components/WorkloadSemiProgress.vue';
import { roundToDecimal, getResourceColor } from '@/utils';
import MetricChart from '@/components/MetricChart.vue';
import ResourceTrend from '@/components/ResourceTrend.vue';
import { buildTimeSeriesOptions } from '@/metrics/chart-presets.mjs';
import { CHART_COLORS } from '@/metrics/chart-colors.mjs';
import { stateTextKey } from '@/metrics/metric-state.mjs';
import { useI18n } from 'vue-i18n';
import { buildNodeDetailLocation } from '@/views/nodes/detail-location.mjs';
import { formatOptionalTelemetry } from './optional-telemetry-display.mjs';
import { getAllocationSlotDisplay } from './allocation-slot-display.mjs';
import DeviceFacts from './components/DeviceFacts.vue';
import { deviceWording, isNpuVendor } from '@/components/device-copy.mjs';
import { getSplitIcon, getSplitModeKey } from '@/components/split-mode.mjs';
import { buildAllocationOptions, findAscendModel, getDeviceConfigStateKey } from './device-config-display.mjs';
import NpuAllocationOption from './components/NpuAllocationOption.vue';
import { cardHealthDisplay } from './card-health.mjs';
import DeviceSplit from '@/components/DeviceSplit.vue';
import { parseDeviceSplitPage } from '@/components/device-split.mjs';

const DEVICE_TREND_MIN_RANGE_MS = 30 * 1000;
const DEVICE_TREND_MAX_RANGE_MS = 31 * 24 * 60 * 60 * 1000;
const DEVICE_TREND_METRICS = [
  { key: 'compute-allocation', metric: 'compute_allocation_percent', sectionIndex: 'compute' },
  { key: 'compute-usage', metric: 'compute_utilization_percent', sectionIndex: 'compute' },
  { key: 'memory-allocation', metric: 'memory_allocation_percent', sectionIndex: 'memory' },
  { key: 'memory-usage', metric: 'memory_utilization_percent', sectionIndex: 'memory' },
  { key: 'temperature', metric: 'temperature_celsius', sectionIndex: 'temperature' },
  { key: 'power', metric: 'power_watts', sectionIndex: 'power' },
];
const BYTES_PER_GIB = 1024 ** 3;

const route = useRoute();
const { t } = useI18n();

const routeCardUuid = computed(() => route.params.uuid);
const routeNodeUid = computed(() => {
  const value = route.query.nodeUid;
  if (typeof value === 'string') return value;
  if (Array.isArray(value) && typeof value[0] === 'string') return value[0];
  return '';
});
const routeDeviceTarget = computed(() => {
  const uuid = routeCardUuid.value;
  if (!uuid) return null;
  return { uuid: String(uuid), nodeUid: routeNodeUid.value };
});
const {
  data: detail,
  status: detailStatus,
  retry: retryDetail,
} = useDetailResource({
  source: routeDeviceTarget,
  isValidSource: (target) => Boolean(target?.uuid),
  request: async ({ uuid, nodeUid: targetNodeUid }, signal) => {
    try {
      return await apiClient.getDevice(uuid, {
        nodeUid: targetNodeUid || undefined,
        signal,
      });
    } catch (error) {
      if (error instanceof ApiClientError && error.status === 404 && error.reason === 'RESOURCE_GPU_NOT_FOUND') {
        return {};
      }
      throw error;
    }
  },
  classify: (payload, { uuid, nodeUid: targetNodeUid }) =>
    classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: targetNodeUid ? { uuid, nodeUid: targetNodeUid } : { uuid },
    }),
});
const isDetailReady = computed(
  () => detailStatus.value === REQUEST_STATUS.READY,
);
const dt = (key) => deviceWording(t(key), isDetailReady.value ? detail.value?.vendor : '');
const splitModeText = computed(() => {
  const key = isDetailReady.value ? getSplitModeKey(detail.value?.mode) : '';
  return key ? t(key) : '--';
});
const splitModeIcon = computed(() => (isDetailReady.value ? getSplitIcon(detail.value?.mode) : ''));
const detailCardUuid = computed(() =>
  isDetailReady.value ? detail.value.uuid : undefined,
);
const detailNodeUid = computed(() =>
  isDetailReady.value ? detail.value.nodeUid || detail.value.node_uid || '' : '',
);
// Plugins that register no split mode leave nothing to lay out.
const splitVisible = computed(() => Boolean(isDetailReady.value && getSplitModeKey(detail.value?.mode)));
const cardContainers = ref([]);
const splitTotal = ref(undefined);
const splitTruncated = ref(false);
const splitStatus = ref('loading');
let splitGeneration = 0;
let splitController = null;
const loadSplit = async () => {
  splitController?.abort();
  splitController = null;
  const generation = ++splitGeneration;
  const uuid = detailCardUuid.value;
  splitStatus.value = 'loading';
  splitTotal.value = undefined;
  splitTruncated.value = false;
  if (!uuid || !splitVisible.value) return;
  const controller = new AbortController();
  splitController = controller;
  try {
    const result = await apiClient.listContainers({
      deviceId: uuid,
      nodeUid: detailNodeUid.value || undefined,
      page: 1,
      pageSize: 100,
    }, { signal: controller.signal });
    if (controller.signal.aborted || generation !== splitGeneration) return;
    const page = parseDeviceSplitPage(result, 100);
    cardContainers.value = page.items;
    splitTotal.value = page.totalContainers;
    splitTruncated.value = page.truncated;
    splitStatus.value = 'ready';
  } catch {
    if (!controller.signal.aborted && generation === splitGeneration) splitStatus.value = 'error';
  } finally {
    if (splitController === controller) splitController = null;
  }
};
watch([detailCardUuid, detailNodeUid], loadSplit, { immediate: true });
const headerName = computed(() =>
  isDetailReady.value ? detail.value.uuid : routeCardUuid.value || '',
);
const nodeUid = ref('');
const nodeDetailLocation = computed(() =>
  buildNodeDetailLocation({
    uid: nodeUid.value,
    nodeName: detail.value?.nodeName,
  }),
);

const headerHealth = ref(null);
watch(detail, async (card, _, onCleanup) => {
  headerHealth.value = null;
  if (!card?.uuid || card.uuid !== routeCardUuid.value) return;
  if (routeNodeUid.value && card.nodeUid && card.nodeUid !== routeNodeUid.value) return;
  const controller = new AbortController();
  onCleanup(() => controller.abort());
  try {
    const health = await getDetailHealth(card, controller.signal);
    if (!controller.signal.aborted) headerHealth.value = health;
  } catch {
    if (!controller.signal.aborted) headerHealth.value = {};
  }
});
const headerStatusDisplay = computed(() =>
  isDetailReady.value && headerHealth.value
    ? cardHealthDisplay(headerHealth.value, t)
    : { icon: '', text: '' },
);

const deviceConfig = ref({ state: 'loading' });
const deviceConfigController = new AbortController();
apiClient.getDeviceConfig({ signal: deviceConfigController.signal })
  .then((reply) => {
    if (!deviceConfigController.signal.aborted) deviceConfig.value = reply || { state: 'error' };
  })
  .catch(() => {
    if (!deviceConfigController.signal.aborted) deviceConfig.value = { state: 'error' };
  });
onUnmounted(() => {
  splitController?.abort();
  splitController = null;
  deviceConfigController.abort();
});
const ascendModel = computed(() => findAscendModel(deviceConfig.value, detail.value?.type));
const npuSpecVisible = computed(() => isDetailReady.value && Boolean(
  ascendModel.value || (deviceConfig.value.state !== 'loaded' && isNpuVendor(detail.value?.vendor)),
));
const deviceConfigParams = computed(() => ({
  namespace: deviceConfig.value.namespace || '--',
  name: deviceConfig.value.name || '--',
}));
const deviceConfigStateText = computed(() => {
  const key = getDeviceConfigStateKey(deviceConfig.value);
  return key ? t(key, deviceConfigParams.value) : '';
});
const formatGiB = (mib) => (mib === undefined ? '--' : `${roundToDecimal(mib / 1024, 2)} GiB`);
const ascendModelFacts = computed(() => {
  const model = ascendModel.value;
  if (!model) return [];
  return [
    { key: 'aiCore', icon: 'vgpu-core', label: t('card.deviceConfig.aiCore'), value: model.aiCore ?? '--' },
    { key: 'aiCpu', icon: 'node-cpu-total', label: t('card.deviceConfig.aiCpu'), value: model.aiCpu ?? '--' },
    { key: 'memory', icon: 'node-memory-total', label: t('card.deviceConfig.memoryAllocatable'), value: formatGiB(model.memoryAllocatableMiB) },
  ];
});
const allocationOptions = computed(() => buildAllocationOptions(ascendModel.value));

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const trendParams = computed(() => (detailCardUuid.value
  ? {
      deviceId: detail.value?.telemetryDeviceId || detailCardUuid.value,
      nodeUid: detailNodeUid.value || undefined,
    }
  : {}));
const { data: rangeSeries } = useTelemetryTrends({
  scope: 'device',
  params: trendParams,
  metrics: DEVICE_TREND_METRICS,
  times,
});
const computeTrend = computed(() => rangeSeries.value.filter((item) => item.sectionIndex === 'compute'));
const memoryTrend = computed(() => rangeSeries.value.filter((item) => item.sectionIndex === 'memory'));

const toFiniteNumber = (value) => {
  if (value === undefined || value === null || value === '') return undefined;
  const num = Number(value);
  return Number.isFinite(num) ? num : undefined;
};

const computeTotalText = computed(() => {
  if (!isDetailReady.value || detail.value?.unconfigured) return '--';
  const total = toFiniteNumber(detail.value?.coreTotal);
  return total === undefined || total <= 0 ? '--' : `${roundToDecimal(total / 100, 1)}`;
});

const memoryTotalText = computed(() => {
  if (!isDetailReady.value || detail.value?.unconfigured) return '--';
  const totalMiB = toFiniteNumber(detail.value?.memoryTotal);
  return totalMiB === undefined || totalMiB <= 0 ? '--' : `${roundToDecimal(totalMiB / 1024, 1)} GiB`;
});

const formatUsedValue = (v, unit, divisor = 1) => {
  const n = Number(v);
  if (!Number.isFinite(n)) return '--';
  const normalizedDivisor = Number(divisor) > 0 ? Number(divisor) : 1;
  const text = `${roundToDecimal(n / normalizedDivisor, 1)}`;
  return unit && String(unit).trim() ? `${text} ${unit}` : text;
};

const computeAllocUncounted = computed(() => {
  if (!isDetailReady.value || !detail.value) return undefined;
  if (typeof detail.value.coreUnknownCount === 'number' && Number.isFinite(detail.value.coreUnknownCount)) {
    return Math.max(0, detail.value.coreUnknownCount);
  }
  return detail.value.coreUsedKnown === false ? null : 0;
});

const computeAllocUsedRaw = computed(() => (
  !isDetailReady.value || detail.value?.isExternal || detail.value?.unconfigured
    ? undefined
    : toFiniteNumber(detail.value?.coreUsed)
));

const computeAllocNothingCounted = computed(() =>
  isNothingCounted(computeAllocUsedRaw.value, computeAllocUncounted.value),
);

const computeAllocUsedText = computed(() =>
  computeAllocNothingCounted.value
    ? '--'
    : formatUsedValue(computeAllocUsedRaw.value, ' ', 100),
);

const computeUsageRatioRaw = computed(() => (
  isDetailReady.value ? toFiniteNumber(detail.value?.utilizationRatio) : undefined
));

const computeUsageUsedText = computed(() =>
  formatUsedValue(computeUsageRatioRaw.value, ' '),
);

const memoryAllocUsedMiB = computed(() => (
  !isDetailReady.value || detail.value?.isExternal || detail.value?.unconfigured
    ? undefined
    : toFiniteNumber(detail.value?.memoryUsed)
));

const memoryAllocUsedText = computed(() =>
  formatUsedValue(memoryAllocUsedMiB.value, 'GiB', 1024),
);

const wholeMemoryUsage = computed(() => {
  if (!isDetailReady.value || !detail.value) return null;
  const usedBytes = toFiniteNumber(detail.value.memoryUsedBytes);
  const physBytes = toFiniteNumber(detail.value.memoryPhysicalBytes);
  if (usedBytes === undefined || physBytes === undefined || physBytes <= 0) return null;
  return {
    usedGiB: usedBytes / BYTES_PER_GIB,
    percent: (usedBytes / physBytes) * 100,
  };
});

const migInstanceMemory = computed(() => {
  if (!isDetailReady.value || detail.value?.mode !== 'mig' || wholeMemoryUsage.value) return null;
  const mig = detail.value?.migInstanceMemory;
  if (!mig) return null;
  const usedBytes = toFiniteNumber(mig.usedBytes);
  if (usedBytes === undefined) return null;
  const totalBytes = toFiniteNumber(mig.totalBytes);
  return {
    usedGiB: usedBytes / BYTES_PER_GIB,
    totalGiB: totalBytes === undefined ? undefined : totalBytes / BYTES_PER_GIB,
    percent: totalBytes !== undefined && totalBytes > 0 ? (100 * usedBytes) / totalBytes : undefined,
  };
});

const memoryUsageHelp = computed(() => {
  if (!migInstanceMemory.value) {
    return t(
      isDetailReady.value && detail.value?.mode === 'mig' && !wholeMemoryUsage.value
        ? 'dashboard.migMemoryMissing'
        : 'dashboard.memUsageRateDescription',
    );
  }
  const { totalGiB, percent } = migInstanceMemory.value;
  return percent === undefined ? t('dashboard.migInstancesUsedHelp')
    : t('dashboard.migInstancesUsageHelp', { total: formatUsedValue(totalGiB, 'GiB') });
});

const memoryUsageUsedText = computed(() =>
  formatUsedValue(migInstanceMemory.value?.usedGiB ?? wholeMemoryUsage.value?.usedGiB, 'GiB'),
);

const computeAllocPercentRaw = computed(() => {
  if (computeAllocUsedRaw.value === undefined) return undefined;
  const total = toFiniteNumber(detail.value?.coreTotal);
  return total !== undefined && total > 0 ? (computeAllocUsedRaw.value / total) * 100 : undefined;
});
const memoryAllocPercentRaw = computed(() => {
  if (memoryAllocUsedMiB.value === undefined) return undefined;
  const totalMiB = toFiniteNumber(detail.value?.memoryTotal);
  return totalMiB !== undefined && totalMiB > 0 ? (memoryAllocUsedMiB.value / totalMiB) * 100 : undefined;
});
const computeUsagePercentRaw = computed(() => (
  computeUsageRatioRaw.value === undefined ? undefined : computeUsageRatioRaw.value * 100
));
const memoryUsagePercentRaw = computed(() =>
  migInstanceMemory.value ? migInstanceMemory.value.percent : wholeMemoryUsage.value?.percent,
);

const clampPercent = (v) => Math.max(0, Math.min(100, v));
const roundPercentForProgress = (p) => (p === undefined ? undefined : roundToDecimal(p, 2));

const computeAllocPercentProgress = computed(() => (computeAllocPercentRaw.value === undefined || computeAllocNothingCounted.value
  ? undefined
  : clampPercent(computeAllocPercentRaw.value)));
const computeUsagePercentProgress = computed(() => (computeUsagePercentRaw.value === undefined ? undefined : clampPercent(computeUsagePercentRaw.value)));
const memoryAllocPercentProgress = computed(() => (memoryAllocPercentRaw.value === undefined ? undefined : clampPercent(memoryAllocPercentRaw.value)));
const memoryUsagePercentProgress = computed(() => (memoryUsagePercentRaw.value === undefined ? undefined : clampPercent(memoryUsagePercentRaw.value)));

const computeAllocPercentProgressRounded = computed(() => roundPercentForProgress(computeAllocPercentProgress.value));
const computeUsagePercentProgressRounded = computed(() => roundPercentForProgress(computeUsagePercentProgress.value));
const memoryAllocPercentProgressRounded = computed(() => roundPercentForProgress(memoryAllocPercentProgress.value));
const memoryUsagePercentProgressRounded = computed(() => roundPercentForProgress(memoryUsagePercentProgress.value));

const computeAllocNote = computed(() => {
  if (computeAllocNothingCounted.value) return nothingCountedMessage(t, computeAllocUncounted.value);
  return isLowerBound(computeAllocUncounted.value) ? lowerBoundMessage(t, computeAllocUncounted.value) : '';
});
const computeAllocPercentText = computed(() => {
  const raw = computeAllocPercentRaw.value;
  if (raw === undefined || computeAllocUncounted.value === undefined || computeAllocNothingCounted.value) return '--';
  return `${isLowerBound(computeAllocUncounted.value) ? '≥' : ''}${roundToDecimal(raw, 2)}%`;
});
const computeUsagePercentText = computed(() => (computeUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(computeUsagePercentRaw.value, 2)}%`));
const memoryAllocPercentText = computed(() => (memoryAllocPercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryAllocPercentRaw.value, 2)}%`));
const memoryUsagePercentText = computed(() => (memoryUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryUsagePercentRaw.value, 2)}%`));
const allocationSlots = computed(() => getAllocationSlotDisplay(detail.value));
const allocatedSlotsText = computed(() => `${allocationSlots.value?.used ?? '--'}`);
const slotLimitText = computed(() => `${allocationSlots.value?.limit ?? '--'}`);

const lineTools = [
  {
    titleKey: 'card.detail.gpuTemperatureTrend',
    seriesNameKey: 'card.detail.gpuTemperature',
    sectionIndex: 'temperature',
    unit: '℃',
  },
  {
    titleKey: 'card.detail.gpuPowerTrend',
    seriesNameKey: 'card.detail.gpuPower',
    sectionIndex: 'power',
    unit: 'W',
  },
];
const trendLoading = computed(() => rangeSeries.value.some(
  (item) => item.refreshing || item.status === REQUEST_STATUS.LOADING,
));

const sensorReadings = computed(() => [
  {
    key: 'card.detail.gpuTemperature',
    loading: detailStatus.value === REQUEST_STATUS.LOADING,
    text: isDetailReady.value ? formatOptionalTelemetry(detail.value?.temperatureCelsius, '℃') : '--',
  },
  {
    key: 'card.detail.gpuPower',
    loading: detailStatus.value === REQUEST_STATUS.LOADING,
    text: isDetailReady.value ? formatOptionalTelemetry(detail.value?.powerWatts, 'W') : '--',
  },
]);

const lineToolsView = computed(() =>
  lineTools.map((item) => {
    const state = rangeSeries.value.find((entry) => entry.sectionIndex === item.sectionIndex);
    const status = state?.status || REQUEST_STATUS.LOADING;
    return {
      ...item,
      title: dt(item.titleKey),
      status,
      refreshing: state?.refreshing || false,
      refreshError: state?.refreshError || null,
      stateText: t(stateTextKey(status)),
      option: buildTimeSeriesOptions({
        series: [
          {
            name: t(item.seriesNameKey),
            data: state?.data,
            color: CHART_COLORS.single,
          },
        ],
        unit: item.unit,
        digits: 1,
      }),
    };
  }),
);

let nodeEnrichmentGeneration = 0;
watch(
  () => [
    detailStatus.value,
    detail.value?.uuid,
    detail.value?.nodeUid,
    detail.value?.node_uid,
    detail.value?.nodeName,
  ],
  async ([status, , directNodeUid, legacyNodeUid, nodeName], _, onCleanup) => {
    const generation = ++nodeEnrichmentGeneration;
    nodeUid.value = '';
    if (status !== REQUEST_STATUS.READY) return;

    const resolvedNodeUid = directNodeUid || legacyNodeUid || '';
    if (resolvedNodeUid) {
      nodeUid.value = resolvedNodeUid;
      return;
    }
    if (!nodeName) return;

    const controller = new AbortController();
    onCleanup(() => controller.abort());
    try {
      const { list = [] } = await apiClient.listNodes({}, { signal: controller.signal });
      if (controller.signal.aborted || generation !== nodeEnrichmentGeneration) return;
      const node = list.find((item) => item?.name === nodeName);
      nodeUid.value = node?.uid || '';
    } catch {
      if (!controller.signal.aborted && generation === nodeEnrichmentGeneration) {
        nodeUid.value = '';
      }
    }
  },
  { immediate: true },
);

</script>

<style scoped lang="scss">
.resource-overview-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}

.resource-overview-cards {
  margin: 0;
  padding: 0;
  list-style: none;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
  gap: 8px;
}

.resource-overview-card {
  display: flex;
  min-width: 0;
}

.resource-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  min-height: 0;
  width: 100%;
  padding: 15px 16px;
  background: #f5f7fa;
  border-radius: 8px;
  border: 0;
  overflow-x: hidden;
}

.resource-card-header {
  display: flex;
  align-items: center;
  gap: 20px;
  width: 100%;
}

.resource-card-icon {
  flex: 0 0 40px;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #ffffff;
  border-radius: 8px;
  box-shadow: 0 4px 10px rgba(2, 5, 8, 0.06);
  font-size: 20px;
}

.resource-card-header-info {
  min-width: 0;
}

.resource-card-value {
  font-size: 20px;
  font-weight: 500;
  color: #1d2b3a;
  line-height: 28px;
}

.resource-card-value--compute {
  font-size: 16px;
  line-height: 22px;
}

.resource-card-sub-title {
  margin-top: 4px;
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
}

.resource-slot-card {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  padding: 12px;
}

.resource-slot-gauge {
  position: relative;
  width: 168px;
  max-width: 100%;
  color: #939ea9;
  font-size: 12px;
  text-align: center;
}

.resource-slot-summary {
  position: absolute;
  bottom: 0;
  left: 0;
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.resource-slot-value {
  display: flex;
  align-items: baseline;
  gap: 4px;
  line-height: 26px;
  white-space: nowrap;

  b {
    font-size: 20px;
    font-weight: 500;
    color: #324558;
  }
}

.resource-slot-label {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  line-height: 20px;
}

@container device-resource-overview (min-width: 900px) {
  .resource-overview-layout.has-slot-summary {
    grid-template-columns: minmax(176px, 0.7fr) minmax(0, 2fr);
  }
}

.resource-card-footer {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  margin-top: 15px;
  width: 100%;
}

.resource-card-rate-wrap {
  width: 100%;
  background: #ffffff;
  border-radius: 6px;
  padding: 10px 12px;
  box-sizing: border-box;
  box-shadow: 0 4px 10px rgba(2, 5, 8, 0.06);
}

.resource-card-footer-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) max-content;
  align-items: center;
  gap: 4px 8px;
}

.resource-card-footer-title {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
  white-space: nowrap;
}

.resource-card-footer-value {
  display: flex;
  min-height: 24px;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-left: auto;
  white-space: nowrap;
}

.resource-card-footer-metric {
  font-size: 14px;
  font-weight: 500;
  color: #939ea9;
  white-space: nowrap;
}

.resource-card-footer-metric--allocated {
  color: #939ea9;
}

.resource-card-footer-reading {
  font-size: 14px;
  color: #324558;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #324558;
}

// Below the cards' minimum width, keep each label above its complete reading.
@container device-resource-overview (max-width: 319px) {
  .resource-card-footer-item {
    grid-template-columns: minmax(0, 1fr);
  }

  .resource-card-footer-value {
    max-width: 100%;
    white-space: normal;
  }
}

.line-box {
  container: metric-trends / inline-size;
  display: flex;
  flex-wrap: wrap;
  gap: 16px;

  > .home-block {
    flex: 1 1 calc(50% - 10px);
    min-width: 0;
    padding: 16px 20px;
    margin-bottom: 0;
  }
}

// Below 700px half a row cannot fit the longest legend on one line, so the trends stack.
@container metric-trends (max-width: 699px) {
  .line-box > .home-block {
    flex-basis: 100%;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  .home-block-content {
    flex: 1;
  }
}

.resource-overview-block {
  container: device-resource-overview / inline-size;

  :deep(.home-block-header) {
    flex-wrap: wrap;
    gap: 8px 16px;
  }

  :deep(.home-block-header > .extra) {
    margin-left: auto;
    max-width: 100%;
  }
}

.resource-card-sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  margin: -1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.card-trend-filter {
  margin-top: 24px;
}

.npu-spec-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0 0 16px;

  &:last-child {
    margin-bottom: 0;
  }
}

.npu-spec-fact {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 20px;
  min-width: 160px;
  padding: 15px 20px;
  border-radius: 8px;
  background: #f5f7fa;
}

.npu-spec-fact-icon {
  display: flex;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 4px 10px rgb(2 5 8 / 6%);
  font-size: 20px;
}

.npu-spec-fact-value {
  display: block;
  color: #324558;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}

.npu-spec-fact-label {
  display: block;
  color: #939ea9;
  font-size: 12px;
  line-height: 20px;
}

.npu-spec-subtitle {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  margin-bottom: 12px;
  color: #1d2b3a;
  font-size: 14px;
  font-weight: 500;
}

.npu-spec-options {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 8px;
}

.npu-spec-super-pod:not(:last-child) {
  margin-bottom: 12px;
}

.npu-spec-note {
  margin: 0;
  color: #5f6b7a;
  font-size: 14px;
  line-height: 22px;
}

</style>
