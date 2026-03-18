<template>
  <t-date-range-picker
    v-model="innerValue"
    :value-type="valueType"
    :enable-time-picker="enableTimePicker"
    :format="format"
    :placeholder="[t('common.startTime'), t('common.endTime')]"
    :separator="t('common.to')"
    :presets="showPresets ? presets : undefined"
    :disable-date="disableFuture ? disableRangeDate : undefined"
    :allow-input="allowInput"
    :clearable="clearable"
    v-bind="$attrs"
  >
    <template #prefixIcon>
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width="1em"
        height="1em"
        fill="currentColor"
        viewBox="0 0 16 16"
      >
        <path
          d="M4.666 2V.667H6V2h4V.667h1.333V2H14c.368 0 .666.299.666.667V6h-1.333V3.334h-2v1.333H10V3.334H6v1.333H4.666V3.334h-2v9.333h4V14H2a.667.667 0 0 1-.667-.666V2.667C1.333 2.299 1.631 2 2 2zm6.667 6a2.667 2.667 0 1 0 0 5.334 2.667 2.667 0 0 0 0-5.334m-4 2.667a4 4 0 1 1 8 0 4 4 0 0 1-8 0m3.333-2v2.276l1.529 1.529.943-.943L12 10.39V8.667z"
        />
      </svg>
    </template>
    <template #suffixIcon>
      {{ null }}
    </template>
  </t-date-range-picker>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  valueType: { type: String, default: 'time-stamp' },
  format: { type: String, default: 'YYYY-MM-DD HH:mm:ss' },
  enableTimePicker: { type: Boolean, default: true },
  allowInput: { type: Boolean, default: true },
  clearable: { type: Boolean, default: true },
  showPresets: { type: Boolean, default: true },
  disableFuture: { type: Boolean, default: true },
});

const emit = defineEmits(['update:modelValue']);
const { t } = useI18n();

const innerValue = computed({
  get() {
    const v = props.modelValue;
    if (!Array.isArray(v) || v.length < 2) return [];
    if (props.valueType === 'time-stamp') {
      return v.map((item) => {
        if (typeof item === 'number') return item;
        if (item instanceof Date) return item.getTime();
        return new Date(item).getTime();
      });
    }
    return v;
  },
  set(val) {
    if (!Array.isArray(val) || val.length < 2) return;
    emit('update:modelValue', val);
  },
});

const presets = computed(() => ({
  [t('timeShortcut.last1h')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000;
    return [start, end];
  },
  [t('timeShortcut.last6h')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 6;
    return [start, end];
  },
  [t('timeShortcut.last12h')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 12;
    return [start, end];
  },
  [t('timeShortcut.last1d')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 24;
    return [start, end];
  },
  [t('timeShortcut.last2d')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 24 * 2;
    return [start, end];
  },
  [t('timeShortcut.last3d')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 24 * 3;
    return [start, end];
  },
  [t('timeShortcut.last1w')]: () => {
    const end = Date.now();
    const start = end - 3600 * 1000 * 24 * 7;
    return [start, end];
  },
}));

function disableRangeDate({ date, partial }) {
  const idx = partial === 'end' ? 1 : 0;
  const d = date?.[idx];
  if (d === undefined || d === null || d === '') return false;
  const ms = d instanceof Date ? d.getTime() : new Date(d).getTime();
  return ms >= Date.now();
}
</script>
