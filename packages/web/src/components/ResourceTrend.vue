<template>
  <BlockBox class="resource-trend" :title="title">
    <MetricChart
      :status="summary.status"
      :option="option"
      :note="summary.note"
      note-placement="legend"
      :state-text="t(stateTextKey(summary.status))"
      :refreshing="summary.refreshing"
      :refresh-error="Boolean(summary.refreshError)"
    />
  </BlockBox>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import BlockBox from '@/components/BlockBox.vue';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import MetricChart from './MetricChart.vue';
import { deviceWording } from './device-copy.mjs';
import { CHART_COLORS } from '../metrics/chart-colors.mjs';
import { buildTimeSeriesOptions } from '../metrics/chart-presets.mjs';
import { stateTextKey, summarizeRangeSeries } from '../metrics/metric-state.mjs';

const props = defineProps({
  kind: { type: String, required: true, validator: value => ['compute', 'memory'].includes(value) },
  // Each page supplies allocation and usage in that order for its own scope.
  series: { type: Array, default: () => [] },
  allocationLabel: { type: String, default: '' },
  vendor: { type: String, default: '' },
});
const { t } = useI18n();
const isMemory = computed(() => props.kind === 'memory');
const title = computed(() => deviceWording(t(isMemory.value ? 'dashboard.overviewMemoryTrend' : 'dashboard.overviewComputeTrend'), props.vendor));

function chartLine(state, name, color) {
  const status = state?.status ?? REQUEST_STATUS.LOADING;
  const stateLabel = status === REQUEST_STATUS.READY || status === REQUEST_STATUS.LOADING
    ? '' : t(status === REQUEST_STATUS.MISSING ? 'common.noData' : stateTextKey(status));
  const legendLabel = stateLabel ? t('dashboard.trendSeriesState', { name, state: stateLabel }) : '';
  return { ...state, name, color, status, legendLabel };
}

const lines = computed(() => [
  chartLine(props.series[0], props.allocationLabel || t('dashboard.allocRateLegend'), CHART_COLORS.allocation),
  chartLine(props.series[1], t(isMemory.value ? 'dashboard.overviewMemoryOccupancyLegend' : 'dashboard.utilizationRateLegend'), CHART_COLORS.usage),
]);
const summary = computed(() => summarizeRangeSeries(lines.value, t));
const option = computed(() => buildTimeSeriesOptions({ series: lines.value }));
</script>

<style scoped>
.resource-trend {
  padding-top: 12px;
}
.resource-trend > :deep(.home-block-header) {
  box-sizing: content-box;
  min-height: 32px;
  padding-bottom: 12px;
}
</style>
