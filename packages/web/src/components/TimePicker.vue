<template>
  <el-date-picker
    v-model="value"
    :type="type"
    range-separator="至"
    start-placeholder="开始时间"
    end-placeholder="结束时间"
    unlink-panels
    :shortcuts="type.includes('range') && shortcuts"
    class="date-picker"
    :disabled-date="disabledDate"
    v-bind="$attrs"
  ></el-date-picker>
</template>

<script setup lang="jsx">
import { computed, defineProps, defineEmits } from 'vue';
import { ElDatePicker } from 'element-plus';

const props = defineProps({
  modelValue: {},
  type: { type: String, default: 'date' },
  parse: { type: Function, default: (times) => times },
});

const emits = defineEmits(['update:modelValue']);

const value = computed({
  get() {
    return props.modelValue;
  },
  set(val) {
    emits('update:modelValue', props.parse(val));
  },
});

const shortcuts = [
  {
    text: '前 1 小时',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 1);
      return [start, end];
    },
  },
  {
    text: '前 6 小时',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 6);
      return [start, end];
    },
  },
  {
    text: '前 12 小时',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 12);
      return [start, end];
    },
  },
  {
    text: '前 1 天',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24);
      return [start, end];
    },
  },
  {
    text: '前 2 天',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 2);
      return [start, end];
    },
  },
  {
    text: '前 3 天',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 3);
      return [start, end];
    },
  },
  {
    text: '前 1 周',
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 7);
      return [start, end];
    },
  },
  // {
  //   text: '前两周',
  //   value: () => {
  //     const end = new Date();
  //     const start = new Date();
  //     start.setTime(start.getTime() - 3600 * 1000 * 24 * 14);
  //     return [start, end];
  //   },
  // },
  // {
  //   text: '前一个月',
  //   value: () => {
  //     const end = new Date();
  //     const start = new Date();
  //     start.setTime(start.getTime() - 3600 * 1000 * 24 * 30);
  //     return [start, end];
  //   },
  // },
  // {
  //   text: '前三个月',
  //   value: () => {
  //     const end = new Date();
  //     const start = new Date();
  //     start.setTime(start.getTime() - 3600 * 1000 * 24 * 90);
  //     return [start, end];
  //   },
  // },
];

const disabledDate = (time) => {
  return time.getTime() >= Date.now();
};
</script>

<style>
.date-picker {
  max-width: 450px;
}
</style>
