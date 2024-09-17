<template>
  <Block title="资源使用情况">
    <div style="display: flex; align-items: center">
      <div class="alarm-left">
        <div
          class="alarm-left-item"
          v-for="{ title, bg, count, color, icon } in alarmConfig"
          :style="{ background: bg }"
        >
          <div class="title">
            {{ title }}
          </div>
          <div class="count" :style="{ color }">
            {{ count }}
          </div>
          <svg-icon :icon="icon" class="icon-bg" />
        </div>
      </div>
      <ul class="usage">
        <li v-for="item in config">
          <echarts-plus :options="getOptions(item)" />
        </li>
      </ul>
    </div>
  </Block>
</template>

<script setup>
import Block from '~/vgpu/views/monitor/overview/Block.vue';
import monitorApi from '~/vgpu/api/monitor';
import { onMounted, ref, defineProps} from 'vue';
import EchartsPlus from '@/components/Echarts-plus.vue';
import getOptions from './config';
import { getDataByPath } from '@/utils';

const props = defineProps(['data']);

const config = ref([
  {
    title: 'vgpu分配率',
    path: 'distributionRate.vgpu',
    percent: 0,
  },

  {
    title: '算力分配率',
    path: 'distributionRate.core',
    percent: 0,
  },
  {
    title: '显存分配率',
    path: 'distributionRate.memory',
    percent: 0,
  },
  {
    title: '算力利用率',
    path: 'useRate.core',
    percent: 0,
  },

  {
    title: '显存利用率',
    path: 'useRate.memory',
    percent: 0,
  },
]);

const alarmConfig = ref([
  {
    title: 'vgpu超配比',
    bg: '#FEF2F2',
    count: 0,
    icon: 'alarm-warning',
    color: '#DC2626',
    path: 'scaling.vgpu',
  },
  {
    title: '算力超配比',
    bg: '#F5F7FA',
    count: 0,
    icon: 'alarm-history',
    path: 'scaling.memory',
  },
  {
    title: '显存超配比',
    bg: '#F5F7FA',
    count: 0,
    icon: 'alarm-history',
    path: 'scaling.core',
  },
]);

onMounted(async () => {
  const res = await monitorApi.usage(props.data);

  config.value = config.value.map((item) => {
    return { ...item, count: getDataByPath(res, item.path) };
  });

  alarmConfig.value = alarmConfig.value.map((item) => {
    return { ...item, count: getDataByPath(res, item.path) };
  });
});
</script>

<style scoped lang="scss">
.alarm-left {
  display: flex;
  flex-direction: column;
  gap: 12px;
  &-item {
    width: 202px;
    height: 80px;
    border-radius: 6px;
    padding: 16px;
    position: relative;
    .title {
      color: #324558;
      font-family: 'PingFang SC';
      font-size: 12px;
      font-style: normal;
      font-weight: 400;
      line-height: 18px; /* 150% */
      margin-bottom: 10px;
    }
    .count {
      font-family: Roboto;
      font-size: 20px;
      font-style: normal;
      font-weight: 700;
      line-height: 100%; /* 20px */
    }
    .icon-bg {
      position: absolute;
      right: 10px;
      bottom: -10px;
      width: 56px;
      height: 56px;
    }
  }
}
.usage {
  flex: 1;
  list-style: none;
  margin: 0;
  padding: 0;
  //padding-left: 180px;
  display: flex;
  height: 200px;
  gap: 20px;
  li {
    flex: 1;
  }
}
</style>
