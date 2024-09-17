<template>
  <div class="gauge-box">
    <echarts-plus
      :options="getOptions({ ...$props, unit: gaugeUnit })"
      class="gauge-box-echarts"
    />
    <div class="gauge-info" v-if="!hideInfo">
      <span>{{ title.includes('使用') ? '使用' : '分配' }}</span>
      <span v-if="!title.includes('算力')">({{ unit }})</span> :
      <b>{{ used.toFixed(1) }}/{{ total.toFixed() }}</b>
    </div>
  </div>
</template>
<script setup>
import EchartsPlus from '@/components/Echarts-plus.vue';
import getOptions from './config';
import { defineProps } from 'vue';

const props = defineProps([
  'title',
  'total',
  'used',
  'unit',
  'gaugeUnit',
  'percent',
  'hideInfo',
]);
</script>

<style>
.gauge-box {
  display: flex;
  flex-direction: column;
  height: 100%;
  text-align: center;
  font-size: 12px;
  .gauge-box-echarts {
    flex: 1;
  }
}
</style>
