<template>
  <div class="home">
    <div class="home-page-title">{{ $t('dashboard.overview') }}</div>

    <div class="home-top">
      <div class="home-top-left">
        <Block :title="$t('dashboard.cardResource')">
          <template #extra>
            <RouterLink
              class="all-btn"
              to="/admin/vgpu/card/admin"
              :aria-label="$t('dashboard.viewAllGpuResources')"
            >
              {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
            </RouterLink>
          </template>
          <div class="card-overview">
            <div v-for="item in cardGaugeConfig" :key="item.id">
              <Gauge v-bind="item" />
            </div>
          </div>
        </Block>

        <Block :title="$t('dashboard.resourceOverview')">
          <ul class="resourceOverview">
            <li
              v-for="{ title, count, icon, to, unit, status, metric } in resourceOverview"
              :key="title"
            >
              <component
                :is="to ? RouterLink : 'div'"
                :to="to || undefined"
                class="resource-overview-item"
                :class="{ 'is-clickable': !!to }"
                :aria-busy="status === 'loading'"
              >
                <div class="avatar vgpu-table-name-icon-card">
                  <svg-icon :icon="icon" />
                </div>
                <div class="main">
                  <div v-if="status === 'loading'" class="resource-count-skeleton">
                    <t-skeleton
                      animation="gradient"
                      :row-col="[{ width: '56%', height: '22px' }]"
                      aria-hidden="true"
                    />
                    <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
                  </div>
                  <div v-else-if="status === 'ready'" class="count">
                    {{ count }}<span v-if="unit" class="count-unit"> {{ unit }}</span>
                  </div>
                  <div v-else class="resource-state-text">
                    {{ getStateText(status, metric) }}
                  </div>
                  <div class="title">
                    {{ title }}
                  </div>
                </div>
                <svg-icon
                  v-if="to"
                  icon="jump"
                  class="resource-overview-nav-icon"
                  aria-hidden="true"
                />
              </component>
            </li>
          </ul>
        </Block>
      </div>

      <div class="home-top-right">
        <Block :title="$t('dashboard.nodeOverview')" class="home-top-right-card">
          <template #extra>
            <RouterLink
              class="all-btn"
              to="/admin/vgpu/node/admin"
              :aria-label="$t('dashboard.viewAllNodes')"
            >
              {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
            </RouterLink>
          </template>
          <div
            v-if="nodeListState.status.value === 'loading'"
            class="node-overview-skeleton"
            aria-busy="true"
          >
            <t-skeleton
              v-for="index in 2"
              :key="index"
              animation="gradient"
              :row-col="[{ width: '100%', height: '56px' }]"
              aria-hidden="true"
            />
            <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
          </div>
          <div
            v-else-if="nodeListState.status.value !== 'ready'"
            class="overview-state"
          >
            {{ getStateText(nodeListState.status.value, false) }}
          </div>
          <ul v-else class="node-all">
            <li
              v-for="{ title, status, count, color } in nodes"
              :key="title"
            >
              <RouterLink
                class="node-overview-link"
                :to="{ path: '/admin/vgpu/node/admin', query: { isSchedulable: status } }"
              >
                <div class="title">{{ title }}</div>
                <div class="node-overview-value">
                  <div class="count" :style="{ color }">
                    {{ count }}
                  </div>
                  <svg-icon icon="jump" class="node-overview-nav-icon" aria-hidden="true" />
                </div>
              </RouterLink>
            </li>
          </ul>
        </Block>
        <Block :title="$t('dashboard.cardTypeDist')" class="home-top-right-card">
          <template #extra>
            <RouterLink
              class="all-btn"
              to="/admin/vgpu/card/admin"
              :aria-label="$t('dashboard.viewAllGpuTypes')"
            >
              {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
            </RouterLink>
          </template>
          <div
            v-if="cardListState.status.value === 'loading'"
            class="card-type-skeleton"
            aria-busy="true"
          >
            <t-skeleton
              animation="gradient"
              :row-col="[{ type: 'circle', size: '132px' }]"
              aria-hidden="true"
            />
            <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
          </div>
          <div
            v-else-if="cardListState.status.value !== 'ready'"
            class="overview-state"
          >
            {{ getStateText(cardListState.status.value, false) }}
          </div>
          <div v-else-if="!cardData.length" class="overview-state">
            {{ $t('common.noData') }}
          </div>
          <div v-else class="card-type-chart">
            <VChart
              :option="getCardOptions(cardData, chartWidth)"
              :autoresize="true"
              @click="handlePieClick"
            />
          </div>
        </Block>
      </div>
    </div>

    <div class="home-bottom">
      <div class="home-bottom-trend-filter" v-if="rangeConfig[0] || rangeConfig[1]">
        <TrendTimeFilter v-model="times" />
      </div>
      <div class="home-bottom-row" v-if="rangeConfig[0] || rangeConfig[1]">
        <div class="home-bottom-col" v-if="rangeConfig[0]">
          <Block :title="rangeConfig[0].title">
            <div
              class="range-chart-content"
              :aria-busy="rangeConfig[0].status === 'loading' || rangeConfig[0].refreshing"
            >
              <template v-if="rangeConfig[0].status === 'loading'">
                <t-skeleton
                  animation="gradient"
                  :row-col="[
                    { width: '100%', height: '200px' },
                    { width: '42%', height: '16px', margin: '16px auto 0' },
                  ]"
                  class="range-chart-skeleton"
                  aria-hidden="true"
                />
                <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
              </template>
              <VChart
                v-else-if="rangeConfig[0].status === 'ready'"
                :option="getRangeOptions(rangeConfig[0].dataSource)"
                :autoresize="true"
                style="height: 250px"
              />
              <div v-else class="overview-state overview-state--chart">
                {{ getStateText(rangeConfig[0].status) }}
              </div>
              <div
                v-if="rangeConfig[0].refreshing || rangeConfig[0].refreshError || rangeConfig[0].partialStatusText"
                class="range-status-list"
                role="status"
              >
                <span
                  v-if="rangeConfig[0].partialStatusText"
                  class="range-partial-status"
                >
                  {{ rangeConfig[0].partialStatusText }}
                </span>
                <span v-if="rangeConfig[0].refreshing" class="range-refresh-status">
                  {{ $t('common.loading') }}
                </span>
                <span
                  v-else-if="rangeConfig[0].refreshError"
                  class="range-refresh-status range-refresh-status--error"
                >
                  {{ $t('common.refreshFailedShowingPreviousResult') }}
                </span>
              </div>
            </div>
          </Block>
        </div>
        <div class="home-bottom-col" v-if="rangeConfig[1]">
          <Block :title="rangeConfig[1].title">
            <div
              class="range-chart-content"
              :aria-busy="rangeConfig[1].status === 'loading' || rangeConfig[1].refreshing"
            >
              <template v-if="rangeConfig[1].status === 'loading'">
                <t-skeleton
                  animation="gradient"
                  :row-col="[
                    { width: '100%', height: '200px' },
                    { width: '42%', height: '16px', margin: '16px auto 0' },
                  ]"
                  class="range-chart-skeleton"
                  aria-hidden="true"
                />
                <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
              </template>
              <VChart
                v-else-if="rangeConfig[1].status === 'ready'"
                :option="getRangeOptions(rangeConfig[1].dataSource)"
                :autoresize="true"
                style="height: 250px"
              />
              <div v-else class="overview-state overview-state--chart">
                {{ getStateText(rangeConfig[1].status) }}
              </div>
              <div
                v-if="rangeConfig[1].refreshing || rangeConfig[1].refreshError || rangeConfig[1].partialStatusText"
                class="range-status-list"
                role="status"
              >
                <span
                  v-if="rangeConfig[1].partialStatusText"
                  class="range-partial-status"
                >
                  {{ rangeConfig[1].partialStatusText }}
                </span>
                <span v-if="rangeConfig[1].refreshing" class="range-refresh-status">
                  {{ $t('common.loading') }}
                </span>
                <span
                  v-else-if="rangeConfig[1].refreshError"
                  class="range-refresh-status range-refresh-status--error"
                >
                  {{ $t('common.refreshFailedShowingPreviousResult') }}
                </span>
              </div>
            </div>
          </Block>
        </div>
      </div>

      <div class="home-bottom-row home-bottom-top5-row">
        <div class="home-bottom-col">
          <TabTop
            v-bind="nodeComputeTop5"
            :onClick="(params) => handleChartClick(params, router)"
          />
        </div>
        <div class="home-bottom-col">
          <TabTop
            v-bind="nodeMemoryTop5"
            :onClick="(params) => handleChartClick(params, router)"
          />
        </div>
        <div class="home-bottom-col">
          <Block :title="t('dashboard.nodeWorkloadTop5')">
            <div
              v-if="nodeWorkloadTop5State.status === 'loading'"
              class="workload-table-skeleton"
              aria-busy="true"
            >
              <t-skeleton
                animation="gradient"
                :row-col="workloadTableSkeletonRows"
                aria-hidden="true"
              />
              <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
            </div>
            <t-table
              v-else-if="nodeWorkloadTop5State.status === 'ready'"
              :columns="nodeWorkloadColumns"
              :data="nodeWorkloadTop5TableData"
              row-key="name"
              row-class-name="top5-item-list-table-row"
              class="top5-item-list-table"
              :bordered="false"
            />
            <div v-else class="overview-state overview-state--chart">
              {{ getStateText(nodeWorkloadTop5State.status) }}
            </div>
          </Block>
        </div>
        <div class="home-bottom-col">
          <Block :title="t('dashboard.nodeWorkloadDistribution')" class="workload-distribution-block">
            <template #extra>
              <t-tooltip>
                <template #content>
                  <div class="workload-distribution-tooltip">
                    <div>{{ t('dashboard.nodeWorkloadDistributionDesc1') }}</div>
                    <div>{{ t('dashboard.nodeWorkloadDistributionDesc2') }}</div>
                  </div>
                </template>
                <svg-icon icon="help-circle" class="workload-distribution-tip-icon" />
              </t-tooltip>
            </template>
            <div
              class="workload-distribution-content"
              :aria-busy="nodeWorkloadDistributionState.status === 'loading'"
            >
              <template v-if="nodeWorkloadDistributionState.status === 'loading'">
                <t-skeleton
                  animation="gradient"
                  :row-col="[
                    { width: '100%', height: '200px' },
                    { width: '42%', height: '16px', margin: '16px auto 0' },
                  ]"
                  class="range-chart-skeleton"
                  aria-hidden="true"
                />
                <span class="overview-sr-only" role="status">{{ $t('common.loading') }}</span>
              </template>
              <VChart
                v-else-if="nodeWorkloadDistributionState.status === 'ready'"
                :option="nodeWorkloadDistributionOptions"
                :autoresize="true"
                style="height: 250px"
              />
              <div v-else class="overview-state overview-state--chart">
                {{ getStateText(nodeWorkloadDistributionState.status) }}
              </div>
            </div>
          </Block>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, reactive, onMounted, h, resolveComponent } from 'vue';
