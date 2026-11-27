<template>
  <t-tooltip :visible="showTooltip" :disabled="disabled" placement="top" :overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE">
    <template #content>{{ text }}</template>
    <button ref="trigger" v-bind="$attrs" type="button"
      @mouseenter="show" @mouseleave="hide"
      @focus="show" @blur="hide" @click="hide">
      <slot />
    </button>
  </t-tooltip>
</template>

<script setup>
import { computed, inject, onBeforeUnmount, ref, watch } from 'vue';
import { LONG_TEXT_TOOLTIP_STYLE } from '../../components/tooltip-policy.mjs';

defineOptions({ inheritAttrs: false });
const props = defineProps({ text: { type: String, required: true }, disabled: Boolean });
const activeTooltip = inject('deviceUsageTooltip');
const trigger = ref(null);
const showTooltip = computed(() => !props.disabled && activeTooltip.value === trigger.value && trigger.value !== null);
function show() {
  if (!props.disabled) activeTooltip.value = trigger.value;
}
function hide() {
  if (activeTooltip.value === trigger.value) activeTooltip.value = null;
}
watch(() => props.disabled, disabled => { if (disabled) hide(); });
onBeforeUnmount(hide);
</script>
