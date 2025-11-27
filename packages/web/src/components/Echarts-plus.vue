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
const myChart = ref(null);

const refresh = debounce(() => {
  if (myChart.value) {
    myChart.value.dispose();
  }
  myChart.value = echarts.init(echartsRef.value);
  if (props.options) {
    myChart.value.setOption(props.options);
  }
  if (props.onClick) {
    myChart.value.on('click', (params) => {
      props.onClick(params, myChart.value);
    });
  }
}, 10);

const resizeObserver = new ResizeObserver(() => {
  requestAnimationFrame(() => {
    refresh();
  });
});

const currentName = ref();

watchEffect(() => {
  const options = props.options;
  nextTick(() => {
    if (myChart.value) {
      myChart.value.dispose();
    }
    myChart.value = echarts.init(echartsRef.value);
    if (props.options) {
      myChart.value.setOption(props.options);
    }

    if (props.onClick) {
      myChart.value.off('click');
      myChart.value.on('click', (params) => {
        props.onClick(params, myChart.value);
      });
    }
  });
});

onMounted(() => {
  resizeObserver.observe(echartsRef.value);
});

onUnmounted(() => {
  if (myChart.value) {
    myChart.value.dispose();
  }
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
