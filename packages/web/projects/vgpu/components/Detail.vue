<template>
  <div>
    <back-header> {{ title }}管理 > {{ props.name }} </back-header>
    <block-box class="node-block">
      <div class="node-detail">
        <div class="node-detail-left">
          <div class="title">详细信息</div>
          <ul class="node-detail-info">
            <li v-for="{ label, value, render } in detailColumns">
              <span class="label">{{ label }}</span>
              <component v-if="render" :is="render(detail)" />
              <span v-else class="value">{{ detail[value] }}</span>
            </li>

            <li class="cp" v-if="!hideCp">
              <span v-for="{ label, count } in cp">
                <span class="label">{{ label }}</span>

                <span class="value">{{ count }} 倍</span>
              </span>
            </li>
          </ul>
        </div>

        <ul class="gauges">
          <li v-for="item in gaugeConfig">
            <Gauge v-bind="item" />
          </li>
        </ul>
      </div>
    </block-box>

    <div class="line-box">
      <block-box
        :title="title.replace('率', '趋势（%）')"
        v-for="{ title, data } in gaugeConfig"
      >
        <template #extra>
          <TimeSelect v-model="time" />
        </template>
        <div style="height: 200px">
          <echarts-plus :options="getLineOptions({ data })" />
        </div>
      </block-box>
    </div>

    <block-box title="显卡列表" v-if="type !== 'deviceuuid'">
      <CardList :hideTitle="true" :filters="filters" />
    </block-box>

    <block-box title="任务列表">
      <TaskList :hideTitle="true" :filters="filters" />
    </block-box>
  </div>
</template>

<script setup lang="jsx">
import BackHeader from '@/components/BackHeader.vue';
import { useRoute } from 'vue-router';
import BlockBox from '@/components/BlockBox.vue';
import { onMounted, ref, watch } from 'vue';
import CardList from '~/vgpu/views/card/admin/index.vue';
import TaskList from '~/vgpu/views/task/admin/index.vue';
import Gauge from '~/vgpu/components/gauge.vue';
import useInstantVector from '~/vgpu/hooks/useInstantVector';
import EchartsPlus from '@/components/Echarts-plus.vue';
import cardApi from '~/vgpu/api/card';
import { timeParse } from '@/utils';
import { getLineOptions } from '~/vgpu/components/config';
import TimeSelect from '~/vgpu/components/timeSelect.vue';

const props = defineProps([
  'title',
  'detailColumns',
  'type',
  'detail',
  'name',
  'filters',
  'hideCp',
]);

const route = useRoute();

const time = ref(1 / 24);

const cp = useInstantVector(
  [
    {
      label: 'vGPU 超配',
      count: '0',
      query: `avg(hami_vgpu_count{node=~"$node"})`,
    },
    {
      label: '算力超配',
      count: '0',
      query: `avg(hami_vcore_scaling{node=~"$node"})`,
    },
    {
      label: '显存超配',
      count: '1.5',
      query: `avg(hami_vmemory_scaling{node=~"$node"})`,
    },
  ],
  (query) => query.replaceAll('$node', props.detail.name),
);

const gaugeConfig = useInstantVector(
  [
    {
      title: '算力分配率',
      percent: 0,
      query: `sum(hami_container_vcore_allocated{${props.type}=~"$${props.type}"})`,
      totalQuery: `sum(hami_core_size{${props.type}=~"$${props.type}"})`,
      percentQuery: `sum(hami_container_vcore_allocated{${props.type}=~"$${props.type}"}) / sum(hami_core_size{${props.type}=~"$${props.type}"}) *100`,
      total: 0,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存分配率',
      percent: 0,
      query: `sum(hami_container_vmemory_allocated{${props.type}=~"$${props.type}"}) / 1024`,
      totalQuery: `sum(hami_memory_size{${props.type}=~"$${props.type}"}) / 1024`,
      percentQuery: `(sum(hami_container_vmemory_allocated{${props.type}=~"$${props.type}"}) / 1024) /(sum(hami_memory_size{${props.type}=~"$${props.type}"}) / 1024) *100`,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
    {
      title: '算力使用率',
      percent: 0,
      query: `avg(hami_core_util{${props.type}=~"$${props.type}"})`,
      percentQuery: `avg(hami_core_util_avg{${props.type}=~"$${props.type}"})`,
      totalQuery: `sum(hami_core_size{${props.type}=~"$${props.type}"})`,
      total: 100,
      used: 0,
      unit: ' ',
    },
    {
      title: '显存使用率',
      percent: 0,
      query: `sum(hami_memory_used{${props.type}=~"$${props.type}"}) / 1024`,
      totalQuery: `sum(hami_memory_size{${props.type}=~"$${props.type}"})/1024`,
      percentQuery: `(sum(hami_memory_used{${props.type}=~"$${props.type}"}) / 1024)/(sum(hami_memory_size{${props.type}=~"$${props.type}"})/1024)*100`,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
  ],
  (query) => query.replaceAll(`$${props.type}`, props.name),
  time,
);

// const fetchLineData = async () => {
//   const start = new Date();
//   start.setTime(start.getTime() - 3600 * 1000 * 24 * time.value);
//
//   const lineReqs = gaugeConfig.value.map((item) =>
//     cardApi.getRangeVector({
//       range: {
//         start: timeParse(start),
//         end: timeParse(new Date()),
//         step: '1m',
//       },
//       query: item.percentQuery.replaceAll(`$${props.type}`, props.name),
//     }),
//   );
//
//   const res = await Promise.all(lineReqs);
//
//   gaugeConfig.value = gaugeConfig.value.map((item, index) => ({
//     ...item,
//     data: res[index].data[0]?.values || [],
//   }));
// };

// watch(
//   () => props.detail,
//   async () => {
//     fetchLineData();
//   },
// );

// watch(time, () => {
//   fetchLineData();
// });
</script>

<style lang="scss">
.node-detail {
  display: flex;
  height: 100%;
  gap: 50px;

  ul {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .title {
    color: #1d2b3a;
    font-family: 'PingFang SC';
    font-size: 14px;
    font-style: normal;
    font-weight: 500;
    //line-height: 20px;
    margin-bottom: 20px;
  }
  .node-detail-left {
    min-width: 500px;
  }
  .node-detail-info {
    display: flex;
    flex-direction: column;
    gap: 15px;
    font-size: 12px;

    .label {
      display: inline-block;
      width: 80px;
      color: #939ea9;
    }

    .cp {
      display: flex;
      gap: 25px;
      margin-top: 10px;
    }
  }

  .gauges {
    flex: 1;
    display: flex;
    li {
      flex: 1;
    }
  }
}

.line-box {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  column-gap: 20px;
}

.node-block {
  display: flex;
  flex-direction: column;
  .home-block-content {
    flex: 1;
  }
}
</style>
