<template>
  <t-button
    class="refresh-button"
    :class="{ 'refresh-button--compact': compact }"
    variant="outline"
    theme="default"
    shape="square"
    :disabled="disabled"
    :aria-label="label"
    :aria-busy="refreshing"
    :title="title || label"
  >
    <template #icon>
      <refresh-icon
        class="refresh-button__icon"
        :class="{ 'is-refreshing': refreshing }"
        aria-hidden="true"
      />
    </template>
  </t-button>
</template>

<script setup>
import { RefreshIcon } from 'tdesign-icons-vue-next';

defineProps({
  label: { type: String, required: true },
  title: { type: String, default: '' },
  refreshing: Boolean,
  disabled: Boolean,
  compact: Boolean,
});
</script>

<style scoped lang="scss">
.refresh-button {
  width: 38px;
  min-width: 38px;
  height: 36px;
  padding: 0;
  border-radius: 6px;
  font: var(--td-font-body-medium);

  &--compact {
    width: 32px;
    min-width: 32px;
    height: 32px;
  }

  &:not(:disabled):hover {
    border-color: var(--td-brand-color);
    color: var(--td-brand-color);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

.refresh-button__icon.is-refreshing {
  color: var(--td-brand-color);
  animation: refresh-button-spin 900ms linear infinite;
}

@keyframes refresh-button-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .refresh-button__icon.is-refreshing {
    animation: none;
  }
}
</style>