import { useI18n } from 'vue-i18n';
import VChart from 'vue-echarts';
import {
  getCardOptions,
  handleChartClick,
  getRangeOptions,
} from './getOptions';
import { createWorkloadDistributionOptions } from './workload-distribution.mjs';
import Block from './Block.vue';
import './style.scss';
import { RouterLink, useRouter } from 'vue-router';
import nodeApi from '~/vgpu/api/node';
import cardApi from '~/vgpu/api/card';
import taskApi from '~/vgpu/api/task';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import useRangeVector from '~/vgpu/hooks/useRangeVector';
import useFetchList from '@/hooks/useFetchList';
import TrendTimeFilter from '@/components/TrendTimeFilter.vue';
import TabTop from '~/vgpu/components/TabTop.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import { getRangeConfigInit } from './config';
import {
  createNodeTopQueries,
  createNodeWorkloadDistributionQuery,
  createOverviewGaugeConfigs,
} from './metric-config.mjs';
import {
  createRequestState,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from '@/hooks/request-state.mjs';
import {
  aggregateStatuses,
  getPartialRangeStates,
  stateTextKey,
} from './overview-state.mjs';

const router = useRouter();
const { t } = useI18n();

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);
const workloadTableSkeletonRows = Array.from({ length: 6 }, () => ({
  width: '100%',
  height: '32px',
}));

