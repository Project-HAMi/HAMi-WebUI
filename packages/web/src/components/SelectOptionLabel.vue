<template>
  <span class="select-option-label">
    <span class="select-option-label__line">
      <span class="select-option-label__main">{{ label }}</span>
      <template v-if="count !== undefined">
        <span class="select-option-label__count" :class="{ 'is-zero': count === 0 }" :title="countLabel" aria-hidden="true">{{ count }}</span>
        <span class="select-option-label__sr-only">{{ countLabel }}</span>
      </template>
    </span>
    <span v-if="shownHints.length" class="select-option-label__detail">
      <span v-for="hint in shownHints" :key="hint">{{ hint }}</span>
    </span>
  </span>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  label: { type: String, required: true },
  hints: { type: Array, default: () => [] },
  count: { type: Number, default: undefined },
  countLabel: { type: String, default: '' },
});
const shownHints = computed(() => props.hints.filter(Boolean));
</script>

<style scoped lang="scss">
.select-option-label {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  line-height: 20px;
}

.select-option-label__line,
.select-option-label__detail {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.select-option-label__main,
.select-option-label__detail > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.select-option-label__main {
  flex: 1 1 auto;
}

.select-option-label__count {
  flex: none;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;

  &.is-zero {
    color: var(--td-text-color-placeholder);
  }
}

.select-option-label__detail {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 16px;
}

.select-option-label__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
}
</style>
