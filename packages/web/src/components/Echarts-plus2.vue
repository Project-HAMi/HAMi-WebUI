<template>
  <div class="echarts-plus" ref="echartsRef"></div>
</template>

<script setup>
import { defineProps, ref, onMounted, onUnmounted, watch, nextTick } from 'vue';
import * as echarts from 'echarts';

const props = defineProps({
  options: Object,
});

const echartsRef = ref(null);

const echartsed = ref();

const resizeObserver = new ResizeObserver((e) => {
  echartsed.value.resize();
});

watch(
  () => props.options,
  (newVal) => {
    echartsed.value.setOption(newVal);
  },
);

onMounted(() => {
  echartsed.value = echarts.init(echartsRef.value);
  nextTick(() => {
    resizeObserver.observe(echartsRef.value);
  });
});

onUnmounted(() => {
  resizeObserver.disconnect();
});
</script>

<style>
.echarts-plus {
  width: 100%;
  height: 100%;
}
</style>
