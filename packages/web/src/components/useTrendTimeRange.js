import { ref, watch } from 'vue';

const HOUR = 60 * 60 * 1000;
const PRESET_HOURS = [1, 3, 6, 24, 168, 720];

const timestamps = (range) => {
  if (!Array.isArray(range) || range.length !== 2) return [];
  return range.map((value) => {
    if (!(value instanceof Date) && typeof value !== 'number' && typeof value !== 'string') return NaN;
    if (typeof value === 'string' && !value.trim()) return NaN;
    return new Date(value).getTime();
  });
};

const rangeError = (range, now) => {
  const [start, end] = timestamps(range);
  if (!Number.isFinite(start) || !Number.isFinite(end)) return 'invalid';
  if (start >= end) return 'order';
  if (end > now) return 'future';
  return '';
};

const copyRange = (range) => timestamps(range).map((value) => new Date(value));
const sameRange = (left, right, precision = 1) => {
  const rightTimes = timestamps(right);
  return timestamps(left).every((value, index) => Math.floor(value / precision) === Math.floor(rightTimes[index] / precision));
};
const presetForRange = (range) => {
  const [start, end] = timestamps(range);
  const hours = PRESET_HOURS.find((value) => Math.abs(end - start - value * HOUR) < 1000);
  return hours ? `${hours}h` : 'custom';
};

export default function useTrendTimeRange(modelValue, onUpdate, now = Date.now) {
  const initialNow = now();
  const hasInitialRange = !rangeError(modelValue.value, initialNow);
  const appliedRange = ref(hasInitialRange
    ? copyRange(modelValue.value)
    : [new Date(initialNow - HOUR), new Date(initialNow)]);
  const currentDateRange = ref(presetForRange(appliedRange.value));
  const customDateRange = ref(copyRange(appliedRange.value));
  const validationError = ref('');

  const resetCustomRange = () => {
    customDateRange.value = copyRange(appliedRange.value);
  };

  const publishRange = (range) => {
    appliedRange.value = copyRange(range);
    resetCustomRange();
    onUpdate(copyRange(range));
  };

  const selectRange = (selection) => {
    validationError.value = '';
    if (selection === 'custom') {
      currentDateRange.value = selection;
      resetCustomRange();
      return;
    }
    const hours = Number(selection.replace('h', ''));
    if (!PRESET_HOURS.includes(hours)) return;
    currentDateRange.value = selection;
    const end = now();
    publishRange([new Date(end - hours * HOUR), new Date(end)]);
  };

  const applyCustomRange = (range) => {
    validationError.value = rangeError(range, now());
    if (validationError.value) {
      resetCustomRange();
      return;
    }
    // Both the inputs and monitoring queries use second precision.
    if (!sameRange(range, appliedRange.value, 1000)) publishRange(range);
  };

  watch(modelValue, (range) => {
    if (rangeError(range, now()) || sameRange(range, appliedRange.value)) return;
    appliedRange.value = copyRange(range);
    validationError.value = '';
    if (currentDateRange.value !== 'custom') currentDateRange.value = presetForRange(range);
    resetCustomRange();
  }, { deep: true });

  if (!hasInitialRange) onUpdate(copyRange(appliedRange.value));

  return {
    applyCustomRange,
    currentDateRange,
    customDateRange,
    resetCustomRange,
    selectRange,
    validationError,
  };
}
