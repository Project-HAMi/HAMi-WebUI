<template>
  <div class="home">
    <div class="home-page-title">{{ $t('dashboard.overview') }}</div>

    <DeviceConfigAlert />

    <Block class="home-gpu-metrics" :title="$t('dashboard.cardResource')" all-to="/accelerators" :all-label="$t('dashboard.viewAllGpuResources')">
      <div class="card-overview">
        <div v-for="item in cardGaugeConfig" :key="item.id">
          <Gauge v-bind="item" />
        </div>
      </div>
    </Block>

    <OverviewInventory :inventory="inventoryState" :workloads="workloadState" />

    <div class="home-bottom">
      <div class="home-bottom-trend-filter" v-if="rangeConfig[0] || rangeConfig[1]">
        <TrendTimeFilter v-model="times" :loading="trendLoading" @refresh-current="refreshCurrentSnapshot" />
      </div>
      <div class="home-bottom-row home-bottom-trend-row" v-if="rangeConfig[0] || rangeConfig[1]">
        <div
          class="home-bottom-col"
          v-for="section in rangeConfig"
          :key="section.key"
        >
          <Block :title="section.title">
            <MetricChart
              :status="section.status"
              :option="section.option"
              :note="section.note"
              :state-text="getStateText(section.status)"
              :refreshing="section.refreshing"
              :refresh-error="Boolean(section.refreshError)"
            />
          </Block>
        </div>
      </div>

      <div class="home-bottom-row home-bottom-workload-row">
        <div class="home-bottom-col">
          <Block :title="t('dashboard.nodeWorkloadTop5')">
            <div
              v-if="nodeWorkloadTop5State.status === 'loading'"
              class="workload-table-skeleton"
              aria-busy="true"
            >
              <t-table
                :columns="nodeWorkloadSkeletonColumns"
                :data="nodeWorkloadSkeletonData"
                row-key="name"
                row-class-name="top5-item-list-table-row"
                class="top5-item-list-table"
                :bordered="false"
                aria-hidden="true"
                inert
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
              <t-tooltip :overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE">
                <template #content>
                  <div class="workload-distribution-tooltip">
                    <div>{{ t('dashboard.nodeWorkloadDistributionDesc1') }}</div>
                    <div>{{ t('dashboard.nodeWorkloadDistributionDesc2') }}</div>
                  </div>
                </template>
                <svg-icon icon="help-circle" class="workload-distribution-tip-icon" />
              </t-tooltip>
            </template>
            <MetricChart
              :status="nodeWorkloadDistributionState.status"
              :option="nodeWorkloadDistributionOptions"
              :state-text="getStateText(nodeWorkloadDistributionState.status)"
            />
          </Block>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import DeviceConfigAlert from '~/vgpu/components/DeviceConfigAlert.vue';
