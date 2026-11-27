<template>
  <div>
    <page-header
      :title="$t('node.detail.title')"
      :name="headerName"
      :status="headerStatusDisplay.text"
      :status-icon="headerStatusDisplay.icon"
    >
      <template #titleSuffix>
        <metric-help
          v-if="headerStatusDisplay.showHelp"
          :description="headerStatusDisplay.description"
          :help-label="$t('node.schedulingStatusHelpLabel')"
        />
      </template>
    </page-header>

    <detail-page-state :status="detailStatus" @retry="retryDetail">
      <template #loading>
        <section class="node-overview-wrap">
          <block-box class="node-overview-panel" :title="$t('node.detail.nodeOverview')" variant="detail">
            <div class="node-detail" :class="{ 'is-en': locale.startsWith('en') }">
              <div class="node-detail-left">
                <div class="node-detail-info-summary node-detail-info-summary-cols">
                  <div v-for="(group, index) in detailColumnGroups" :key="index" class="summary-col">
                    <div v-for="{ label, value } in group" :key="label" class="summary-item">
                      <div class="summary-item-label">{{ label }}</div>
                      <div class="summary-item-value">
                        <span v-if="value === 'uid'" class="ellipsis-text is-singleline" style="width: 78%"><LoadingValue /></span>
                        <LoadingValue v-else width="78%" />
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </block-box>
          <block-box class="node-overview-panel node-workload-panel" :title="$t('node.detail.resourceOverview')" variant="detail">
            <ul class="node-system-resource-cards">
              <li v-for="(title, index) in [$t('dashboard.computePowerTotal'), $t('dashboard.memoryTotal')]" :key="title" class="resource-overview-card">
                <div class="resource-card">
                  <div class="resource-card-header">
                    <div class="resource-card-icon"><LoadingValue :height="24" width="24px" /></div>
                    <div class="resource-card-header-info">
                      <div class="resource-card-value" :class="{ 'resource-card-value--compute': index === 0 }"><LoadingValue :height="index === 0 ? 22 : 28" width="80px" /></div>
                      <div class="resource-card-sub-title">{{ title }}</div>
                    </div>
                  </div>
                  <div class="resource-card-footer">
                    <div v-for="label in [$t('dashboard.allocRateLegend'), $t(index === 0 ? 'dashboard.utilizationRateLegend' : 'dashboard.overviewMemoryOccupancyLegend')]" :key="label" class="resource-card-rate-wrap">
                      <div class="resource-card-footer-item">
                        <div class="resource-card-footer-title">{{ label }}</div>
                        <div class="resource-card-footer-value"><LoadingValue :height="24" width="64px" /></div>
                      </div>
                    </div>
                  </div>
                </div>
              </li>
            </ul>
          </block-box>
        </section>
        <TrendTimeFilter :model-value="times" class="node-trend-filter" loading />
        <div class="line-box">
          <ResourceTrend kind="compute" />
          <ResourceTrend kind="memory" />
        </div>
      </template>
    <section class="node-overview-wrap">
      <block-box class="node-overview-panel" :title="$t('node.detail.nodeOverview')" variant="detail">
        <div class="node-detail" :class="{ 'is-en': locale.startsWith('en') }">
          <div class="node-detail-left">
            <div class="node-detail-info-summary node-detail-info-summary-cols">
              <div
                v-for="(group, groupIndex) in detailColumnGroups"
                :key="groupIndex"
                class="summary-col"
              >
                <div
                  v-for="{ label, value, render } in group"
                  :key="label"
                  class="summary-item"
                >
                  <div class="summary-item-label">{{ label }}</div>
                  <div v-if="render" class="summary-item-value">
                    <component :is="render(detail)" />
                  </div>
                  <div v-else class="summary-item-value">{{ detail[value] }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </block-box>

      <block-box class="node-overview-panel node-workload-panel" :title="$t('node.detail.resourceOverview')" variant="detail">
        <ul class="node-system-resource-cards">
          <li class="resource-overview-card">
            <div class="resource-card">
              <div class="resource-card-header">
                <div class="resource-card-icon">
                  <svg-icon icon="vgpu-core" />
                </div>
                <div class="resource-card-header-info">
                  <div class="resource-card-value resource-card-value--compute">
                    {{ computePowerTotalText }}
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
                      <span class="resource-card-footer-label">{{ $t('dashboard.allocRateLegend') }}</span>
                    </div>
                    <div class="resource-card-footer-value">
                      <span class="resource-card-footer-percent">{{ computeAllocPercentText }}</span>
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
                      <span class="resource-card-footer-label">{{ $t('dashboard.utilizationRateLegend') }}</span>
                    </div>
                    <div class="resource-card-footer-value">
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
                  <div class="resource-card-value resource-card-value--compute">
                    {{ gpuMemoryTotalText }}
                  </div>
                  <div class="resource-card-sub-title">
                    {{ $t('dashboard.memoryTotal') }}
                  </div>
                </div>
              </div>

              <div class="resource-card-footer">
                <div class="resource-card-rate-wrap">
                  <div class="resource-card-footer-item">
                    <div class="resource-card-footer-title">
                      <span class="resource-card-footer-label">{{ $t('dashboard.allocRateLegend') }}</span>
                      <metric-help
                        :description="$t('dashboard.memAllocRateDescription')"
                        :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memAllocRate') })"
                      />
                    </div>
                    <div class="resource-card-footer-value">
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
                      <span class="resource-card-footer-label">{{ $t('dashboard.overviewMemoryOccupancyLegend') }}</span>
                      <metric-help
                        :description="$t('dashboard.memUsageRateDescription')"
                        :help-label="$t('dashboard.metricHelpLabel', { metric: $t('dashboard.memUsageRate') })"
                      />
                    </div>
                    <div class="resource-card-footer-value">
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
    </section>

    <NodeDeviceAllocation :node="allocationNode" />

    <TrendTimeFilter
      v-model="times"
      class="node-trend-filter"
      :loading="trendLoading"
      :min-range-ms="NODE_TREND_MIN_RANGE_MS"
      :max-range-ms="NODE_TREND_MAX_RANGE_MS"
    />

    <div class="line-box">
      <ResourceTrend kind="compute" :series="computeTrend" />
      <ResourceTrend kind="memory" :series="memoryTrend" />
    </div>
    </detail-page-state>
  </div>
</template>

<script setup lang="jsx">
import PageHeader from '@/components/PageHeader.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import TrendTimeFilter from '@/components/TrendTimeFilter.vue';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { computed, ref } from 'vue';
import DetailPageState from '~/vgpu/components/DetailPageState.vue';
import LoadingValue from '~/vgpu/components/LoadingValue.vue';
import MetricHelp from '~/vgpu/components/MetricHelp.vue';
import {
  classifyDetailPayload,
} from '~/vgpu/hooks/detail-resource-state.mjs';
import useDetailResource from '~/vgpu/hooks/useDetailResource';
import apiClient, { ApiClientError } from '@/api/client.mjs';
import { useTelemetryTrends } from '@/hooks/useTelemetryTrends.mjs';
import ResourceTrend from '~/vgpu/components/ResourceTrend.vue';
import { useI18n } from 'vue-i18n';
import { getResourceColor, roundToDecimal } from '@/utils';
import { getNodeSchedulingEligibilityStatus } from '~/vgpu/views/node/node-status.mjs';
import NodeDeviceAllocation from './NodeDeviceAllocation.vue';

const NODE_TREND_MIN_RANGE_MS = 30 * 1000;
const NODE_TREND_MAX_RANGE_MS = 31 * 24 * 60 * 60 * 1000;
const NODE_TREND_METRICS = [
  { key: 'compute-allocation', metric: 'compute_allocation_percent', sectionIndex: 'compute' },
  { key: 'compute-usage', metric: 'compute_utilization_percent', sectionIndex: 'compute' },
  { key: 'memory-allocation', metric: 'memory_allocation_percent', sectionIndex: 'memory' },
  { key: 'memory-usage', metric: 'memory_utilization_percent', sectionIndex: 'memory' },
];

const route = useRoute();
const { t, locale } = useI18n();

const nodeSource = computed(() => String(route.params.uid || ''));
const requestedNodeName = computed(() => {
  const value = route.query.nodeName;
  return Array.isArray(value) ? value[0] : value;
});
const {
  data: detail,
  status: detailStatus,
  retry: retryDetail,
} = useDetailResource({
  source: nodeSource,
  request: async (uid, signal) => {
    try {
      const reply = await apiClient.getNode(uid, { signal });
      if (reply?.inventoryCoverage?.entity !== 'device') {
        throw new Error('Node inventory coverage is missing');
      }
      return reply;
    } catch (error) {
      if (error instanceof ApiClientError && error.status === 404 && error.reason === 'RESOURCE_NODE_NOT_FOUND') {
        return {};
      }
      throw error;
    }
  },
  classify: (payload, uid) =>
    classifyDetailPayload(payload, {
      identityKeys: ['uid', 'name'],
      expectedIdentity: { uid },
    }),
});
const allocationNode = computed(() => detailStatus.value === REQUEST_STATUS.READY
  ? { uid: detail.value.uid, name: detail.value.name }
  : null);
const headerName = computed(() =>
  detailStatus.value === REQUEST_STATUS.READY
    ? detail.value.name
    : requestedNodeName.value || nodeSource.value,
);

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);
const trendParams = computed(() => (allocationNode.value
  ? { node: allocationNode.value.name, nodeUid: allocationNode.value.uid }
  : {}));

