<template>
  <ul class="preview">
    <li class="preview-item preview-item--legend" v-if="!hidePie">
      <block-box
        :title="type === 'node' ? t('chart.nodeVendorDist') : t('chart.cardTypeDist')"
        class="nodeCard"
      >
        <div class="pie">
          <VChart
            ref="pieChartRef"
            :option="getPreviewBarPie(pieData, t(isNodeType ? 'common.unitNode' : 'common.unitSheet'))"
            :autoresize="true"
            @click="onPieClick"
          />
        </div>

        <ul class="nodeCard-legend">
          <li
            v-for="{ name, value, color, percent } in pieDataWithPercent"
            :key="name"
            :class="{ 'is-current': currentName === name }"
          >
            <span class="legend-label">
              <span class="color-box" :style="{ backgroundColor: color }" aria-hidden="true"></span>
              <span class="legend-name"><EllipsisText :text="name" focusable /></span>
            </span>
            <span class="legend-count">{{ value }} ({{ percent }}%)</span>
          </li>
        </ul>
      </block-box>
    </li>
    <li v-for="chart in rankingCharts" :key="chart.key" class="preview-item">
      <TabTop :title="chart.title" :config="chart.config" :onClick="handleClick" class="node-top" />
    </li>
  </ul>
</template>

<script setup>
import BlockBox from '@/components/BlockBox.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import { donutSliceColors } from '~/vgpu/metrics/vendor-colors.mjs';
import { getPreviewBarPie } from '~/vgpu/components/preview-pie.mjs';
import { onMounted, ref, computed } from 'vue';
import VChart from 'vue-echarts';
import cardApi from '~/vgpu/api/card';
import TabTop from '~/vgpu/components/TabTop.vue';
import {
  nodeVendorDistributionQuery,
  buildGroupedResourceTopQueries,
} from '~/vgpu/metrics/query-contract.mjs';

const props = defineProps({
  title: {
    default: '',
  },
  type: {
    default: 'node',
  },
  hidePie: Boolean,
  modelDistribution: Array,
  rankings: Object,
  rankingStatus: String,
  handleClick: Function,
  handlePieClick: Function,
  currentName: String,
});
import { useI18n } from 'vue-i18n';
const { t } = useI18n();
const pieChartRef = ref();

const onPieClick = (params) => {
  if (props.modelDistribution && !props.modelDistribution.some(row => row.name && row.name === params.name)) return;
  props.handlePieClick?.(params, pieChartRef.value);
};

const isNodeType = computed(() => props.type === 'node');
const nodeQueries = buildGroupedResourceTopQueries('node');
const rankingCharts = computed(() => {
  const series = (metric, key, label) => ({
    key, tab: t(label),
    ...(isNodeType.value
      ? { query: nodeQueries[metric], nameKey: 'node' }
      : { data: props.rankings[metric], status: props.rankingStatus }),
  });
  return [
    {
      key: 'compute', title: t(isNodeType.value ? 'dashboard.nodeComputeTop5' : 'dashboard.gpuComputeTop5'),
      config: [series('computeAllocation', 'alloc', 'dashboard.allocRate'), series('computeUsage', 'usage', 'dashboard.utilizationRateLegend')],
    },
    {
      key: 'memory', title: t(isNodeType.value ? 'dashboard.nodeMemoryTop5' : 'dashboard.gpuMemoryTop5'),
      config: [series('memoryAllocation', 'alloc', 'dashboard.allocRate'), series('memoryUsage', 'usage', 'dashboard.overviewMemoryOccupancyLegend')],
    },
  ];
});

const nodeVendors = ref([]);
const pieData = computed(() => {
  const rows = isNodeType.value ? nodeVendors.value : (props.modelDistribution || []).map(row => ({
    ...row, name: row.name || t('dashboard.unknownModel'),
  }));
  const colors = donutSliceColors(rows);
  return rows.map(row => ({ ...row, color: colors.get(row.name) }));
});

const totalCount = computed(() =>
  pieData.value.reduce((sum, item) => sum + Number(item.value || 0), 0),
);

const pieDataWithPercent = computed(() => {
  const total = totalCount.value || 0;
  if (!total) {
    return pieData.value.map((item) => ({
      ...item,
      percent: 0,
    }));
  }
  return pieData.value
    .slice()
    .sort((a, b) => Number(b.value || 0) - Number(a.value || 0))
    .map((item) => ({
      ...item,
      percent: Math.round((Number(item.value || 0) * 100) / total),
    }));
});


onMounted(async () => {
  if (!isNodeType.value || props.hidePie) return;
  const { data } = await cardApi.getInstantVector({ query: nodeVendorDistributionQuery });
  nodeVendors.value = data.map(item => ({
    name: item.metric.provider, vendor: item.metric.provider, value: Number(item.value),
  }));
});
</script>

<style scoped lang="scss">
ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
// The cards share one row while each keeps a usable width, and wrap rather than spill past the page.
.preview {
  width: 100%;
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 20px;
  .preview-item {
    flex: 2 1 360px;
    min-width: 0;
  }
  // Room for a model name beside its count.
  .preview-item--legend {
    flex: 1 1 220px;
    min-width: min(250px, 100%);
  }

  .nodeCard {
    height: 100%;

    .pie {
      width: 200px;
      max-width: 100%;
      aspect-ratio: 1;
      margin: 0 auto;
    }

    .nodeCard-legend {
      width: 100%;
      display: flex;
      flex-direction: column;
      gap: 15px;
      max-height: calc(3 * 18px + 2 * 15px);
      overflow-y: auto;
      padding-right: 10px;

      &::-webkit-scrollbar {
        width: 6px;
      }

      &::-webkit-scrollbar-thumb {
        background-color: rgba(0, 0, 0, 0.2);
        border-radius: 3px;
      }

      li {
        display: flex;
        align-items: center;
        gap: 6px;
        min-width: 0;
        font-size: 12px;
        line-height: 18px;

        &.is-current {
          font-weight: bold;
        }
      }

      // One line per model at every width: a long name gives way to an ellipsis, never the count.
      .legend-label {
        display: flex;
        flex: 1 1 auto;
        align-items: center;
        gap: 6px;
        min-width: 0;
      }

      .color-box {
        flex: none;
        width: 8px;
        height: 8px;
        border-radius: 2px;
      }

      // A flex box, so the inline-block name adds no baseline gap below the row.
      .legend-name {
        display: flex;
        min-width: 0;
      }

      .legend-count {
        flex: none;
        margin-left: 4px;
        white-space: nowrap;
        font-variant-numeric: tabular-nums;
      }
    }
  }

  .node-top {
    container: top5-card / inline-size;
    display: flex;
    flex-direction: column;
    min-height: 300px;
    height: 100%;
    & > :nth-child(2) {
      flex: 1;
      max-height: 240px;
    }
  }

  // The English titles differ in length, so both switches drop under their titles at one width instead of one by one.
  @container top5-card (max-width: 431px) {
    .node-top:lang(en) :deep(.home-block-header .title) {
      flex-basis: 100%;
    }
  }
}
</style>