const handlePieClick = (params) => {
  router.push({
    path: '/admin/vgpu/card/admin',
    query: { type: params.data.name },
  });
};

const chartWidth = ref(200);

const nodeWorkloadTop5State = reactive(createRequestState([]));
const nodeWorkloadDistributionState = reactive(createRequestState([]));
const nodeWorkloadColumns = computed(() => [
  {
    colKey: 'index',
    title: '',
    width: 56,
    cell: (_h, { row }) => {
      const Tag = resolveComponent('t-tag');
      return h(Tag, { class: 'row-tag' }, () => row.index);
    },
  },
  {
    colKey: 'name',
    title: t('dashboard.node'),
    ellipsis: {
      props: { theme: 'default' },
      content: (_h, cellParams) => cellParams.row.name,
    },
    cell: (_h, { row }) =>
      h(
        'span',
        {
          class: 'node-workload-name-link',
          onClick: (e) => {
            e.stopPropagation();
            handleChartClick({ data: row }, router);
          },
        },
        row.name,
      ),
  },
  {
    colKey: 'value',
    title: t('dashboard.workloadCount'),
  },
]);
const nodeWorkloadTop5TableData = computed(() =>
  nodeWorkloadTop5State.data.map((item, idx) => ({
    ...item,
    index: idx + 1,
  })),
);

