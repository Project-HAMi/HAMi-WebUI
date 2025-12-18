<template>
  <el-date-picker
    v-model="value"
    :type="type"
    :range-separator="t('common.to')"
    :start-placeholder="t('common.startTime')"
    :end-placeholder="t('common.endTime')"
    unlink-panels
    :shortcuts="type.includes('range') && shortcuts"
    class="date-picker"
    :disabled-date="disabledDate"
    v-bind="$attrs"
  ></el-date-picker>
</template>

<script setup lang="jsx">
import { computed } from 'vue';
import { ElDatePicker } from 'element-plus';
import { useI18n } from 'vue-i18n';

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

const { t } = useI18n();
const shortcuts = computed(() => [
  {
    text: t('timeShortcut.last1h'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 1);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last6h'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 6);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last12h'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 12);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last1d'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last2d'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 2);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last3d'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 3);
      return [start, end];
    },
  },
  {
    text: t('timeShortcut.last1w'),
    value: () => {
      const end = new Date();
      const start = new Date();
      start.setTime(start.getTime() - 3600 * 1000 * 24 * 7);
      return [start, end];
    },
  },
]);
const disabledDate = (time) => {
  return time.getTime() >= Date.now();
};
</script>

<style>
.date-picker {
  max-width: 450px;
}
</style>
