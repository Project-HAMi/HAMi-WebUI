<template>
  <t-tooltip :content="$t('card.unconfiguredTip')" :overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE">
    <t-tag
      class="unconfigured-tag"
      theme="warning"
      variant="light"
      size="small"
      tabindex="0"
      :aria-label="`${$t('card.unconfigured')}: ${$t('card.unconfiguredTip')}`"
    >
      {{ $t('card.unconfigured') }}
    </t-tag>
  </t-tooltip>
</template>

<script setup>
import { LONG_TEXT_TOOLTIP_STYLE } from '~/vgpu/components/tooltip-policy.mjs';
</script>

<style lang="scss" scoped>
.unconfigured-tag {
  flex-shrink: 0;
  cursor: help;
}
</style>
