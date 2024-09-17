<template>
  <div class="home">
    <div class="home-left">
      <Block title="显卡资源">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            全部<svg-icon icon="more" style="margin-left: 4px" />
          </div>
        </template>
        <div class="card-overview">
          <div v-for="item in cardGaugeConfig" :key="item.title">
            <Gauge v-bind="item" />
          </div>
        </div>
      </Block>
      <Block title="资源总览">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            全部<svg-icon icon="more" style="margin-left: 4px" />
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
      <Block title="节点总览" style="margin-bottom: 16px">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/node/admin')">
            全部<svg-icon icon="more" style="margin-left: 4px" />
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
      <Block title="显卡类型分布" style="margin-bottom: 16px">
        <template #extra>
          <div class="all-btn" @click="router.push('/admin/vgpu/card/admin')">
            全部<svg-icon icon="more" style="margin-left: 4px" />
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
import {
  getCardOptions,
  handleChartClick,
  getRangeOptions,
} from './getOptions';
import Block from './Block.vue';
import './style.scss';
import { timeParse, getDaysInRange, getRandom } from '@/utils';
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
import { rangeConfigInit } from './config';

const router = useRouter();

const end = new Date();
const start = new Date();
start.setTime(start.getTime() - 3600 * 1000);

const times = ref([start, end]);

const handlePieClick = (params) => {
  router.push(`/admin/vgpu/card/admin?type=${params.data.name}`);
};


const alarmData = ref([])
const chartWidth = ref(200);

