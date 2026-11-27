<template>
  <div class="home-layout">
    <header class="home-page-title" tabindex="-1" :aria-label="$t('dashboard.overview')">
      <span>{{ $t('dashboard.overview') }}</span>
      <OverviewIssuesPopover
        :issues="deviceInsights.issues"
        :status="issueStatus"
        @details-visible="issueDetailsOpen = $event"
      />
    </header>
    <div class="home">
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
        <div class="home-bottom-trend-filter">
          <TrendTimeFilter
            v-model="times"
            :loading="trendLoading"
            :min-range-ms="OVERVIEW_TREND_MIN_RANGE_MS"
            :max-range-ms="OVERVIEW_TREND_MAX_RANGE_MS"
            @refresh-current="refreshCurrentSnapshot"
          />
        </div>
        <div class="home-bottom-row home-bottom-trend-row">
          <div
            class="home-bottom-col"
            v-for="section in rangeConfig"
            :key="section.key"
          >
            <ResourceTrend :kind="section.key" :series="section.series" />
          </div>
        </div>

        <div id="device-insights" class="home-device-insights">
          <DeviceUsageDistribution :devices="deviceInsights.devices" :status="usageStatus" @details-visible="usageDetailsOpen = $event" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import DeviceConfigAlert from '~/vgpu/components/DeviceConfigAlert.vue';
import { ref, shallowRef, computed, watch, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import ResourceTrend from '~/vgpu/components/ResourceTrend.vue';
import Block from './Block.vue';
import './style.scss';
import resourcesApi from '@/api/client.mjs';
import DeviceUsageDistribution from './DeviceUsageDistribution.vue';
import OverviewIssuesPopover from './OverviewIssuesPopover.vue';
import { attachLegacyDeviceLinks } from './device-detail-links.mjs';
import { useTelemetryTrends } from '@/hooks/useTelemetryTrends.mjs';
import TrendTimeFilter from '@/components/TrendTimeFilter.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import { startVisibleSnapshots } from './visible-snapshots.mjs';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import OverviewInventory from './OverviewInventory.vue';
import { readContainerSummary, readOverviewGauges, readResourceInventory } from './resource-summary.mjs';

const { t } = useI18n();

const OVERVIEW_TREND_METRICS = [
  { key: 'compute-allocation', metric: 'compute_allocation_percent', sectionIndex: 'compute' },
  { key: 'compute-usage', metric: 'compute_utilization_percent', sectionIndex: 'compute' },
  { key: 'memory-allocation', metric: 'memory_allocation_percent', sectionIndex: 'memory' },
  { key: 'memory-usage', metric: 'memory_utilization_percent', sectionIndex: 'memory' },
];
const TREND_SECTION_KEYS = ['compute', 'memory'];
const OVERVIEW_TREND_MIN_RANGE_MS = 30 * 1000;
const OVERVIEW_TREND_MAX_RANGE_MS = 31 * 24 * 60 * 60 * 1000;

const end = new Date();
const start = new Date(end.getTime() - 3600 * 1000);

const times = ref([start, end]);
const snapshotReaders = new Set();
const subscribe = (reader) => {
  snapshotReaders.add(reader);
  return () => snapshotReaders.delete(reader);
};
let snapshots;
const refreshCurrentSnapshot = (at) => snapshots?.refresh(at);

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

const inventoryState = useInventorySource(
  (signal) => resourcesApi.getOverviewInventory({ signal }),
  readResourceInventory,
);
const workloadState = useInventorySource(
  (signal) => resourcesApi.listContainers({ page: 1, pageSize: 1 }, { signal }),
  readContainerSummary,
);

const cardGaugeConfig = computed(() =>
  readOverviewGauges(inventoryState.value.data?.resourceSummary, inventoryState.value.status).map((item) => ({
    ...item,
    title: t(item.titleKey),
    description: t(item.descriptionKey),
    metricHelpLabel: t('dashboard.metricHelpLabel', {
      metric: t(item.titleKey),
    }),
  })),
);

// Only detail links need the device registration list. Inventory totals and graphs can render on their own.
const issueDetailsOpen = ref(false);
const usageDetailsOpen = ref(false);
const legacyRows = shallowRef([]);
const detailsOpen = computed(() => issueDetailsOpen.value || usageDetailsOpen.value);
watch([detailsOpen, () => inventoryState.value.data], async ([open, inventory], _previous, onCleanup) => {
  if (!open || !inventory) {
    legacyRows.value = [];
    return;
  }
  const controller = new AbortController();
  onCleanup(() => controller.abort());
  try {
    const reply = await resourcesApi.listDevices({}, { signal: controller.signal });
    if (!controller.signal.aborted) legacyRows.value = Array.isArray(reply?.list) ? reply.list : [];
  } catch {
    if (!controller.signal.aborted) legacyRows.value = [];
  }
});
const deviceInsights = computed(() => inventoryState.value.data
  ? attachLegacyDeviceLinks(inventoryState.value.data, legacyRows.value)
  : { devices: [], issues: [] });
const issueStatus = computed(() => inventoryState.value.data?.issueStatus
  ?? (inventoryState.value.status === REQUEST_STATUS.LOADING ? 'loading' : 'error'));
const usageStatus = computed(() => inventoryState.value.data?.usageStatus ?? inventoryState.value.status);

const { data: rangeSeries } = useTelemetryTrends({
  scope: 'cluster',
  metrics: OVERVIEW_TREND_METRICS,
  times,
});
const trendLoading = computed(() => rangeSeries.value.some(item => item.refreshing || item.status === REQUEST_STATUS.LOADING));
const rangeConfig = computed(() => TREND_SECTION_KEYS.map((key) => ({
  key, series: rangeSeries.value.filter(item => item.sectionIndex === key),
})));

onUnmounted(() => snapshots?.stop());
onMounted(() => {
  snapshots = startVisibleSnapshots(snapshotReaders, end);
});

</script>
