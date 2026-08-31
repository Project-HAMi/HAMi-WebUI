<template>
  <t-tooltip :content="description" :visible="visible">
    <button
      type="button"
      class="metric-help"
      :aria-label="helpLabel"
      :aria-describedby="descriptionId"
      @mouseenter="hovered = true"
      @mouseleave="hovered = false"
      @focus="focused = true"
      @blur="focused = false"
      @keydown.esc="dismiss"
    >
      <help-circle-icon aria-hidden="true" />
    </button>
  </t-tooltip>
  <span :id="descriptionId" class="metric-help__description">
    {{ description }}
  </span>
</template>

<script setup>
import { computed, ref, useId } from 'vue';
import { HelpCircleIcon } from 'tdesign-icons-vue-next';

defineProps({
  description: { type: String, required: true },
  helpLabel: { type: String, required: true },
});

const descriptionId = useId();
const hovered = ref(false);
const focused = ref(false);
const visible = computed(() => hovered.value || focused.value);

const dismiss = () => {
  hovered.value = false;
  focused.value = false;
};
</script>

<style lang="scss" scoped>
.metric-help {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: 0;
  color: #939ea9;
  background: transparent;
  cursor: help;

  svg {
    width: 14px;
    height: 14px;
  }

  &:hover {
    color: var(--el-color-primary);
  }

  &:focus-visible {
    border-radius: 4px;
    outline: 2px solid var(--el-color-primary);
    outline-offset: 1px;
  }

  &__description {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
}
</style>
