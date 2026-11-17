<template>
  <el-tooltip
    v-model:visible="hintOpen"
    :disabled="!showFullVersion"
    ref="tooltip"
    :content="version"
    :trigger="['hover', 'focus']"
    placement="bottom-start"
    effect="light"
    :show-arrow="false"
    :offset="4"
    popper-class="about-version-hint"
    append-to=".t-dialog__ctx:has(.about-dialog)"
    :popper-options="placementOptions"
  >
    <component
      :is="showFullVersion ? 'button' : 'span'"
      ref="tag"
      :type="showFullVersion ? 'button' : undefined"
      class="version-tag"
      :aria-label="showFullVersion ? version : undefined"
      @click="showFullVersion && tooltip?.onOpen()"
      @keydown.esc="onEscape"
    >{{ label }}</component>
  </el-tooltip>
</template>

<script setup>
import { computed, ref, watch } from 'vue';
import { shortVersion } from './about-display.mjs';

const placementOptions = {
  strategy: 'fixed',
  modifiers: [{ name: 'preventOverflow', options: { padding: 12 } }],
};
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
const tooltip = ref(null);
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
button.version-tag:focus-visible { outline: 2px solid var(--td-brand-color); outline-offset: 2px; }
/* Keep the overlay in the modal layer, outside the centered dialog and scrollable tabs. */
:global(.el-popper.about-version-hint) {
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