// Current allocation, capacity, and physical usage come from one inventory snapshot.
const { data: rangeSeries } = useTelemetryTrends({
  scope: 'node',
  params: trendParams,
  metrics: NODE_TREND_METRICS,
  times,
});
const computeTrend = computed(() => rangeSeries.value.filter((item) => item.sectionIndex === 'compute'));
const memoryTrend = computed(() => rangeSeries.value.filter((item) => item.sectionIndex === 'memory'));
const trendLoading = computed(() => rangeSeries.value.some((item) => item.refreshing || item.status === REQUEST_STATUS.LOADING));
const computeAllocPercentRaw = computed(() => detail.value.coreAllocationPercent);
const memoryAllocPercentRaw = computed(() => detail.value.memoryAllocationPercent);
const computeUsagePercentRaw = computed(() => detail.value.computeUtilizationPercent);
const memoryUsagePercentRaw = computed(() => detail.value.memoryUsagePercent);

const buildPercentViews = (raw) => {
  const progress = computed(() => Number.isFinite(raw.value) ? Math.max(0, Math.min(100, raw.value)) : undefined);
  const progressRounded = computed(() => progress.value === undefined ? undefined : roundToDecimal(progress.value, 2));
  const text = computed(() => Number.isFinite(raw.value) ? `${roundToDecimal(raw.value, 2).toFixed(2)}%` : '--');
  return { progress, progressRounded, text };
};

