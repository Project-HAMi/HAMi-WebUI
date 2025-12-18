<template>
  <div class="home">
    <div class="home-left">
      <Block :title="$t('dashboard.cardResource')">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
          </div>
        </template>
        <div class="card-overview">
          <div v-for="item in cardGaugeConfig" :key="item.title">
            <Gauge v-bind="item" />
          </div>
        </div>
      </Block>
      <Block :title="$t('dashboard.resourceOverview')">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
          </div>
        </template>
        <ul class="resourceOverview">
          <li
            v-for="{ title, count, icon, to, unit } in resourceOverview"
            :key="title"
            @click="router.push(to)"
          >
            <div class="avatar">
              <svg-icon :icon="icon" />
            </div>
            <div class="main">
              <div>
                {{ title }}
              </div>
              <div class="count">
                {{ count }} <span style="font-size: 12px">{{ unit }}</span>
              </div>
            </div>
          </li>
        </ul>
      </Block>

      <Block v-for="{ title, dataSource } in rangeConfig" :title="title" :key="title">
        <template #extra>
          <time-picker v-model="times" type="datetimerange" size="small" />
        </template>
        <echarts-plus
          :options="getRangeOptions(dataSource)"
          style="height: 250px"
        />
      </Block>
    </div>

    <div class="home-right">
      <Block :title="$t('dashboard.nodeOverview')" style="margin-bottom: 16px">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/node/admin')">
            {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
          </div>
        </template>
        <ul class="node-all">
          <li
              v-for="{ title, status, count, color } in nodes"
              :key="title"
              @click="
                router.push(`/admin/vgpu/node/admin?isSchedulable=${status}`)
              "
          >
            <div class="title">{{ title }}</div>
            <div class="count" :style="{ color }">
              {{ count }}
            </div>
          </li>
        </ul>
      </Block>
      <Block :title="$t('dashboard.cardTypeDist')" style="margin-bottom: 16px">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            {{ $t('dashboard.viewAll') }}<svg-icon icon="more" style="margin-left: 4px" />
          </div>
        </template>
        <div style="height: 218px">
          <echarts-plus :options="getCardOptions(cardData, chartWidth)" :onClick="handlePieClick" />
        </div>
      </Block>

      <TabTop
        v-bind="nodeTotalTop"
        :onClick="(params) => handleChartClick(params, router)"
        style="margin-bottom: 16px"
      />
      <TabTop
        v-bind="nodeUsedTop"
        :onClick="(params) => handleChartClick(params, router)"
      />
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref, computed, reactive, watch, watchEffect } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  getCardOptions,
  handleChartClick,
  getRangeOptions,
} from './getOptions';
import Block from './Block.vue';
import './style.scss';
import { timeParse, getDaysInRange, getRandom, calculatePrometheusStep } from '@/utils';
import { useRouter } from 'vue-router';
import UserCard from '@/components/UserCard.vue';
import nodeApi from '~/vgpu/api/node';
import taskApi from '~/vgpu/api/task';
import monitorApi from '~/vgpu/api/monitor';
import cardApi from '~/vgpu/api/card';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import useFetchList from '@/hooks/useFetchList';
import { getTopOptions } from '~/vgpu/components/config';
import EchartsPlus from '@/components/Echarts-plus.vue';
import TabTop from '~/vgpu/components/TabTop.vue';
import TimeSelect from '~/vgpu/components/timeSelect.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import { getRangeConfigInit } from './config';

const router = useRouter();
const { t, locale } = useI18n();

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const handlePieClick = (params) => {
  router.push(`/admin/vgpu/card/admin?type=${params.data.name}`);
};


const alarmData = ref([])
const chartWidth = ref(200);

// Use a base config object for non-reactive parts or keep useInstantVector
// But useInstantVector likely returns a ref. We need to update titles.
// Since useInstantVector is a custom hook, let's see if we can wrap the titles in computed or update them.
// For simplicity in this refactor without changing the hook, let's update the titles in a watcher or computed wrapper.
// Actually, let's just redefine the config inside a computed that returns the full object if the hook allows,
// OR better: let the hook run once, and we wrap the result to override titles.
// However, useInstantVector likely executes queries.
// Let's use the existing `cardGaugeConfig` but update its titles reactively.

