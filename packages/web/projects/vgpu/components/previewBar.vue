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
            :option="getPreviewBarPie(pieData)"
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
    <li v-if="isNodeType" class="preview-item">
      <TabTop v-bind="nodeComputeTop5" :onClick="handleClick" class="node-top" />
    </li>
    <li v-else class="preview-item">
      <TabTop v-bind="gpuComputeTop5" :onClick="handleClick" class="node-top" />
    </li>
    <li v-if="isNodeType" class="preview-item">
      <TabTop v-bind="nodeMemoryTop5" :onClick="handleClick" class="node-top" />
    </li>
    <li v-else class="preview-item">
      <TabTop v-bind="gpuMemoryTop5" :onClick="handleClick" class="node-top" />
    </li>
  </ul>
</template>

<script setup>
import BlockBox from '@/components/BlockBox.vue';
import EllipsisText from '@/components/EllipsisText.vue';
import { categoricalColor } from '~/vgpu/metrics/chart-colors.mjs';
import { getPreviewBarPie } from '~/vgpu/components/preview-pie.mjs';
import { onMounted, ref, computed } from 'vue';
import VChart from 'vue-echarts';
import cardApi from '~/vgpu/api/card';
import TabTop from '~/vgpu/components/TabTop.vue';
import {
  buildGroupedResourceTopQueries,
  buildUnknownComputeShareQuery,
} from '~/vgpu/metrics/query-contract.mjs';

const props = defineProps({
  title: {
    default: '',
  },
  type: {
    default: 'node',
  },
  hidePie: Boolean,
  handleClick: Function,
  handlePieClick: Function,
  currentName: String,
});
import { useI18n } from 'vue-i18n';
const { t } = useI18n();
const pieChartRef = ref();

const onPieClick = (params) => {
  props.handlePieClick?.(params, pieChartRef.value);
};

const isNodeType = computed(() => props.type === 'node');
const resourceTopQueries = computed(() =>
  buildGroupedResourceTopQueries(props.type),
);


const nodeComputeTop5 = computed(() => ({
  title: t('dashboard.nodeComputeTop5'),
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.computeAllocation,
      uncountedQuery: buildUnknownComputeShareQuery({ groupLabel: props.type }),
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.computeUsage,
    },
  ],
}));

const nodeMemoryTop5 = computed(() => ({
  title: t('dashboard.nodeMemoryTop5'),
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.memoryAllocation,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.memoryUsage,
    },
  ],
}));

const gpuComputeTop5 = computed(() => ({
  title: t('dashboard.gpuComputeTop5'),
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.computeAllocation,
      uncountedQuery: buildUnknownComputeShareQuery({ groupLabel: props.type }),
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.computeUsage,
    },
  ],
}));

const gpuMemoryTop5 = computed(() => ({
  title: t('dashboard.gpuMemoryTop5'),
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.memoryAllocation,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: resourceTopQueries.value.memoryUsage,
    },
  ],
}));

const pieConfig = {
  device_uuid: {
    query:
      'count by (device_type) (sum by (device_uuid, device_type) (hami_vgpu_count))',
    key: 'device_type',
  },
  node: {
    query: 'count by (provider) (sum by (node,provider) (hami_vgpu_count))',
    key: 'provider',
  },
};

const pieData = ref([]);

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
  const thisPieConfig = pieConfig[props.type];

  const { data } = await cardApi.getInstantVector({
    query: thisPieConfig.query,
  });

  pieData.value = data.map((item, index) => ({
    name: item.metric[thisPieConfig.key],
    value: Number(item.value),
    color: categoricalColor(index),
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