const {
  progress: computeAllocPercentProgress,
  progressRounded: computeAllocPercentProgressRounded,
  text: computeAllocPercentText,
} = buildPercentViews(computeAllocPercentRaw);
const {
  progress: computeUsagePercentProgress,
  progressRounded: computeUsagePercentProgressRounded,
  text: computeUsagePercentText,
} = buildPercentViews(computeUsagePercentRaw);
const {
  progress: memoryAllocPercentProgress,
  progressRounded: memoryAllocPercentProgressRounded,
  text: memoryAllocPercentText,
} = buildPercentViews(memoryAllocPercentRaw);
const {
  progress: memoryUsagePercentProgress,
  progressRounded: memoryUsagePercentProgressRounded,
  text: memoryUsagePercentText,
} = buildPercentViews(memoryUsagePercentRaw);

const computePowerTotalText = computed(() => Number.isFinite(detail.value.coreTotal)
  ? String(roundToDecimal(detail.value.coreTotal / 100, 1)) : '--');
const gpuMemoryTotalText = computed(() => Number.isFinite(detail.value.memoryTotal)
  ? `${(detail.value.memoryTotal / 1024).toFixed(2)} GiB` : '--');

const headerStatusDisplay = computed(() => {
  if (detailStatus.value !== REQUEST_STATUS.READY) {
    return { icon: '', text: '' };
  }
  const status = getNodeSchedulingEligibilityStatus(detail.value);
  return {
    icon: status.icon,
    text: t(status.labelKey),
    description: status.descriptionKey ? t(status.descriptionKey) : '',
    showHelp: status.showHelp,
  };
});

const toDisplayText = (value) => (value === '' || value === undefined || value === null ? '--' : value);

