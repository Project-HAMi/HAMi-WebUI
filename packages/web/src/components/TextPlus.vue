<template>
  <div class="text-plus">
    <component
      :is="to ? RouterLink : 'span'"
      :to="to || undefined"
      class="text-plus__value"
      :class="{ 'vgpu-resource-link': to }"
      :aria-label="to ? text : undefined"
    >
      <EllipsisText
        :text="text"
        :mode="mode"
        :tooltip-class="tooltipClass"
        :tooltip-append-to="tooltipAppendTo"
        :tooltip="tooltip"
      />
    </component>
  </div>
</template>

<script setup>
import { RouterLink } from 'vue-router';
import EllipsisText from './EllipsisText.vue';

defineProps({
  text: String,
  to: null,
  mode: {
    type: String,
    default: 'middle',
  },
  tooltip: { type: String, default: 'always' },
  tooltipAppendTo: Object,
  tooltipClass: {
    type: String,
    default: 'vgpu-long-text-tooltip',
  },
});
</script>

<style lang="scss">
.text-plus {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  min-width: 0;

  &__value {
    flex: 1;
    min-width: 0;
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: inline-flex;
    align-items: last baseline;
  }

  .vgpu-resource-link .ellipsis-text {
    text-decoration: underline;
    text-decoration-color: var(--resource-link-underline);
    text-underline-offset: 3px;
  }

  span.text-plus__value { color: #324558; }
}
</style>
