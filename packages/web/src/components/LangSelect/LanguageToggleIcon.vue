<template>
  <Translate1Icon
    class="language-toggle-icon"
    :class="{ 'is-english': english }"
    :stroke-width="1.6"
    aria-hidden="true"
  />
</template>

<script setup>
import { Translate1Icon } from 'tdesign-icons-vue-next';

defineProps({ english: { type: Boolean, default: false } });
</script>

<style scoped>
.language-toggle-icon {
  display: block;
  flex-shrink: 0;
}

.language-toggle-icon :deep(path) {
  transition: transform 160ms ease, opacity 160ms ease;
}

.language-toggle-icon :deep(path:nth-of-type(2)) {
  opacity: 0.6;
}

/* Keep TDesign's original glyphs. They have different heights, so align the
   leading glyph to y=3 and the trailing glyph to y=21 in either language.
   This keeps the artwork's outer bounds fixed when their positions switch. */
.is-english :deep(path:first-of-type) {
  transform: translate(9px, 6.1402px);
  opacity: 0.6;
}

.is-english :deep(path:nth-of-type(2)) {
  transform: translate(-9px, -8.5px);
  opacity: 1;
}

@media (prefers-reduced-motion: reduce) {
  .language-toggle-icon :deep(path) {
    transition: none;
  }
}
</style>
