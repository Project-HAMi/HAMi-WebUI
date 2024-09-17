<template>
  <div class="echarts-plus" ref="echartsRef"></div>
</template>

<script setup>
import {
  defineProps,
  defineExpose,
  watchEffect,
  ref,
  onMounted,
  nextTick,
  onUnmounted,
} from 'vue';
import * as echarts from 'echarts';
import { debounce } from 'lodash';

const props = defineProps({
  options: Object,
  onClick: Function,
});

const echartsRef = ref(null);

const refresh = debounce(() => {
  const myChart = echarts.init(echartsRef.value);
  myChart.setOption(props.options);
  myChart.on('finished', () => {
    myChart.resize();
    myChart.off('finished');
  });
  if (props.onClick) {
    // myChart.on('click', props.onClick);
  }
}, 10);

const resizeObserver = new ResizeObserver((e) => {
  refresh();
});

const currentName = ref();

watchEffect(() => {
  const options = props.options;
  nextTick(() => {
    const myChart = echarts.init(echartsRef.value);
    myChart.setOption(props.options);

    if (props.onClick) {
      myChart.off('click');
      myChart.on('click', (params) => {
        props.onClick(params, myChart);
      });
    }
  });
});

onMounted(() => {
  resizeObserver.observe(echartsRef.value);
});

onUnmounted(() => {
  resizeObserver.disconnect();
});

defineExpose({ currentName });
</script>

<style>
.echarts-plus {
  width: 100%;
  height: 100%;
}
</style>
