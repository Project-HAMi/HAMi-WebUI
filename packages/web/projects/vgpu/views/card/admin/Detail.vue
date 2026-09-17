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
    </block-box>

    <block-box class="resource-overview-block" :title="$t('card.detail.resourceOverview')">
      <ul class="resource-overview-cards">
        <li class="resource-overview-card">
          <div class="progress-wrapper">
            <workload-semi-progress :percent="workloadCountPercentProgress" />
            <div class="workload-progress-summary">
              <div class="workload-progress-value">
                <b>{{ workloadCountUsedText }}</b> / {{ workloadCountTotalText }}
              </div>
              <div class="workload-progress-subtitle">
                <span>{{ $t('card.detail.workloadCount') }}</span>
                <t-tooltip :content="$t('card.detail.workloadCountTip')">
                  <help-circle-icon class="resource-card-help-icon" />
                </t-tooltip>
              </div>
            </div>
          </div>
        </li>
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
                      {{ $t('dashboard.allocated') }} / {{ $t('dashboard.allocRateLegend') }}
                    </span>
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ computeAllocUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <t-tooltip
                      v-if="computeAllocUncounted"
                      :content="$t('dashboard.metricLowerBound', { count: computeAllocUncounted })"
                    >
                      <span class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>
                    </t-tooltip>
                    <span v-else class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>
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
                      {{ $t('dashboard.used') }} / {{ $t('dashboard.usageRateLegend') }}
                    </span>
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric">{{ computeUsageUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ computeUsagePercentText }}</span>
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
                      {{ $t('dashboard.allocated') }} / {{ $t('dashboard.allocRateLegend') }}
                    </span>
                    <metric-help
                      :description="$t('dashboard.memAllocRateDescription')"
                      :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memAllocRate') })"
                    />
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric resource-card-footer-metric--allocated">{{ memoryAllocUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ memoryAllocPercentText }}</span>
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
                      {{ $t('dashboard.used') }} / {{ $t('dashboard.usageRateLegend') }}
                    </span>
                    <metric-help
                      :description="$t('dashboard.memUsageRateDescription')"
                      :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memUsageRate') })"
                    />
                  </div>
                  <div class="resource-card-footer-value">
                    <span class="resource-card-footer-metric">{{ memoryUsageUsedText }}</span>
                    <span class="resource-card-footer-sep">/</span>
                    <span class="resource-card-footer-percent">{{ memoryUsagePercentText }}</span>
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
    </block-box>

    <TrendTimeFilter v-model="times" />
    <div class="line-box">
      <block-box
        v-for="section in trendSections"
        :key="section.title"
        :title="section.title"
      >
        <MetricChart
          :status="section.status"
          :option="section.option"
          :state-text="section.stateText"
        />
      </block-box>

      <block-box
        v-for="item in lineToolsView"
        :key="item.title"
        :title="item.title"
      >
        <MetricChart
          :status="item.status"
          :option="item.option"
          :state-text="item.stateText"
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
import { HelpCircleIcon } from 'tdesign-icons-vue-next';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import { readReadyMetricField } from '~/vgpu/hooks/instant-vector-state.mjs';
import useDetailResource from '~/vgpu/hooks/useDetailResource.js';
import { classifyDetailPayload } from '~/vgpu/hooks/detail-resource-state.mjs';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import cardApi from '~/vgpu/api/card';
import nodeApi from '~/vgpu/api/node';
import WorkloadSemiProgress from './components/WorkloadSemiProgress.vue';
import { timeParse, calculatePrometheusStep, roundToDecimal, getResourceColor } from '@/utils';
import MetricChart from '~/vgpu/components/MetricChart.vue';
import { buildTimeSeriesOptions } from '~/vgpu/metrics/chart-presets.mjs';
import { CHART_COLORS } from '~/vgpu/metrics/chart-colors.mjs';
import { aggregateStatuses, stateTextKey } from '~/vgpu/metrics/metric-state.mjs';
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
import UnconfiguredTag from './components/UnconfiguredTag.vue';
import deviceConfigApi from '~/vgpu/api/deviceConfig';
import { deviceWording, isNpuVendor } from '~/vgpu/components/device-copy.mjs';
import { buildAllocationOptions, findAscendModel, getDeviceConfigStateKey } from './device-config-display.mjs';
import NpuAllocationOption from './components/NpuAllocationOption.vue';

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
const detailCardUuid = computed(() =>
  isDetailReady.value ? detail.value.uuid : undefined,
);
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

const basicPowerText = computed(() => {
  const v = lineTools.value[1]?.percent;
  const unit = lineTools.value[1]?.gaugeUnit || 'W';
  return formatOptionalTelemetry(v, unit);
});
const basicTemperatureText = computed(() => {
  const v = lineTools.value[0]?.percent;
  const unit = lineTools.value[0]?.gaugeUnit || '℃';
  return formatOptionalTelemetry(v, unit);
});

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

