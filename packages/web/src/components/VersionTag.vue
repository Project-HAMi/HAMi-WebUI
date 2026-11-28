<template>
  <t-tooltip
    v-model:visible="hintOpen"
    :disabled="!showFullVersion"
    trigger="hover"
    placement="bottom-left"
    theme="light"
    :show-arrow="false"
    overlay-class-name="about-version-hint"
    attach=".t-dialog__ctx:has(.about-dialog)"
  >
    <template #content><span role="tooltip">{{ version }}</span></template>
    <component
      :is="showFullVersion ? 'button' : 'span'"
      ref="tag"
      :type="showFullVersion ? 'button' : undefined"
      class="version-tag"
      :aria-label="showFullVersion ? version : undefined"
      @focus="showFullVersion && (hintOpen = true)"
      @blur="hintOpen = false"
      @click="showFullVersion && (hintOpen = true)"
      @keydown.esc="onEscape"
    >{{ label }}</component>
  </t-tooltip>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { shortVersion } from './about-display.mjs';

const props = defineProps({ version: { type: String, required: true } });
const label = computed(() => shortVersion(props.version));
const tag = ref(null);
const clipped = ref(false);
const showFullVersion = computed(() => label.value !== props.version || clipped.value);

watch([tag, label], ([element], _, onCleanup) => {
  if (!element) return;
  const measure = () => { clipped.value = element.scrollWidth > element.clientWidth; };
  measure();
  const observer = new ResizeObserver(measure);
  observer.observe(element);
  onCleanup(() => observer.disconnect());
}, { flush: 'post' });
const hintOpen = ref(false);
function onEscape(event) {
  if (hintOpen.value) {
    event.stopPropagation();
    hintOpen.value = false;
  }
}
</script>

<style scoped>
.version-tag {
  display: inline-block;
  max-width: 100%;
  padding: 0 5px;
  border: 0;
  border-radius: 4px;
  background: #f1f4f7;
  color: #596e81;
  font: inherit;
  font-size: 10px;
  line-height: 16px;
  font-variant-numeric: tabular-nums;
  vertical-align: middle;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
button.version-tag { cursor: pointer; }
button.version-tag:hover, button.version-tag:focus-visible { background: #e5ebf1; color: #32475a; }
button.version-tag:focus-visible { outline: 2px solid var(--td-brand-color, #2563eb); outline-offset: 2px; }
/* Keep the overlay in the modal layer, outside the centered dialog and scrollable tabs. */
:global(.t-popup.about-version-hint .t-popup__content) {
  box-sizing: border-box;
  width: max-content;
  max-width: min(440px, calc(100vw - 24px));
  padding: 4px 8px;
  border: 1px solid #dce4ed;
  border-radius: 5px;
  background: #fff;
  box-shadow: 0 2px 6px rgb(35 53 77 / 8%);
  color: #405469;
  font-size: 11px;
  line-height: 16px;
  overflow-wrap: anywhere;
}
</style>