const nodeWorkloadDistributionOptions = computed(() => {
  return createWorkloadDistributionOptions({
    rows: nodeWorkloadDistributionState.data,
    translate: t,
  });
});

// These page-specific vectors share request-state mechanics without introducing
// a universal query component for two one-off presentations.
const fetchVectorRows = async (state, query, transform) => {
  const hasResolved = state.hasResolved;
  const requestId = startRequest(state, { hasResolved });
  try {
    const res = await cardApi.getInstantVector({ query });
    if (!Array.isArray(res?.data)) {
      resolveRequest(state, {
        data: [],
        status: REQUEST_STATUS.INVALID,
        requestId,
      });
      return;
    }
    const rows = res.data
      .map(transform)
      .filter((row) => Number.isFinite(row?.value));
    resolveRequest(state, {
      data: rows,
      status: rows.length
        ? REQUEST_STATUS.READY
        : res.data.length
          ? REQUEST_STATUS.INVALID
          : REQUEST_STATUS.MISSING,
      requestId,
    });
  } catch (error) {
    rejectRequest(state, error, { hasResolved, requestId });
  }
};

const fetchNodeWorkloadTop5 = () =>
  fetchVectorRows(
    nodeWorkloadTop5State,
    'topk(5, count(count by (node, container_pod_uuid) (hami_container_vgpu_allocated{})) by (node))',
    (item) => ({
      name: item?.metric?.node || '-',
      value: Number(item?.value),
    }),
  ).then(() => {
    if (nodeWorkloadTop5State.status === REQUEST_STATUS.READY) {
      nodeWorkloadTop5State.data = nodeWorkloadTop5State.data
        .sort((a, b) => b.value - a.value)
        .slice(0, 5);
    }
  });

const fetchNodeWorkloadDistribution = () =>
  fetchVectorRows(
    nodeWorkloadDistributionState,
    createNodeWorkloadDistributionQuery(),
    (item) => ({
      name: item?.metric?.node || '-',
      value: Number(item?.value),
    }),
  ).then(() => {
    if (nodeWorkloadDistributionState.status === REQUEST_STATUS.READY) {
      nodeWorkloadDistributionState.data = nodeWorkloadDistributionState.data.sort(
        (a, b) => b.value - a.value,
      );
    }
  });

const _cardGaugeConfig = useInstantVector(createOverviewGaugeConfigs());

const clusterResourceConfig = useInstantVector([
  {
    query: 'sum(kube_node_status_allocatable{resource="cpu"})',
    total: 0,
    used: 0,
  },
  {
    query: 'sum(kube_node_status_allocatable{resource="memory"})',
    total: 0,
    used: 0,
  },
]);

const cardGaugeConfig = computed(() => {
  return _cardGaugeConfig.value.map((item) => ({
    ...item,
    title: t(item.titleKey),
    description: t(item.descriptionKey),
    detailLabel: t(item.detailLabelKey),
    unit: item.unitKey ? t(item.unitKey) : item.unit,
    metricHelpLabel: t('dashboard.metricHelpLabel', {
      metric: t(item.titleKey),
    }),
  }));
});

const nodeListState = useFetchList(() =>
  nodeApi.getNodeListReq({ filters: {} }),
);
const cardListState = useFetchList(() =>
  cardApi.getCardListReq({ filters: {} }),
);
const taskListState = useFetchList(
  () => taskApi.getTaskListReq({ filters: {} }),
  'items',
);

const nodeData = nodeListState.data;
const cardData = cardListState.data;
const taskData = taskListState.data;

const readyMetricValue = (metric, field, format) => {
  if (metric?.status !== REQUEST_STATUS.READY) return undefined;
  const value = Number(metric?.[field]);
  return Number.isFinite(value) ? format(value) : undefined;
};