const detailColumns = computed(() => [
  {
    label: t('node.detail.nodeIpAddress'),
    value: 'ip',
    render: ({ ip }) => <span>{ip || '--'}</span>,
  },
  {
    label: t('node.detail.nodeUuid'),
    value: 'uid',
    render: ({ uid }) => (
      <EllipsisText text={uid || '--'} mode="middle" tooltip="always" />
    ),
  },
  {
    label: t('node.detail.osVersion'),
    value: 'osImage',
    render: ({ osImage }) => (
      <span>{toDisplayText(osImage)}</span>
    ),
  },
  {
    label: t('node.detail.kubeletVersion'),
    value: 'kubeletVersion',
    render: ({ kubeletVersion }) => (
      <span>{toDisplayText(kubeletVersion)}</span>
    ),
  },
  {
    label: t('node.detail.containerRuntime'),
    value: 'containerRuntimeVersion',
    render: ({ containerRuntimeVersion }) => (
      <span>{toDisplayText(containerRuntimeVersion)}</span>
    ),
  },
  {
    label: t('node.detail.architecture'),
    value: 'architecture',
    render: ({ architecture }) => (
      <span>{toDisplayText(architecture)}</span>
    ),
  },
  {
    label: t('node.detail.kernelVersion'),
    value: 'kernelVersion',
    render: ({ kernelVersion }) => (
      <span>{toDisplayText(kernelVersion)}</span>
    ),
  },
  {
    label: t('node.detail.creationTime'),
    value: 'creationTimestamp',
    render: ({ creationTimestamp }) => (
      <span>{toDisplayText(creationTimestamp)}</span>
    ),
  },
]);

const detailColumnGroups = computed(() => {
  const columns = detailColumns.value || [];
  const mid = Math.ceil(columns.length / 2);
  return [columns.slice(0, mid), columns.slice(mid)];
});

</script>

<style scoped lang="scss">
.node-overview-wrap {
  display: flex;
  margin-top: 16px;
  margin-bottom: 16px;
  gap: 16px;
}

.node-trend-filter {
  margin-top: 24px;
}

.node-overview-panel {
  flex: 1 1 48%;
  min-width: 0;
  margin-bottom: 0;
}

.node-detail {
  display: flex;
  width: 100%;
  height: 100%;

  .node-detail-left {
    width: 100%;
  }

  .node-detail-info-summary {
    width: 100%;

    &-cols {
      display: flex;
      flex-flow: row nowrap;
      gap: 12px;
    }
  }

  .summary-col {
    flex: 1;
    min-width: 0;
    padding: 8px 12px;
    border-radius: 8px;
    background: rgb(0 0 0 / 2%);
  }

  .summary-item {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    margin-bottom: 6px;

    &:last-child {
      margin-bottom: 0;
    }
  }

  .summary-item-label {
    color: #939ea9;
    font-size: 12px;
    font-weight: 400;
    line-height: 16px;

    &::after {
      content: ': ';
    }
  }

  .summary-item-value {
    color: #1d2b3a;
    font-size: 14px;
    font-weight: 500;
    line-height: 20px;
    min-width: 0;

    :deep(.ellipsis-text) {
      margin-right: 8px;
    }
  }

  &.is-en {
    .summary-item-label {
      word-break: break-word;
    }
  }
}

// The cards follow the panel's width, which the sidebar and embedding change apart from the viewport.
.node-workload-panel {
  min-width: 0;
  container: node-resources / inline-size;
}

.node-system-resource-cards {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: row;
  gap: 12px;

  .resource-overview-card {
    flex: 1;
    min-width: 0;
  }
}

.resource-overview-card {
  flex: 1;
}

.resource-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 15px 14px;
  background: #f5f7fa;
  border-radius: 8px;
}

.resource-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
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

.resource-card-footer {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 15px;
  width: 100%;
}

.resource-card-rate-wrap {
  width: 100%;
  min-width: 0;
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
  gap: 4px 8px;
}

.resource-card-footer-title {
  display: inline-flex;
  flex: 1 1 auto;
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
  flex: 0 0 auto;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  margin-left: auto;
  white-space: nowrap;
}

.resource-card-footer-percent {
  font-size: 14px;
  font-weight: 500;
  color: #1d2b3a;
}

.line-box {
  container: metric-trends / inline-size;
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 16px;
  row-gap: 16px;

  > .home-block {
    min-width: 0;
    margin-bottom: 0;
  }
}

// Below 700px half a row cannot fit the longest legend on one line, so the trends stack.
@container metric-trends (max-width: 699px) {
  .line-box > .home-block {
    grid-column: 1 / -1;
  }
}

.node-block {
  display: flex;
  flex-direction: column;
  box-shadow: none;
  margin-bottom: 15px;
  .home-block-content {
    flex: 1;
  }
}

@container node-resources (max-width: 511px) {
  .resource-card-header {
    gap: 10px;
  }

  .resource-card-icon {
    flex-basis: 32px;
    width: 32px;
    height: 32px;
  }
}

@container node-resources (max-width: 399px) {
  .node-system-resource-cards {
    flex-direction: column;
  }
}

@media (max-width: 1200px) {
  .node-overview-wrap {
    flex-direction: column;
  }

  .node-detail .node-detail-info-summary-cols {
    flex-direction: column;
  }
}

</style>
