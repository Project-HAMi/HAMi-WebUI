<template>
  <div
    ref="root"
    class="segmented-control"
    :class="[`segmented-control--${size}`, { 'is-equal': equal, 'is-ready': ready }]"
    role="group"
    :aria-label="ariaLabel"
  >
    <span
      class="segmented-control__indicator"
      :class="{ 'is-fading': fading }"
      :style="indicatorStyle"
      aria-hidden="true"
    />
    <button
      v-for="option in options"
      :key="String(option.value)"
      type="button"
      class="segmented-control__option"
      :class="{ 'is-active': option.value === modelValue }"
      :aria-pressed="option.value === modelValue"
      :aria-label="option.ariaLabel"
      @click="select(option.value)"
    >
      <!-- The space keeps the accessible name "Abnormal 3". -->
      <span class="segmented-control__label">{{ option.label }}</span> <span v-if="option.count !== undefined && option.count !== null" class="segmented-control__count" :class="{ 'is-danger': option.tone === 'danger' && option.count > 0 }">{{ option.count }}</span>
    </button>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';

const props = defineProps({
  modelValue: { type: [String, Number], default: '' },
  // [{ value, label, count?, tone?: 'danger', ariaLabel? }]
  options: { type: Array, default: () => [] },
  ariaLabel: { type: String, default: undefined },
  size: { type: String, default: 'medium', validator: (value) => ['small', 'medium'].includes(value) },
  equal: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'change']);

const root = ref(null);
const box = reactive({ x: 0, width: 0 });
const ready = ref(false);
const fading = ref(false);
let observer;

const reducedMotion = () => typeof window !== 'undefined'
  && typeof window.matchMedia === 'function'
  && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

const measure = () => {
  const index = props.options.findIndex((option) => option.value === props.modelValue);
  const button = index >= 0 ? root.value?.querySelectorAll('.segmented-control__option')[index] : null;
  box.x = button ? button.offsetLeft : 0;
  box.width = button ? button.offsetWidth : 0;
};

const indicatorStyle = computed(() => ({
  width: `${box.width}px`,
  transform: `translateX(${box.x}px)`,
  visibility: box.width ? 'visible' : 'hidden',
}));

const optionSignature = computed(() => JSON.stringify(
  props.options.map((option) => [option.value, option.label, option.count ?? null]),
));

watch(() => [props.modelValue, optionSignature.value], async ([value], previous) => {
  // Reduced motion fades instead of sliding.
  const dissolve = Boolean(previous) && value !== previous[0] && reducedMotion();
  if (dissolve) fading.value = true;
  await nextTick();
  measure();
  if (dissolve) requestAnimationFrame(() => { fading.value = false; });
});

onMounted(() => {
  measure();
  // Avoid sliding in from the left edge on mount.
  requestAnimationFrame(() => { ready.value = true; });
  if (typeof ResizeObserver === 'function') {
    observer = new ResizeObserver(measure);
    observer.observe(root.value);
  }
});
onBeforeUnmount(() => observer?.disconnect());

const select = (value) => {
  if (value === props.modelValue) return;
  emit('update:modelValue', value);
  emit('change', value);
};
</script>

<style lang="scss" scoped>
.segmented-control {
  position: relative;
  display: inline-flex;
  align-items: stretch;
  flex: none;
  padding: 3px;
  border-radius: 8px;
  background: #e7edf4;
  box-sizing: border-box;

  &--small {
    height: 30px;
  }

  &--medium {
    height: 36px;
  }
}

.segmented-control__indicator {
  position: absolute;
  top: 3px;
  bottom: 3px;
  left: 0;
  border-radius: 5px;
  background: #fff;
  box-shadow: 0 1px 3px rgb(32 48 64 / 12%), 0 0 0 0.5px rgb(32 48 64 / 4%);
  pointer-events: none;
}

.segmented-control.is-ready .segmented-control__indicator {
  transition:
    transform 240ms cubic-bezier(0.2, 0, 0, 1),
    width 240ms cubic-bezier(0.2, 0, 0, 1);
}

.segmented-control__option {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-width: 0;
  padding: 0 10px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: #526477;
  font: inherit;
  font-size: 13px;
  font-weight: 500;
  line-height: 1;
  white-space: nowrap;
  cursor: pointer;
  transition: color 160ms ease;

  &:hover {
    color: #203040;
  }

  &.is-active {
    color: #2563eb;
  }

  &:focus-visible {
    outline: 2px solid #2563eb;
    outline-offset: -1px;
  }
}

.segmented-control.is-equal .segmented-control__option {
  flex: 1 1 0;
  padding: 0;
}

.segmented-control__count {
  min-width: 16px;
  color: #8493a3;
  font-size: 12px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  text-align: center;
  transition: color 160ms ease;

  .is-active > & {
    color: #5b8def;
  }

  &.is-danger {
    color: #d54941;
  }
}

@media (prefers-reduced-motion: reduce) {
  .segmented-control.is-ready .segmented-control__indicator {
    transition: opacity 160ms ease-out;
  }

  .segmented-control__indicator.is-fading {
    opacity: 0;
    transition: none !important;
  }

  .segmented-control__option,
  .segmented-control__count {
    transition: none;
  }
}
</style>