const resourceOverview = computed(() => [
  {
    title: t('dashboard.nodeTotal'),
    count: nodeData.value.length,
    status: nodeListState.status.value,
    metric: false,
    icon: 'vgpu-node',
    unit: '',
    to: '/admin/vgpu/node/admin',
  },
  {
    title: t('dashboard.gpuCardCount'),
    count: cardData.value.length,
    status: cardListState.status.value,
    metric: false,
    icon: 'vgpu-gpu-d',
    unit: '',
    to: '/admin/vgpu/card/admin',
  },
  {
    title: t('dashboard.cpuTotalCores'),
    count: readyMetricValue(
      clusterResourceConfig.value[0],
      'used',
      (value) => value.toFixed(0),
    ),
    status: clusterResourceConfig.value[0]?.status,
    metric: true,
    icon: 'vgpu-card',
    unit: t('dashboard.cpuCoreUnit'),
  },
  {
    title: t('dashboard.workloadCount'),
    count: taskData.value.length,
    status: taskListState.status.value,
    metric: false,
    icon: 'vgpu-workload',
    unit: '',
    to: '/admin/vgpu/task/admin',
  },
  {
    title: t('dashboard.memoryTotal'),
    count: readyMetricValue(
      _cardGaugeConfig.value[2],
      'total',
      (value) => value.toFixed(0),
    ),
    status: _cardGaugeConfig.value[2]?.status,
    metric: true,
    icon: 'vgpu-mem',
    unit: 'GiB',
  },
  {
    title: t('dashboard.systemMemoryTotal'),
    count: readyMetricValue(
      clusterResourceConfig.value[1],
      'used',
      (value) => (value / 1024 / 1024 / 1024).toFixed(0),
    ),
    status: clusterResourceConfig.value[1]?.status,
    metric: true,
    icon: 'vgpu-core',
    unit: 'GiB',
  },
]);

const nodes = computed(() => [

  {
    title: t('dashboard.schedulable'),
    count: nodeData.value.filter((item) => !item.isExternal && item.isSchedulable).length,
    isSchedulable: true,
    isExternal: false,
    status: 'true',
    color: '#16A34A',
  },
  {
    title: t('dashboard.unschedulable'),
    count: nodeData.value.filter((item) => !item.isExternal && !item.isSchedulable).length,
    isSchedulable: false,
    isExternal: false,
    status: 'false',
    color: '#1D2B3A',
  },
]);

const nodeTopQueries = createNodeTopQueries();

const nodeComputeTop5 = computed(() => ({
  title: t('dashboard.nodeComputeTop5'),
  key: 'compute',
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: 'node',
      data: [],
      query: nodeTopQueries.computeAllocation,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      data: [],
      nameKey: 'node',
      query: nodeTopQueries.computeUsage,
    },
  ],
}));

const nodeMemoryTop5 = computed(() => ({
  title: t('dashboard.nodeMemoryTop5'),
  key: 'memory',
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: 'node',
      data: [],
      query: nodeTopQueries.memoryAllocation,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: 'node',
      data: [],
      query: nodeTopQueries.memoryUsage,
    },
  ],
}));


const rangeDefinitions = getRangeConfigInit(t);
const rangeSeriesDefinitions = rangeDefinitions.flatMap((section, sectionIndex) =>
  section.dataSource.map((series, seriesIndex) => ({
    ...series,
    sectionIndex,
    seriesIndex,
  })),
);
const { data: rangeSeries } = useRangeVector(
  rangeSeriesDefinitions,
  (query) => query,
  times,
);

const rangeConfig = computed(() => {
  const translated = getRangeConfigInit(t);
  return translated.map((section, sectionIndex) => {
    const dataSource = section.dataSource.map((series, seriesIndex) => {
      const state = rangeSeries.value.find(
        (item) =>
          item.sectionIndex === sectionIndex && item.seriesIndex === seriesIndex,
      );
      return {
        ...series,
        data: state?.data || [],
        status: state?.status || REQUEST_STATUS.LOADING,
        refreshing: state?.refreshing || false,
        refreshError: state?.refreshError || null,
      };
    });
    const status = aggregateStatuses(dataSource);
    const partialStatusText = getPartialRangeStates(dataSource)
      .map((item) => `${item.name}: ${t(stateTextKey(item.status))}`)
      .join(' · ');
    return {
      ...section,
      dataSource,
      status,
      refreshing: dataSource.some((item) => item.refreshing),
      refreshError: dataSource.find((item) => item.refreshError)?.refreshError || null,
      partialStatusText,
    };
  });
});

const getStateText = (status, metric = true) =>
  t(stateTextKey(status, { metric }));

onMounted(() => {
  fetchNodeWorkloadTop5();
  fetchNodeWorkloadDistribution();
});

</script>

<style>
.card-overview {
  padding-bottom: 10px;
  min-height: 140px;
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 12px;
}
</style>