const gaugeData = useInstantVector(
  _gaugeConfigBase.map(item => ({ ...item, title: t(item.titleKey) })),
  renderCardQuery,
  times,
);

// The card's allocation rate leaves out allocations whose share HAMi does not
// state, so it reads as a lower bound while any exist.
const uncountedMetric = useInstantVector(
  [{ query: buildUnknownComputeShareQuery({ selector: cardMetricSelector }) }],
  renderCardQuery,
  times,
);
const computeAllocUncounted = computed(() => {
  const count = Number(readReadyMetricField(uncountedMetric.value[0], 'count'));
  return Number.isFinite(count) && count > 0 ? count : 0;
});

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
  detail.value?.isExternal
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

const computeAllocPercentProgress = computed(() => (computeAllocPercentRaw.value === undefined ? undefined : clampPercent(computeAllocPercentRaw.value)));
const computeUsagePercentProgress = computed(() => (computeUsagePercentRaw.value === undefined ? undefined : clampPercent(computeUsagePercentRaw.value)));
const memoryAllocPercentProgress = computed(() => (memoryAllocPercentRaw.value === undefined ? undefined : clampPercent(memoryAllocPercentRaw.value)));
const memoryUsagePercentProgress = computed(() => (memoryUsagePercentRaw.value === undefined ? undefined : clampPercent(memoryUsagePercentRaw.value)));

const computeAllocPercentProgressRounded = computed(() => roundPercentForProgress(computeAllocPercentProgress.value));
const computeUsagePercentProgressRounded = computed(() => roundPercentForProgress(computeUsagePercentProgress.value));
const memoryAllocPercentProgressRounded = computed(() => roundPercentForProgress(memoryAllocPercentProgress.value));
const memoryUsagePercentProgressRounded = computed(() => roundPercentForProgress(memoryUsagePercentProgress.value));

const computeAllocPercentText = computed(() => (computeAllocPercentRaw.value === undefined
  ? '--'
  : `${computeAllocUncounted.value ? '≥' : ''}${roundToDecimal(computeAllocPercentRaw.value, 2)}%`));
const computeUsagePercentText = computed(() => (computeUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(computeUsagePercentRaw.value, 2)}%`));
const memoryAllocPercentText = computed(() => (memoryAllocPercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryAllocPercentRaw.value, 2)}%`));
const memoryUsagePercentText = computed(() => (memoryUsagePercentRaw.value === undefined ? '--' : `${roundToDecimal(memoryUsagePercentRaw.value, 2)}%`));
const workloadCountUsed = computed(() => Number(detail.value?.vgpuUsed));
const workloadCountTotal = computed(() => Number(detail.value?.vgpuTotal));
const workloadCountUsedText = computed(() => (Number.isFinite(workloadCountUsed.value) ? `${workloadCountUsed.value}` : '--'));
const workloadCountTotalText = computed(() => (Number.isFinite(workloadCountTotal.value) ? `${workloadCountTotal.value}` : '--'));
const workloadCountPercentRaw = computed(() => {
  if (!Number.isFinite(workloadCountUsed.value) || !Number.isFinite(workloadCountTotal.value) || workloadCountTotal.value <= 0) {
    return 0;
  }
  return (workloadCountUsed.value / workloadCountTotal.value) * 100;
});
const workloadCountPercentProgress = computed(() => clampPercent(workloadCountPercentRaw.value));

const lineTools = ref([
  {
    titleKey: 'card.detail.gpuTemperatureTrend',
    seriesNameKey: 'card.detail.gpuTemperature',
    query: `avg by (device_no,driver_version) (hami_device_temperature{device_uuid=$device_uuid})`,
    data: [],
    unit: '℃',
    gaugeUnit: '℃',
    percent: undefined,
    total: 0,
    hideInfo: true,
    showProgress: false,
  },
  {
    titleKey: 'card.detail.gpuPowerTrend',
    seriesNameKey: 'card.detail.gpuPower',
    query: `avg by (device_no,driver_version) (hami_device_power{device_uuid=$device_uuid})`,
    data: [],
    unit: 'W',
    gaugeUnit: 'W',
    percent: undefined,
    total: 0,
    hideInfo: true,
    showProgress: false,
  },
]);

const lineLoading = ref(true);