const cardGaugeConfig = useInstantVector([
  {
    title: 'vGPU 分配率',
    percent: 0,
    query: `avg(sum (hami_container_vgpu_allocated) by (instance))`,
    totalQuery: `avg(sum (hami_vgpu_count) by (instance))`,
    percentQuery: `avg(sum (hami_container_vgpu_allocated) by (instance))/avg(sum (hami_vgpu_count) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: '个',
  },
  {
    title: '算力分配率',
    percent: 0,
    query: `avg(sum(hami_container_vcore_allocated) by (instance))`,
    totalQuery: `avg(sum(hami_core_size) by (instance))`,
    percentQuery: `avg(sum(hami_container_vcore_allocated) by (instance))/avg(sum(hami_core_size) by (instance)) *100`,
    total: 0,
    used: 0,
    unit: ' ',
  },
  {
    title: '显存分配率',
    percent: 0,
    query: `avg(sum(hami_container_vmemory_allocated) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size) by (instance)) / 1024`,
    percentQuery: `(avg(sum(hami_container_vmemory_allocated) by (instance)) / 1024 )/(avg(sum(hami_memory_size) by (instance)) / 1024) *100 `,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
  {
    title: '算力使用率',
    percent: 0,
    query: `avg(sum(hami_core_util) by (instance))`,
    percentQuery: `avg(sum(hami_core_util_avg) by (instance))`,
    totalQuery: `avg(sum(hami_core_size) by (instance))`,
    total: 100,
    used: 0,
    unit: ' ',
  },
  {
    title: '显存使用率',
    percent: 0,
    query: `avg(sum(hami_memory_used) by (instance)) / 1024`,
    totalQuery: `avg(sum(hami_memory_size) by (instance))/1024`,
    percentQuery: `(avg(sum(hami_memory_used) by (instance)) / 1024)/(avg(sum(hami_memory_size) by (instance))/1024)*100`,
    total: 0,
    used: 0,
    unit: 'GiB',
  },
]);

const resourceOverview = ref([
  {
    title: '节点',
    count: 0,
    icon: 'vgpu-node',
    unit: '个',
  },
  {
    title: '显卡',
    count: 0,
    icon: 'vgpu-gpu-d',
    unit: '张',
  },
  {
    title: 'vGPU',
    count: 0,
    icon: 'vgpu-card',
    unit: '个',
  },
  {
    title: '算力',
    count: 12,
    icon: 'vgpu-core',
    unit: ' ',
  },
  {
    title: '显存',
    count: 31,
    icon: 'vgpu-mem',
    unit: 'GiB',
  },
]);

const nodeConfig = reactive({
  instance: [
    { title: '可调度', key: 'yes', color: '#2563EB', value: 0 },
    { title: '禁止调度', key: 'no', color: '#DC2626', value: 0 },
  ],
  core: [
    { title: '已分配', key: 'used', color: '#2563EB', value: 0 },
    { title: '闲置', key: 'free', color: '#B6C2CD', value: 0 },
    { title: '分配率', key: 'percent', color: '#B6C2CD', value: 0, unit: '%' },
  ],
  memory: [
    {
      title: '已分配',
      key: 'used',
      color: '#2563EB',
      value: 0,
      unit: 'GiB',
    },
    {
      title: '闲置',
      key: 'free',
      color: '#B6C2CD',
      value: 0,
      unit: 'GiB',
    },
    {
      title: '分配率',
      key: 'percent',
      color: '#B6C2CD',
      value: 0,
      unit: '%',
    },
  ],
});

const nodeData = useFetchList(nodeApi.getNodeListReq({ filters: {} }));

const cardData = useFetchList(cardApi.getCardListReq({ filters: {} }));

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
    title: '可调度',
    count: nodeData.value.filter((item) => !item.isExternal && item.isSchedulable).length,
    isSchedulable: true,
    isExternal: false,
    status: 'true',
    color: '#16A34A',
  },
  {
    title: '禁止调度',
    count: nodeData.value.filter((item) => !item.isExternal && !item.isSchedulable).length,
    isSchedulable: false,
    isExternal: false,
    status: 'false',
    color: '#1D2B3A',
  },
]);

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

const nodeUsedTop = {
  title: '节点资源使用率 Top5',
  key: 'used',
  config: [
    {
      tab: '算力',
      key: 'core',
      nameKey: 'node',
      data: [],
      query: 'topk(5, avg(hami_core_util_avg) by (node))',
    },
    {
      tab: '显存',
      key: 'memory',
      data: [],
      nameKey: 'node',
      query:
        'topk(5, avg(hami_memory_used) by (node) / avg(hami_memory_size) by (node) * 100)',
    },
  ],
};

const nodeTotalTop = {
  title: '节点资源分配率 Top5',
  key: 'used',
  config: [
    {
      tab: 'vGPU',
      key: 'vgpu',
      nameKey: 'node',
      data: [],
      query: `topk(5, avg(hami_container_vgpu_allocated{}) by (node) / avg(hami_vgpu_count{}) by (node) * 100)`,
    },
    {
      tab: '算力',
      key: 'core',
      nameKey: 'node',
      data: [],
      query:
        'topk(5, avg(hami_container_vcore_allocated{}) by (node) / avg(hami_core_size{}) by (node) * 100)',
    },
    {
      tab: '显存',
      key: 'memory',
      data: [],
      nameKey: 'node',
      query:
        'topk(5, avg(hami_container_vmemory_allocated{}) by (node) / avg(hami_memory_size{}) by (node) * 100)',
    },
  ],
};

const rangeConfig = ref(rangeConfigInit);

const fetchRangeData = () => {

  const params = {
    range: {
      start: timeParse(times.value[0]),
      end: timeParse(times.value[1]),
      step: '1m',
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
          v.data = res.data[0].values;
        });
    }
  }


  cardApi
      .getRangeVector({
        ...params,
        query: `sum({__name__=~"alert:.*:count"})`,
      })
      .then((res) => {
        alarmData.value = res.data[0].values;
      });

};

watchEffect(() => {
  resourceOverview.value[0].count = nodeData.value.length;
  resourceOverview.value[1].count = cardData.value.length;
  resourceOverview.value[2].count = cardGaugeConfig.value[0].total;
  resourceOverview.value[3].count = cardGaugeConfig.value[1].total;
  resourceOverview.value[4].count = cardGaugeConfig.value[2].total.toFixed(0);
});

onMounted(async () => {
  const summary = await monitorApi.summary({
    filters: {},
  });

  const nodeDataRes = {
    yes: nodeData.value.filter((item) => item.isSchedulable).length,
    no: nodeData.value.filter((item) => !item.isSchedulable).length,
  };

  nodeConfig.instance = nodeConfig.instance.map((item) => {
    return { ...item, value: nodeDataRes[item.key] };
  });

  nodeConfig.core = nodeConfig.core.map((item) => {
    const core_total = cardDetail.value[1].percent;
    const coreData = {
      percent: ((summary.coreUsed / core_total).toFixed(2) * 100).toFixed(0),
      used: summary.coreUsed,
      free: summary.coreTotal - summary.coreUsed,
    };

    return { ...item, value: coreData[item.key] };
  });

  nodeConfig.memory = nodeConfig.memory.map((item) => {
    const memory_total = cardDetail.value[2].percent;
    const coreData = {
      percent: (
        (summary.memoryUsed / 1024 / memory_total).toFixed(2) * 100
      ).toFixed(0),
      used: summary.memoryUsed,
      free: summary.memoryTotal - summary.memoryUsed,
    };

    return { ...item, value: coreData[item.key] };
  });
});

watch(
  times,
  () => {
    // fetchLineData();
    fetchRangeData();
  },
  { immediate: true },
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
