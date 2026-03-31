<template>
  <ul class="preview">
    <li class="preview-item" style="width: 20%; flex: none" v-if="!hidePie">
      <block-box
        :title="type === 'node' ? t('chart.nodeVendorDist') : t('chart.cardTypeDist')"
        class="nodeCard"
      >
        <div class="pie">
          <echarts-plus
            :options="getPreviewBarPie(pieData, props)"
            :onClick="handlePieClick"
          />
        </div>

        <ul class="nodeCard-legend">
          <li
            v-for="{ name, value, color, percent } in pieDataWithPercent"
            :key="name"
            :style="{
              fontWeight: currentName === name ? 'bold' : 'normal',
            }"
          >
            <div class="left">
              <span
                class="color-box"
                :style="{
                  'background-color': color,
                }"
              ></span>
              <span> {{ name }}</span>
            </div>

            <span>{{ value }} ({{ percent }}%)</span>
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
import EchartsPlus from '@/components/Echarts-plus.vue';
import { getPreviewBarPie } from '~/vgpu/components/config';
import { onMounted, ref, computed } from 'vue';
import cardApi from '~/vgpu/api/card';
import TabTop from '~/vgpu/components/TabTop.vue';

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

const isNodeType = computed(() => props.type === 'node');

const nodeComputeTop5 = computed(() => ({
  title: t('dashboard.nodeComputeTop5'),
  config: [
    {
      tab: t('dashboard.allocRate'),
      key: 'alloc',
      nameKey: props.type,
      data: [],
      query: `topk(5, sum(hami_container_vcore_allocated{}) by (${props.type}) / sum(hami_core_size{}) by (${props.type}) * 100)`,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: `topk(5, avg(hami_core_util_avg) by (${props.type}))`,
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
      query: `topk(5, sum(hami_container_vmemory_allocated{}) by (${props.type}) / sum(hami_memory_size{}) by (${props.type}) * 100)`,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: `topk(5, sum(hami_memory_used) by (${props.type}) / sum(hami_memory_size) by (${props.type}) * 100)`,
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
      query: `topk(5, sum(hami_container_vcore_allocated{}) by (${props.type}) / sum(hami_core_size{}) by (${props.type}) * 100)`,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: `topk(5, avg(hami_core_util_avg) by (${props.type}))`,
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
      query: `topk(5, sum(hami_container_vmemory_allocated{}) by (${props.type}) / sum(hami_memory_size{}) by (${props.type}) * 100)`,
    },
    {
      tab: t('dashboard.usageRateLegend'),
      key: 'usage',
      nameKey: props.type,
      data: [],
      query: `topk(5, sum(hami_memory_used) by (${props.type}) / sum(hami_memory_size{}) by (${props.type}) * 100)`,
    },
  ],
}));

const pieConfig = {
  deviceuuid: {
    query:
      'count by (devicetype) (sum by (deviceuuid, devicetype) (hami_vgpu_count))',
    key: 'devicetype',
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

  const colors = ['#5470c6', '#91cc75', '#2563EB', '#16A34A', '#7dd3fc', '#86efac'];
  pieData.value = data.map((item, index) => {
    return {
      name: item.metric[thisPieConfig.key],
      value: Number(item.value),
      color: colors[index],
    };
  });
});
</script>

<style scoped lang="scss">
ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.preview {
  width: 100%;
  display: flex;
  gap: 16px;
  margin-bottom: 20px;
  .preview-item {
    flex: 1;
  }

  .nodeCard {
    height: 100%;

    .pie {
      width: 200px;
      height: 200px;
      margin: 0 auto;
    }

    .nodeCard-legend {
      width: 100%;
      display: flex;
      flex-direction: column;
      gap: 15px;
      max-height: calc(3 * (12px + 15px));
      overflow-y: auto;
      padding-right: 10px;

      /* 自定义滚动条样式（可选） */
      &::-webkit-scrollbar {
        width: 6px;
      }

      &::-webkit-scrollbar-thumb {
        background-color: rgba(0, 0, 0, 0.2);
        border-radius: 3px;
      }

      li {
        display: flex;
        justify-content: space-between;
        font-size: 12px;
        align-items: center;
        .left {
          display: flex;
          align-items: center;
          gap: 5px;
        }
        .color-box {
          width: 10px;
          height: 10px;
          display: inline-block;
        }
      }
    }
  }

  .node-top {
    display: flex;
    flex-direction: column;
    min-height: 300px;
    height: 100%;
    & > :nth-child(2) {
      flex: 1;
      max-height: 240px;
    }
  }
}
</style>
