<template>
  <div class="trend-time-filter">
    <div class="left">
      <t-radio-group
        v-model="currentDateRange"
        theme="button"
        :options="dateRangeOptions"
      />
      <t-date-range-picker
        v-if="showCustomDateRangePicker"
        v-model="customDateRange"
        :placeholder="[t('common.pleaseSelectDate'), t('common.pleaseSelectDate')]"
        :separator="t('common.to')"
        enable-time-picker
        allow-input
        clearable
        class="trend-time-filter-custom"
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
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  modelValue: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(['update:modelValue']);
const { t } = useI18n();

const currentDateRange = ref('1h');
const customDateRange = ref([]);
const showCustomDateRangePicker = computed(() => currentDateRange.value === 'custom');
const dateRangeOptions = computed(() => [
  { label: t('dashboard.timeRange_1h'), value: '1h' },
  { label: t('dashboard.timeRange_3h'), value: '3h' },
  { label: t('dashboard.timeRange_6h'), value: '6h' },
  { label: t('dashboard.timeRange_1d'), value: '24h' },
  { label: t('dashboard.timeRange_7d'), value: '168h' },
  { label: t('dashboard.timeRange_30d'), value: '720h' },
  { label: t('dashboard.timeRange_custom'), value: 'custom' },
]);

const emitPresetRange = (range) => {
  if (range === 'custom') return;
  const hours = Number(String(range).replace('h', ''));
  if (!hours) return;
  const rangeEnd = new Date();
  const rangeStart = new Date(rangeEnd.getTime() - hours * 3600 * 1000);
  emit('update:modelValue', [rangeStart, rangeEnd]);
};

watch(
  currentDateRange,
  (range) => {
    emitPresetRange(range);
  },
  { immediate: true },
);

watch(
  customDateRange,
  (val) => {
    if (!showCustomDateRangePicker.value || !Array.isArray(val) || val.length !== 2) return;
    const [startTime, endTime] = val;
    if (!startTime || !endTime) return;
    emit('update:modelValue', [startTime, endTime]);
  },
  { deep: true },
);

watch(
  () => props.modelValue,
  (val) => {
    if (!Array.isArray(val) || val.length !== 2) return;
    if (showCustomDateRangePicker.value) {
      customDateRange.value = val;
    }
  },
  { deep: true },
);
</script>

<style lang="scss" scoped>
.trend-time-filter {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  flex-wrap: nowrap;
  margin-bottom: 16px;
  overflow-x: auto;

  .left {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 8px;
    flex: 1;
    min-width: 0;
    overflow-x: auto;
  }

  :deep(.t-radio-group) {
    flex-shrink: 0;
    flex-wrap: nowrap;
    white-space: nowrap;
  }

  :deep(.t-radio-button) {
    white-space: nowrap;
  }
}

.trend-time-filter-custom {
  flex-shrink: 0;
}
</style>