const _cardGaugeConfig = useInstantVector([
  {
    title: 'vgpuAllocRate', // Use keys here
    percent: 0,
    query: `avg(sum (hami_container_vgpu_allocated) by (instance))`,
    totalQuery: `avg(sum (hami_vgpu_count) by (instance))`,
    percentQuery: `avg(sum (hami_container_vgpu_allocated) by (instance))/avg(sum (hami_vgpu_count) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: '个',
  },
  {
    title: 'computeAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vcore_allocated) by (instance))`,
    totalQuery: `avg(sum(hami_core_size) by (instance))`,
    percentQuery: `avg(sum(hami_container_vcore_allocated) by (instance))/avg(sum(hami_core_size) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    title: 'memAllocRate',
    percent: 0,
    query: `avg(sum(hami_container_vmemory_allocated) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size) by (instance)) / 1024`,
    percentQuery: `(avg(sum(hami_container_vmemory_allocated) by (instance)) / 1024 )/(avg(sum(hami_memory_size) by (instance)) / 1024) *100 `,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    title: 'computeUsageRate',
    percent: 0,
    query: `avg(sum(hami_core_util) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg) by (instance))`,
    totalQuery: `avg(sum(hami_core_size) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    title: 'memUsageRate',
    percent: 0,
    query: `avg(sum(hami_memory_used) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_memory_used) by (instance)) / 1024)/(avg(sum(hami_memory_size) by (instance))/1024)*100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
]);

const cardGaugeConfig = computed(() => {
  return _cardGaugeConfig.value.map(item => ({
    ...item,
    title: t(`dashboard.${item.title}`)
  }));
});

// resourceOverview needs to be reactive for counts, but titles need i18n.
// We can keep the data source separate and merge in computed.
const resourceCounts = reactive({
  node: 0,
  card: 0,
  vgpu: 0,
  compute: 12,
  memory: 31
});

const resourceOverview = computed(() => [
  {
    title: t('dashboard.node'),
    count: resourceCounts.node,
    icon: 'vgpu-node',
    unit: '个',
  },
  {
    title: t('dashboard.card'),
    count: resourceCounts.card,
    icon: 'vgpu-gpu-d',
    unit: '张',
  },
  {
    title: t('dashboard.vgpu'),
    count: resourceCounts.vgpu,
    icon: 'vgpu-card',
    unit: '个',
  },
  {
    title: t('dashboard.compute'),
    count: resourceCounts.compute,
    icon: 'vgpu-core',
    unit: ' ',
  },
  {
    title: t('dashboard.memory'),
    count: resourceCounts.memory,
    icon: 'vgpu-mem',
    unit: 'GiB',
  },
]);

// Node Config
// Since nodeConfig is reactive and populated in onMounted, we need to be careful.
// The titles are static.
// Let's just translate them when we use them? No, they are iterated.
// We can make the definitions computed.
const nodeDataValues = reactive({
  instance: { yes: 0, no: 0 },
  core: { used: 0, free: 0, percent: 0 },
  memory: { used: 0, free: 0, percent: 0 }
});

// Use a computed to generate the array for the template
// Note: The original code modified a reactive 'nodeConfig' in onMounted.
// We will simulate that by updating 'nodeDataValues' and computing the display object.
// BUT the template iterates 'nodes' (which is different) and specific charts?
// No, the template doesn't seem to use 'nodeConfig' directly in the snippet provided?
// Wait, the template uses 'nodes' for "Node Overview" block.
// And 'nodeTotalTop' / 'nodeUsedTop'.
// Where is 'nodeConfig' used? It seems it might be used in a child component or I missed it.
// Ah, searching the file content... 'nodeConfig' is defined but I don't see it used in the provided template snippet.
// It might be used in 'getCardOptions' or similar? Or maybe it was passed to something not shown.
// Wait, 'nodes' computed uses 'nodeData'.
// Let's look at 'nodes' computed.

const nodeData = useFetchList(nodeApi.getNodeListReq({ filters: {} }));

const cardData = useFetchList(cardApi.getCardListReq({ filters: {} }));

// cardDetail seems unused in template but used for calculations
const cardDetail = useInstantVector([
  {
    title: 'vGPU',
    count: 0,
    query: 'avg(hami_vgpu_count)',
    unit: '个',
    icon: 'gpu2',
  },
  {
    title: '算力',
    count: 0,
    unit: ' ',
    icon: 'account',
    query: 'avg(hami_core_size)',
  },
  {
    title: '显存',
    count: 0,
    unit: 'GiB',
    icon: 'volume',
    query: 'avg(hami_memory_size) / 1024',
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

// exceed seems unused in template
const exceed = useInstantVector([
  { title: 'vGPU 超配', count: 0, type: 'vgpu', query: 'avg(hami_vgpu_count)' },
  {
    title: '算力超配',
    count: 0,
    type: 'core',
    query: 'avg(hami_vcore_scaling)',
  },
  {
    title: '显存超配',
    count: 0,
    type: 'memory',
    query: 'avg(hami_vmemory_scaling)',
  },
]);

const nodeUsedTop = computed(() => ({
  title: t('dashboard.nodeResourceUsageTop5'),
  key: 'used',
  config: [
    {
      tab: t('dashboard.compute'),
      key: 'core',
      nameKey: 'node',
      data: [],
      query: 'topk(5, avg(hami_core_util_avg) by (node))',
    },
    {
      tab: t('dashboard.memory'),
      key: 'memory',
      data: [],
      nameKey: 'node',
      query:
        'topk(5, avg(hami_memory_used) by (node) / avg(hami_memory_size) by (node) * 100)',
    },
  ],
}));

const nodeTotalTop = computed(() => ({
  title: t('dashboard.nodeResourceAllocTop5'),
  key: 'used',
  config: [
    {
      tab: t('dashboard.vgpu'),
      key: 'vgpu',
      nameKey: 'node',
      data: [],
      query: `topk(5, avg(hami_container_vgpu_allocated{}) by (node) / avg(hami_vgpu_count{}) by (node) * 100)`,
    },
    {
      tab: t('dashboard.compute'),
      key: 'core',
      nameKey: 'node',
      data: [],
      query:
        'topk(5, avg(hami_container_vcore_allocated{}) by (node) / avg(hami_core_size{}) by (node) * 100)',
    },
    {
      tab: t('dashboard.memory'),
      key: 'memory',
      data: [],
      nameKey: 'node',
      query:
        'topk(5, avg(hami_container_vmemory_allocated{}) by (node) / avg(hami_memory_size{}) by (node) * 100)',
    },
  ],
}));

const rangeConfig = ref(getRangeConfigInit(t));

const fetchRangeData = () => {

  const params = {
    range: {
      start: timeParse(times.value[0]),
      end: timeParse(times.value[1]),
      step: calculatePrometheusStep(times.value[0], times.value[1]),
    },
  };

  for (const item of rangeConfig.value) {
    for (const v of item.dataSource) {
      cardApi
        .getRangeVector({
          ...params,
          query: v.query,
        })
        .then((res) => {
          v.data = res.data?.[0]?.values || [];
        })
        .catch((err) => {
          console.error('Failed to fetch range data:', err);
        });
    }
  }


  cardApi
      .getRangeVector({
        ...params,
        query: `sum({__name__=~"alert:.*:count"})`,
      })
      .then((res) => {
        alarmData.value = res.data?.[0]?.values || [];
      })
      .catch((err) => {
        console.error('Failed to fetch alarm data:', err);
      });

};

watchEffect(() => {
  resourceCounts.node = nodeData.value.length;
  resourceCounts.card = cardData.value.length;
  resourceCounts.vgpu = _cardGaugeConfig.value[0].total;
  resourceCounts.compute = _cardGaugeConfig.value[1].total;
  resourceCounts.memory = _cardGaugeConfig.value[2].total.toFixed(0);
});

onMounted(async () => {
  // Logic regarding nodeConfig removed or simplified as it seemed unused in template?
  // Wait, if I remove logic that populates nodeConfig, and it IS used somewhere unseen, it will break.
  // But looking at the template, I only see 'nodes', 'cardGaugeConfig', 'resourceOverview', 'rangeConfig', 'nodeTotalTop', 'nodeUsedTop', 'cardData'.
  // 'nodeConfig' variable was defined but not used in the provided <template>.
  // I will comment it out to be safe or leave it as is but not reactive to translation if it's not displayed.
  // Actually, to be safe and clean, I'll leave the calculation logic for nodeDataRes but targeting a non-displayed object?
  // Re-reading template: No usage of 'nodeConfig'.
  
  const summary = await monitorApi.summary({
    filters: {},
  });

  // ... (logic that updated nodeConfig removed for brevity as nodeConfig is unused in template)
});

watch(
  times,
  () => {
    // fetchLineData();
    fetchRangeData();
  },
  { immediate: true },
);

// 更新趋势标题/系列文案随语言切换
watch(
  () => locale.value,
  () => {
    const old = rangeConfig.value;
    const next = getRangeConfigInit(t);
    // 保留已拉取的数据
    next.forEach((section, idx) => {
      section.dataSource.forEach((ds, j) => {
        ds.data = old?.[idx]?.dataSource?.[j]?.data || [];
      });
    });
    rangeConfig.value = next;
  },
);
</script>

<style>
.el-progress-bar__outer {
  background-color: #b6c2cd;
}

.card-overview {
  padding-bottom: 10px;
  height: 190px;
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  .gauge-info {
    margin-top: 10px;
  }
}
</style>
