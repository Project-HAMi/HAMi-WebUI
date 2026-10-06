<template>
  <div class="trend-time-filter">
    <div class="left">
      <t-radio-group
        :value="currentDateRange"
        theme="button"
        @change="selectRange"
        @keydown.capture="onPresetKeydown"
      >
        <t-radio-button
          v-for="option in dateRangeOptions"
          :key="option.value"
          :value="option.value"
          :data-time-range="option.value"
          @click="refreshSelectedRange(option.value)"
        >
          {{ option.label }}
        </t-radio-button>
      </t-radio-group>
      <t-date-range-picker
        v-if="showCustomDateRangePicker"
        :key="pickerKey"
        ref="customRangePicker"
        v-range-labels="[t('common.startTime'), t('common.endTime')]"
        :value="customDateRange"
        value-type="Date"
        :placeholder="[t('common.startTime'), t('common.endTime')]"
        :separator="t('common.to')"
        :disable-date="{ after: dayjs().format('YYYY-MM-DD') }"
        :time-picker-props="timePickerProps"
        :popup-props="{
          overlayClassName: 'trend-time-filter-popup',
          onVisibleChange: onPickerVisibleChange,
        }"
        :range-input-props="{ inputProps: { onEnter: onCustomEnter } }"
        enable-time-picker
        allow-input
        need-confirm
        :clearable="false"
        class="trend-time-filter-custom"
        @change="onCustomRangeChange"
        @blur="onCustomBlur"
        @keydown.capture="onCustomKeydown"
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
    <span class="trend-time-filter-feedback" role="status" aria-atomic="true">{{ feedback }}</span>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, toRef } from 'vue';
import dayjs from 'dayjs';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import useTrendTimeRange from './useTrendTimeRange.js';

const props = defineProps({
  modelValue: {
    type: Array,
    default: () => [],
  },
});

const emit = defineEmits(['update:modelValue']);
const { t } = useI18n();

const {
  applyCustomRange,
  currentDateRange,
  customDateRange,
  resetCustomRange,
  selectRange: applyPresetRange,
  validationError,
} = useTrendTimeRange(toRef(props, 'modelValue'), (range) => emit('update:modelValue', range));
const showCustomDateRangePicker = computed(() => currentDateRange.value === 'custom');
// Reuse the panel so deferred scroll events cannot outlive it when switching inputs.
const timePickerProps = { key: 'trend-range-time' };
// TDesign forwards inputProps ARIA attributes to wrappers, not native inputs.
const setRangeLabels = (element, { value }) => {
  element.querySelectorAll('input').forEach((input, index) => {
    if (value[index]) input.setAttribute('aria-label', value[index]);
  });
};
const vRangeLabels = { mounted: setRangeLabels, updated: setRangeLabels };
const dateRangeOptions = computed(() => [
  { label: t('dashboard.timeRange_1h'), value: '1h' },
  { label: t('dashboard.timeRange_3h'), value: '3h' },
  { label: t('dashboard.timeRange_6h'), value: '6h' },
  { label: t('dashboard.timeRange_1d'), value: '24h' },
  { label: t('dashboard.timeRange_7d'), value: '168h' },
  { label: t('dashboard.timeRange_30d'), value: '720h' },
  { label: t('dashboard.timeRange_custom'), value: 'custom' },
]);

const feedback = ref('');
let warningMessage;
let pendingEnter = false;
const clearFeedback = () => {
  pendingEnter = false;
  if (warningMessage) MessagePlugin.close(warningMessage);
  warningMessage = undefined;
  feedback.value = '';
};
const showRangeWarning = async (error) => {
  clearFeedback();
  const message = t(`timeRange.${error}`);
  const pendingMessage = MessagePlugin.warning({
    content: message,
    duration: 5000,
    onClose: () => {
      if (warningMessage === pendingMessage) warningMessage = undefined;
    },
  });
  warningMessage = pendingMessage;
  // Keep a live region mounted, and clear it before repeating the same rejection.
  await nextTick();
  if (warningMessage === pendingMessage) feedback.value = message;
};
onBeforeUnmount(clearFeedback);

