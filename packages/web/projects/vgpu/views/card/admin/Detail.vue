<template>
  <div>
    <page-header
      :title="dt('card.detail.title')"
      :name="headerName"
      :status="headerStatusDisplay.text"
      :status-icon="headerStatusDisplay.icon"
    />
    <detail-page-state :status="detailStatus" @retry="retryDetail">
    <block-box class="node-block">
      <div class="card-detail">
        <div class="card-detail-left">
          <div class="title">{{ $t('card.detail.detailInfo') }}</div>
          <div class="basic-info-row">
            <div class="basic-info-card">
              <RouterLink
                v-if="nodeDetailLocation"
                :to="nodeDetailLocation"
                class="basic-info-title basic-info-node-link"
              >
                <span class="text">{{ detail.nodeName || '--' }}</span>
                <span class="basic-info-share">
                  <svg-icon icon="jump" />
                </span>
              </RouterLink>
              <div v-else class="basic-info-title">
                <span class="text">{{ detail.nodeName || '--' }}</span>
              </div>
              <div class="basic-info-subtitle">{{ $t('card.node') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                <svg-icon v-if="gpuTypeIcon && detail.type" :icon="gpuTypeIcon" class="gpu-type-icon" />
                {{ detail.type || '--' }}
                <UnconfiguredTag v-if="detail.unconfigured" />
              </div>
              <div class="basic-info-subtitle">{{ dt('card.model') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                <svg-icon v-if="splitModeIcon" :icon="splitModeIcon" class="split-mode-icon" aria-hidden="true" />
                {{ splitModeText }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.splitMode.label') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                {{ basicTemperatureText }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.detail.gpuTemperature') }}</div>
            </div>
            <div class="basic-info-card">
              <div class="basic-info-title">
                {{ basicPowerText }}
              </div>
              <div class="basic-info-subtitle">{{ $t('card.detail.gpuPower') }}</div>
            </div>
          </div>
        </div>
      </div>
    </block-box>

    <block-box class="resource-overview-block" :title="$t('card.detail.resourceOverview')">
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
                        {{ $t('dashboard.used') }}
                      </span>
                      <metric-help
                        :description="$t('dashboard.memUsageRateDescription')"
                        :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memUsageRate') })"
                      />
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-reading">
                        <span class="resource-card-footer-metric">{{ memoryUsageUsedText }}</span>
                        (<span class="resource-card-footer-percent">{{ memoryUsagePercentText }}</span>)
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

    <block-box v-if="splitVisible" class="device-split-block" :title="$t('card.split.title')">
      <template v-if="detail.mode === 'mig'" #extra>
        <MetricHelp multiline :description="$t('card.split.help')" :help-label="$t('card.split.title')" />
      </template>
      <DeviceSplit :device="detail" :containers="cardContainers" :status="splitStatus" :show-shared-count="false" @retry="loadSplit" />
    </block-box>

    <block-box v-if="npuSpecVisible" class="npu-spec-block" :title="$t('card.deviceConfig.title')">
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

    <TrendTimeFilter v-model="times" :loading="trendLoading" class="card-trend-filter" />
    <div class="line-box">
      <block-box
        v-for="section in trendSections"
        :key="section.key"
        :title="section.title"
      >
        <MetricChart
          :status="section.status"
          :option="section.option"
          :note="section.note"
          :state-text="section.stateText"
          :refreshing="section.refreshing"
          :refresh-error="Boolean(section.refreshError)"
        />
      </block-box>

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
import { RouterLink, useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import DetailPageState from '~/vgpu/components/DetailPageState.vue';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import { ref, watch, computed } from 'vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import useRangeVector from '~/vgpu/hooks/useRangeVector';
import { readReadyMetricField } from '~/vgpu/hooks/instant-vector-state.mjs';
import {
  isLowerBound,
  isNothingCounted,
  lowerBoundMessage,
  nothingCountedMessage,
  readTrendUncounted,
  readUncountedMetric,
} from '~/vgpu/metrics/uncounted.mjs';
import useDetailResource from '~/vgpu/hooks/useDetailResource.js';
import { classifyDetailPayload } from '~/vgpu/hooks/detail-resource-state.mjs';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import cardApi from '~/vgpu/api/card';
import WorkloadSemiProgress from './components/WorkloadSemiProgress.vue';
import nodeApi from '~/vgpu/api/node';
import { roundToDecimal, getResourceColor } from '@/utils';
import MetricChart from '~/vgpu/components/MetricChart.vue';
import { buildTimeSeriesOptions } from '~/vgpu/metrics/chart-presets.mjs';
import { CHART_COLORS } from '~/vgpu/metrics/chart-colors.mjs';
import { stateTextKey, summarizeRangeSeries } from '~/vgpu/metrics/metric-state.mjs';
import { useI18n } from 'vue-i18n';
import {
  buildComputeAllocationQueries,
  buildUnknownComputeShareQuery,
  buildMemoryAllocationQueries,
  buildMemoryUsageQueries,
} from '~/vgpu/metrics/query-contract.mjs';
import { renderPromQLTemplate } from '~/vgpu/metrics/promql-template.mjs';
import { buildNodeDetailLocation } from '~/vgpu/views/node/detail-location.mjs';
import { formatOptionalTelemetry } from './optional-telemetry-display.mjs';
import { getAllocationSlotDisplay } from './allocation-slot-display.mjs';
import UnconfiguredTag from './components/UnconfiguredTag.vue';
import deviceConfigApi from '~/vgpu/api/deviceConfig';
import { deviceWording, isNpuVendor } from '~/vgpu/components/device-copy.mjs';
import { getSplitIcon, getSplitModeKey } from '~/vgpu/components/split-mode.mjs';
import { buildAllocationOptions, findAscendModel, getDeviceConfigStateKey } from './device-config-display.mjs';
import NpuAllocationOption from './components/NpuAllocationOption.vue';
import DeviceSplit from '~/vgpu/components/DeviceSplit.vue';
import taskApi from '~/vgpu/api/task';

const route = useRoute();
const { t } = useI18n();

const routeCardUuid = computed(() => route.params.uuid);
const {
  data: detail,
  status: detailStatus,
  retry: retryDetail,
} = useDetailResource({
  source: routeCardUuid,
  request: (uuid) => cardApi.getCardDetail({ uid: uuid }),
  classify: (payload, uuid) =>
    classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid },
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
// Plugins that register no split mode leave nothing to lay out.
const splitVisible = computed(() => Boolean(isDetailReady.value && getSplitModeKey(detail.value?.mode)));
const cardContainers = ref([]);
const splitStatus = ref('loading');
let splitGeneration = 0;
const loadSplit = async () => {
  const generation = ++splitGeneration;
  const uuid = detailCardUuid.value;
  splitStatus.value = 'loading';
  if (!uuid || !splitVisible.value) return;
  try {
    const result = await taskApi.getWorkloads({ filters: { deviceId: uuid }, page: 1, pageSize: 100 });
    if (generation !== splitGeneration) return;
    cardContainers.value = Array.isArray(result?.items) ? result.items : [];
    splitStatus.value = 'ready';
  } catch {
    if (generation === splitGeneration) splitStatus.value = 'error';
  }
};
watch(detailCardUuid, loadSplit, { immediate: true });
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
const CARD_TYPE_ICON_MAP = {
  NVIDIA: 'gpu-nvidia',
  MXC: 'gpu-nvidia',
  ASCEND: 'gpu-ascend',
  METAX: 'gpu-metax',
  AWS: 'gpu-aws',
  AWSNEURON: 'gpu-aws',
};

const extractLetters = (value) => {
  if (typeof value === 'string') {
    return value.match(/[a-z]+/gi) || [];
  }
  return [];
};

const gpuTypeIcon = computed(() => {
  const vendor = extractLetters(detail.value?.type)?.[0]?.toUpperCase();
  if (!vendor) return '';
  return CARD_TYPE_ICON_MAP[vendor] || 'vgpu-card';
});
const getCardStatusDisplay = ({ health, isExternal }) => {
  if (isExternal || health === undefined || health === null) {
    return { icon: 'status-unmanaged', text: t('card.unknown') };
  }
  if (health) {
    return { icon: 'status-schedulable', text: t('card.normal') };
  }
  return { icon: 'status-unschedulable', text: t('card.abnormal') };
};
const headerStatusDisplay = computed(() =>
  isDetailReady.value
    ? getCardStatusDisplay(detail.value || {})
    : { icon: '', text: '' },
);

const deviceConfig = ref({ state: 'loading' });
deviceConfigApi.getDeviceConfig()
  .then((reply) => { deviceConfig.value = reply || { state: 'error' }; })
  .catch(() => { deviceConfig.value = { state: 'error' }; });
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
const cardMetricSelector = 'device_uuid=$device_uuid';
const computeAllocationQueries = buildComputeAllocationQueries({
  selector: cardMetricSelector,
});
const memoryAllocationQueries = buildMemoryAllocationQueries({
  selector: cardMetricSelector,
});
const memoryUsageQueries = buildMemoryUsageQueries({
  selector: cardMetricSelector,
});

const basicPowerText = computed(() =>
  formatOptionalTelemetry(readReadyMetricField(telemetryNow.value[1], 'used'), 'W'));
const basicTemperatureText = computed(() =>
  formatOptionalTelemetry(readReadyMetricField(telemetryNow.value[0], 'used'), '℃'));

const _gaugeConfigBase = [
  {
    titleKey: 'dashboard.computeAllocRate',
    percent: 0,
    query: computeAllocationQueries.query,
    totalQuery: computeAllocationQueries.totalQuery,
    percentQuery: computeAllocationQueries.percentQuery,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memAllocRate',
    percent: 0,
    query: memoryAllocationQueries.query,
    totalQuery: memoryAllocationQueries.totalQuery,
    percentQuery: memoryAllocationQueries.percentQuery,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    titleKey: 'dashboard.computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_core_util{device_uuid=$device_uuid}) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg{device_uuid=$device_uuid}) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    titleKey: 'dashboard.memUsageRate',
    percent: 0,
    query: memoryUsageQueries.query,
    totalQuery: memoryUsageQueries.totalQuery,
    percentQuery: memoryUsageQueries.percentQuery,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
];

const renderCardQuery = (query) => renderPromQLTemplate(query, {
  device_uuid: detailCardUuid.value,
});

// Current values only: each trend chart reads its lines from one range group below.
const gaugeData = useInstantVector(
  _gaugeConfigBase.map(({ percentQuery, ...item }) => ({ ...item, title: t(item.titleKey) })),
  renderCardQuery,
  times,
);

// The card's allocation rate leaves out allocations whose share HAMi does not
// state, so it reads as a lower bound unless none are confirmed.
const uncountedQuery = buildUnknownComputeShareQuery({ selector: cardMetricSelector });
const uncountedMetric = useInstantVector([{ query: uncountedQuery }], renderCardQuery, times);
const computeAllocUncounted = computed(() => readUncountedMetric(uncountedMetric.value[0]));
// A chart's lines, and the count that qualifies one, share a group: they settle
// for one range together, or the chart keeps its last range and says so.
const trendQuery = (index) => _gaugeConfigBase[index].percentQuery;
const { data: computeTrend } = useRangeVector(
  [{ query: trendQuery(0) }, { query: trendQuery(2) }, { query: uncountedQuery, optional: true }],
  renderCardQuery,
  times,
);
const { data: memoryTrend } = useRangeVector(
  [{ query: trendQuery(1) }, { query: trendQuery(3) }],
  renderCardQuery,
  times,
);
const computeAllocLegend = computed(() => t(isLowerBound(readTrendUncounted(computeTrend.value[2]))
  ? 'dashboard.allocRateLowerBoundLegend'
  : 'dashboard.allocRateLegend'));

const gaugeConfig = computed(() =>
  gaugeData.value.map((item) => ({
    ...item,
    title: item.titleKey ? t(item.titleKey) : item.title,
  })),
);

const readGaugeField = (index, field) =>
  readReadyMetricField(gaugeConfig.value?.[index], field);

const computeTotalText = computed(() => {
  const total = readGaugeField(0, 'total');
  return total === undefined ? '--' : `${roundToDecimal(total / 100, 1)}`;
});

const memoryTotalText = computed(() => {
  const total = readGaugeField(1, 'total');
  return total === undefined ? '--' : `${roundToDecimal(total, 1)} GiB`;
});

const formatUsedValue = (v, unit, divisor = 1) => {
  const n = Number(v);
  if (!Number.isFinite(n)) return '--';
  const normalizedDivisor = Number(divisor) > 0 ? Number(divisor) : 1;
  const text = `${roundToDecimal(n / normalizedDivisor, 1)}`;
  return unit && String(unit).trim() ? `${text} ${unit}` : text;
};

const computeAllocUsedText = computed(() =>
  detail.value?.isExternal || computeAllocNothingCounted.value
    ? '--'
    : formatUsedValue(
        readGaugeField(0, 'used'),
        gaugeConfig.value?.[0]?.unit,
        100,
      ),
);
const computeUsageUsedText = computed(() =>
  formatUsedValue(
    readGaugeField(2, 'used'),
    gaugeConfig.value?.[2]?.unit,
    100,
  ),
);
const memoryAllocUsedText = computed(() =>
  detail.value?.isExternal
    ? '--'
    : formatUsedValue(
        readGaugeField(1, 'used'),
        gaugeConfig.value?.[1]?.unit,
      ),
);
const memoryUsageUsedText = computed(() =>
  formatUsedValue(readGaugeField(3, 'used'), gaugeConfig.value?.[3]?.unit),
);

const computeAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  return readGaugeField(0, 'percent');
});
const memoryAllocPercentRaw = computed(() => {
  if (detail.value?.isExternal) return undefined;
  return readGaugeField(1, 'percent');
});
const computeUsagePercentRaw = computed(() => readGaugeField(2, 'percent'));
const memoryUsagePercentRaw = computed(() => readGaugeField(3, 'percent'));

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

const computeAllocNothingCounted = computed(() => isNothingCounted(readGaugeField(0, 'used'), computeAllocUncounted.value));
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
    query: `avg by (device_no,driver_version) (hami_device_temperature{device_uuid=$device_uuid})`,
    unit: '℃',
  },
  {
    titleKey: 'card.detail.gpuPowerTrend',
    seriesNameKey: 'card.detail.gpuPower',
    query: `avg by (device_no,driver_version) (hami_device_power{device_uuid=$device_uuid})`,
    unit: 'W',
  },
];
// Each line is its own range group: it keeps its last result while a new range loads.
const { data: telemetryTrend } = useRangeVector(
  lineTools.map(({ query }, sectionIndex) => ({ query, sectionIndex })),
  renderCardQuery,
  times,
);
const trendLoading = computed(() => [...computeTrend.value, ...memoryTrend.value, ...telemetryTrend.value]
  .some((item) => item.refreshing || item.status === REQUEST_STATUS.LOADING));
// The current readings do not depend on the chart range.
const telemetryNow = useInstantVector(lineTools.map(({ query }) => ({ query })), renderCardQuery);

const lineToolsView = computed(() =>
  lineTools.map((item, index) => {
    const state = telemetryTrend.value[index];
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

const trendSection = (key, title, [allocation, usage], allocationName) => {
  const series = [
    { ...allocation, name: allocationName, color: CHART_COLORS.allocation },
    { ...usage, name: t('dashboard.usageRateLegend'), color: CHART_COLORS.usage },
  ];
  const summary = summarizeRangeSeries(series, t);
  return {
    key,
    title,
    ...summary,
    stateText: t(stateTextKey(summary.status)),
    option: buildTimeSeriesOptions({ series }),
  };
};
const trendSections = computed(() => [
  trendSection('compute', dt('dashboard.gpuComputeAllocUsageTrend'), computeTrend.value, computeAllocLegend.value),
  trendSection('memory', dt('dashboard.gpuMemAllocUsageTrend'), memoryTrend.value, t('dashboard.allocRateLegend')),
]);

let nodeEnrichmentGeneration = 0;
watch(
  () => [
    detailStatus.value,
    detail.value?.uuid,
    detail.value?.nodeUid,
    detail.value?.node_uid,
    detail.value?.nodeName,
  ],
  async ([status, , directNodeUid, legacyNodeUid, nodeName]) => {
    const generation = ++nodeEnrichmentGeneration;
    nodeUid.value = '';
    if (status !== REQUEST_STATUS.READY) return;

    const resolvedNodeUid = directNodeUid || legacyNodeUid || '';
    if (resolvedNodeUid) {
      nodeUid.value = resolvedNodeUid;
      return;
    }
    if (!nodeName) return;

    try {
      const { list = [] } = await nodeApi.getNodes({ filters: {} });
      if (generation !== nodeEnrichmentGeneration) return;
      const node = list.find((item) => item?.name === nodeName);
      nodeUid.value = node?.uid || '';
    } catch {
      if (generation === nodeEnrichmentGeneration) {
        nodeUid.value = '';
      }
    }
  },
  { immediate: true },
);

</script>

<style scoped lang="scss">
.card-detail {
  display: flex;
  width: 100%;
  height: 100%;

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .title {
    color: #1d2b3a;
    font-family: 'PingFang SC';
    font-size: 16px;
    font-style: normal;
    font-weight: 500;
    margin-bottom: 20px;
  }
  .card-detail-left {
    width: 100%;
  }

  .basic-info-row {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }

  .basic-info-card {
    display: flex;
    flex: 1;
    flex-direction: column;
    justify-content: center;
    min-width: 0;
    gap: 2px;
    padding: 15px 20px;
    border-radius: 8px;
    background: #f5f7fa;
    border: 0;
    overflow-x: hidden;
  }

  .basic-info-title {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    font-size: 16px;
    font-weight: 500;
    color: #324558;
    line-height: 28px;
    cursor: default;

    .text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .basic-info-share {
    display: inline-flex;
    align-items: center;
  }

  .basic-info-node-link {
    align-self: flex-start;
    max-width: 100%;
    border-radius: 3px;
    cursor: pointer;
    text-decoration: none;
    transition: color 0.2s ease;

    .text {
      text-decoration: underline;
      text-decoration-color: transparent;
      text-underline-offset: 3px;
      transition: text-decoration-color 0.2s ease;
    }

    .basic-info-share {
      transition: color 0.2s ease;
    }

    &:hover,
    &:focus-visible {
      color: var(--el-color-primary);

      .text {
        text-decoration-color: currentColor;
      }
    }

    &:focus-visible {
      outline: 2px solid var(--el-color-primary);
      outline-offset: 2px;
    }
  }

  .basic-info-subtitle {
    color: #939ea9;
    font-size: 12px;
    line-height: 20px;
  }

.split-mode-icon {
  width: 18px;
  height: 18px;
}

.gpu-type-icon {
  width: 16px;
  height: 16px;
  color: #64748b;
}
}

.resource-overview-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  margin-top: 12px;
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
  box-shadow: none;
  .home-block-content {
    flex: 1;
  }
}

.resource-overview-block {
  container: device-resource-overview / inline-size;
  margin-bottom: 16px;
  box-shadow: none;

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

.device-split-block {
  margin-bottom: 16px;
  box-shadow: none;

  :deep(.home-block-content) {
    padding-top: 12px;
  }
}

.npu-spec-block {
  margin-bottom: 16px;
  box-shadow: none;
}

.card-trend-filter {
  margin-top: 24px;
}

.npu-spec-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0 16px;

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