import { ref, shallowRef, computed, reactive, onMounted, onUnmounted, h, resolveComponent } from 'vue';
import { useI18n } from 'vue-i18n';
import { MessagePlugin } from 'tdesign-vue-next';
import { buildNodeDetailLocation } from '~/vgpu/views/node/detail-location.mjs';
import MetricChart from '~/vgpu/components/MetricChart.vue';
import { buildTimeSeriesOptions } from '~/vgpu/metrics/chart-presets.mjs';
import { createWorkloadDistributionOptions } from './workload-distribution.mjs';
import Block from './Block.vue';
import './style.scss';
import { useRouter } from 'vue-router';
import resourcesApi from '~/vgpu/api/resources';
import cardApi from '~/vgpu/api/card';
import taskApi from '~/vgpu/api/task';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import useRangeVector from '~/vgpu/hooks/useRangeVector';
import TrendTimeFilter from '@/components/TrendTimeFilter.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import LoadingValue from '~/vgpu/components/LoadingValue.vue';
import { LONG_TEXT_TOOLTIP_STYLE } from '~/vgpu/components/tooltip-policy.mjs';
import { getRangeConfigInit } from './config';
import {
  createNodeWorkloadDistributionQuery,
  createOverviewGaugeConfigs,
} from './metric-config.mjs';
import { startVisibleSnapshots } from './visible-snapshots.mjs';
import {
  createRequestState,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from '@/hooks/request-state.mjs';
import OverviewInventory from './OverviewInventory.vue';
import { readContainerSummary, readResourceInventory } from './resource-summary.mjs';
import { stateTextKey, summarizeRangeSeries } from '~/vgpu/metrics/metric-state.mjs';

const router = useRouter();
const { t } = useI18n();

const end = new Date();
const start = new Date(end.getTime() - 3600 * 1000);

const times = ref([start, end]);
const nodeWorkloadSkeletonData = Array.from({ length: 5 }, (_, index) => ({ name: `loading-${index}` }));

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
        'button',
        {
          type: 'button',
          class: 'node-workload-name-link',
          onClick: (e) => {
            e.stopPropagation();
            openWorkloadNode(row.name);
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
const nodeWorkloadSkeletonColumns = computed(() => nodeWorkloadColumns.value.map(({ colKey, title, width }) => ({
  colKey,
  title,
  width,
  cell: () => h(LoadingValue, { height: 24, width: colKey === 'index' ? '24px' : '70%' }),
})));
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

const snapshotReaders = new Set();
const subscribe = (reader) => {
  snapshotReaders.add(reader);
  return () => snapshotReaders.delete(reader);
};
let snapshots;
const refreshCurrentSnapshot = (at) => snapshots?.refresh(at);
const _cardGaugeConfig = useInstantVector(createOverviewGaugeConfigs(), undefined, undefined, { subscribe });

const cardGaugeConfig = computed(() => _cardGaugeConfig.value.map((item) => ({
  ...item,
  title: t(item.titleKey),
  description: t(item.descriptionKey),
  detailLabel: t(item.detailLabelKey),
  unit: item.unitKey ? t(item.unitKey) : item.unit,
  metricHelpLabel: t('dashboard.metricHelpLabel', {
    metric: t(item.titleKey),
  }),
})));

// Keep the previous successful response visible while refreshing. Once a request
// fails, clear that source instead of presenting old counts as current.
function useInventorySource(load, project) {
  const result = shallowRef({ status: REQUEST_STATUS.LOADING, data: null });
  subscribe(async (_time, signal) => {
    let reply;
    try {
      reply = await load(signal);
    } catch {
      if (!signal.aborted) result.value = { status: REQUEST_STATUS.ERROR, data: null };
      return;
    }
    if (signal.aborted) return;
    try {
      result.value = { status: REQUEST_STATUS.READY, data: project(reply) };
    } catch {
      result.value = { status: REQUEST_STATUS.INVALID, data: null };
    }
  });
  return result;
}

const inventoryState = useInventorySource(resourcesApi.overviewInventory, readResourceInventory);
const workloadState = useInventorySource(
  signal => taskApi.getWorkloads({ filters: {}, page: 1, pageSize: 1 }, signal),
  readContainerSummary,
);

function openWorkloadNode(name) {
  const node = inventoryState.value.data?.nodes.find((row) => row.name === name);
  const to = buildNodeDetailLocation({ uid: node?.uid, nodeName: name });
  if (to) router.push(to);
  else MessagePlugin.error(t('node.nodeNotFound'));
}

const rangeDefinitions = getRangeConfigInit(t);
const rangeSeriesDefinitions = rangeDefinitions.flatMap((section, sectionIndex) =>
  section.dataSource.map((series, seriesIndex) => ({ ...series, sectionIndex, seriesIndex })),
);
const { data: rangeSeries } = useRangeVector(
  rangeSeriesDefinitions,
  (query) => query,
  times,
);
const trendLoading = computed(() => rangeSeries.value.some((item) => item.refreshing || item.status === REQUEST_STATUS.LOADING));

const rangeConfig = computed(() => {
  const translated = getRangeConfigInit(t);
  return translated.map((section, sectionIndex) => {
    const dataSource = section.dataSource.map((series, seriesIndex) => {
      const state = rangeSeries.value.find(
        (item) => item.sectionIndex === sectionIndex && item.seriesIndex === seriesIndex,
      );
      return {
        ...series,
        data: state?.data || [],
        status: state?.status || REQUEST_STATUS.LOADING,
        refreshing: state?.refreshing || false,
        refreshError: state?.refreshError || null,
      };
    });
    return {
      ...section,
      ...summarizeRangeSeries(dataSource, t),
      option: buildTimeSeriesOptions({ series: dataSource }),
    };
  });
});

const getStateText = (status, metric = true) =>
  t(stateTextKey(status, { metric }));

onUnmounted(() => snapshots?.stop());
onMounted(() => {
  snapshots = startVisibleSnapshots(snapshotReaders, end);
  fetchNodeWorkloadTop5();
  fetchNodeWorkloadDistribution();
});

</script>