const selectRange = (selection) => {
  clearFeedback();
  applyPresetRange(selection);
};
const refreshSelectedRange = (selection) => {
  if (selection === currentDateRange.value) selectRange(selection);
};

const onPresetKeydown = (event) => {
  if (event.key !== 'Enter' && event.key !== ' ') return;
  const selection = event.target.closest('[data-time-range]')?.dataset.timeRange;
  if (!selection) return;
  event.preventDefault();
  event.stopPropagation();
  if (!event.repeat) selectRange(selection);
};

const pickerKey = ref(0);
const customRangePicker = ref(null);
const onCustomRangeChange = async (range, context) => {
  pendingEnter = false;
  // TDesign converts empty inputs to now in dayjsValue; validate them first.
  const incomplete = range.some((date) => date === '' || date == null);
  // The picker may reorder dates; validate the original input order as well.
  applyCustomRange(incomplete ? range : context?.dayjsValue?.map((date) => date.toDate()) ?? range);
  if (validationError.value) {
    showRangeWarning(validationError.value);
    return;
  }
  clearFeedback();
  if (context?.trigger === 'confirm') {
    // TDesign has no controlled picker visibility and may keep editing after Confirm.
    pickerVisible.value = false;
    pickerKey.value += 1;
    await nextTick();
    customRangePicker.value?.$el.querySelectorAll('input')[1]?.focus();
  }
};

const pickerVisible = ref(false);
const onCustomKeydown = (event) => {
  if (event.key !== 'Enter' || event.target.tagName !== 'INPUT') return;
  if (event.isComposing || event.keyCode === 229) {
    event.stopPropagation();
    return;
  }
  if (event.repeat) {
    event.preventDefault();
    event.stopPropagation();
    return;
  }
  pendingEnter = true;
};
const onCustomEnter = () => {
  pickerVisible.value = false;
  // TDesign calls inputProps.onEnter after its own handler. Malformed text is
  // restored without change; only report here when onCustomRangeChange did not run.
  if (pendingEnter) {
    pendingEnter = false;
    resetCustomRange();
    showRangeWarning('invalid');
  }
};
const onPickerVisibleChange = (visible, context) => {
  // Switching between inputs keeps TDesign's panel open despite a false toggle.
  pickerVisible.value = context.trigger === 'trigger-element-click' || visible;
  if (!pickerVisible.value) resetCustomRange();
};
const onCustomBlur = ({ e }) => {
  if (!pickerVisible.value && !e.relatedTarget?.closest('.trend-time-filter-custom')) resetCustomRange();
};
</script>

<style lang="scss" scoped>
.trend-time-filter {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  min-width: 0;
  margin-bottom: 16px;

  .left {
    display: flex;
    align-items: center;
    justify-content: flex-start;
    flex-wrap: wrap;
    gap: 8px 12px;
    flex: 1;
    min-width: 0;
  }

  :deep(.t-radio-group) {
    flex: 0 1 auto;
    flex-wrap: nowrap;
    min-width: 0;
    overflow-x: auto;
    white-space: nowrap;
  }

  :deep(.t-radio-button) {
    white-space: nowrap;
  }
}

.trend-time-filter-custom {
  flex: 0 1 420px;
  min-width: 0;
  max-width: 100%;

  :deep(.t-range-input:not(.t-is-disabled) .t-input:not(.t-is-disabled):is(:hover, .t-is-focused)) {
    background-color: var(--td-brand-color-light);
  }
}

.trend-time-filter-feedback {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}
</style>

<style lang="scss">
// Scope the body-attached popup's overlapping preview to one solid range color.
.trend-time-filter-popup {
  .t-date-picker__cell--highlight.t-date-picker__cell--hover-highlight::after {
    background-color: var(--td-brand-color-light);
  }

  .t-date-picker__panel-date
    .t-date-picker__cell:not(.t-date-picker__cell--active):not(.t-date-picker__cell--disabled):not(.t-date-picker__cell--additional):hover {
    .t-date-picker__cell-inner {
      box-shadow: none;
      background-color: var(--td-brand-color-light-hover);
      color: var(--td-brand-color);
    }

    &.t-date-picker__cell--hover-highlight .t-date-picker__cell-inner {
      background-color: transparent;
    }
  }
}
</style>