const lineToolsView = computed(() =>
  lineTools.value.map((item) => {
    const status = lineLoading.value
      ? REQUEST_STATUS.LOADING
      : (item.data?.length ? REQUEST_STATUS.READY : REQUEST_STATUS.MISSING);
    return {
      ...item,
      title: dt(item.titleKey),
      status,
      stateText: t(stateTextKey(status)),
      option: buildTimeSeriesOptions({
        series: [
          {
            name: t(item.seriesNameKey),
            data: item.data,
            color: CHART_COLORS.single,
          },
        ],
        unit: item.unit,
      }),
    };
  }),
);

// Allocation and usage of one resource share a chart; both come from the
// gauges already queried for this card.
const trendSections = computed(() => [
  { title: dt('dashboard.gpuComputeAllocUsageTrend'), allocation: 0, usage: 2 },
  { title: dt('dashboard.gpuMemAllocUsageTrend'), allocation: 1, usage: 3 },
].map(({ title, allocation, usage }) => {
  const metrics = [gaugeConfig.value?.[allocation], gaugeConfig.value?.[usage]];
  const status = aggregateStatuses(metrics);
  return {
    title,
    status,
    stateText: t(stateTextKey(status)),
    option: buildTimeSeriesOptions({
      series: [
        {
          name: t('dashboard.allocRateLegend'),
          data: metrics[0]?.data,
          color: CHART_COLORS.allocation,
        },
        {
          name: t('dashboard.usageRateLegend'),
          data: metrics[1]?.data,
          color: CHART_COLORS.usage,
        },
      ],
    }),
  };
}));

let lineRequestGeneration = 0;
const resetLineData = () => {
  lineTools.value.forEach((item) => {
    item.data = [];
    item.percent = undefined;
  });
};

const fetchLineData = async () => {
  const generation = ++lineRequestGeneration;
  const uuid = detailCardUuid.value;
  if (!uuid) {
    resetLineData();
    lineLoading.value = false;
    return;
  }
  lineLoading.value = true;

  const requests = lineTools.value.flatMap((item, index) => {
    const query = renderPromQLTemplate(item.query, { device_uuid: uuid });
    const rangeRequest = cardApi
      .getRangeVector({
        range: {
          start: timeParse(times.value[0]),
          end: timeParse(times.value[1]),
          step: calculatePrometheusStep(times.value[0], times.value[1]),
        },
        query,
      })
      .then((res) => {
        if (generation !== lineRequestGeneration) return;
        lineTools.value[index].data = res.data?.[0]?.values || [];
      })
      .catch(() => {
        if (generation !== lineRequestGeneration) return;
        lineTools.value[index].data = [];
      });

    const instantRequest = cardApi
      .getInstantVector({ query })
      .then((res) => {
        if (generation !== lineRequestGeneration) return;
        lineTools.value[index].percent = res.data?.[0]?.value;
      })
      .catch(() => {
        if (generation !== lineRequestGeneration) return;
        lineTools.value[index].percent = undefined;
      });

    return [rangeRequest, instantRequest];
  });

  await Promise.all(requests);
  if (generation === lineRequestGeneration) lineLoading.value = false;
};

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

watch([times, detailCardUuid], fetchLineData, { immediate: true });
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

.gpu-type-icon {
  width: 16px;
  height: 16px;
  color: #64748b;
}
}

.resource-overview-cards {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  display: flex;
  gap: 8px;
}

.resource-overview-card {
  flex: 1;
  display: flex;
}

.progress-wrapper {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
}

.resource-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  min-height: 0;
  width: 100%;
  padding: 15px 20px;
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

.workload-progress-summary {
  margin-top: -76px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  color: #939ea9;
}

.workload-progress-value {
  line-height: 28px;

  b {
    font-size: 20px;
    color: #324558;
  }
}

.workload-progress-subtitle {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.resource-card-help-icon {
  color: #939ea9;
  font-size: 14px;
  cursor: pointer;
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
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.resource-card-footer-title {
  display: inline-flex;
  flex: 1 1 160px;
  align-items: center;
  gap: 4px;
  min-width: 0;
  font-size: 12px;
  color: #939ea9;
  line-height: 20px;
}

.resource-card-footer-value {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  margin-left: auto;
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

.resource-card-footer-sep {
  font-size: 12px;
  color: #b6c2cd;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #324558;
}

.line-box {
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

.node-block {
  display: flex;
  flex-direction: column;
  box-shadow: none;
  .home-block-content {
    flex: 1;
  }
}

.resource-overview-block {
  margin-bottom: 24px;
  box-shadow: none;
}

.npu-spec-block {
  margin-bottom: 16px;
  box-shadow: none;
}

.npu-spec-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 12px 0 16px;
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
  align-items: baseline;
  gap: 4px 12px;
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

.npu-spec-super-pod {
  margin-bottom: 12px;
}

.npu-spec-note {
  margin: 0;
  color: #5f6b7a;
  font-size: 14px;
  line-height: 22px;
}


</style>
