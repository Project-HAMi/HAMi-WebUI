<template>
  <span id="NumberInput">
    <el-input-number v-model="value" v-bind="$attrs" :disabled="disabled" /><span
      class="unit"
      v-if="unit"
      >{{ unit }}</span
    >
  </span>
</template>

<script setup>
import { defineProps, defineEmits, computed, watchEffect } from 'vue';

const props = defineProps({
  modelValue: Number,
  unit: String,
  ratio: Number,
  disabled: Boolean,
});

const emits = defineEmits(['update:modelValue']);

const value = computed({
  get() {
    if (props.ratio) {
      return Math.ceil(props.modelValue / props.ratio);
    }
    return props.modelValue || 0;
  },
  set(val) {
    let newVal = val;
    if (props.ratio) {
      newVal = val * props.ratio;
    }
    emits('update:modelValue', newVal);
  },
});
</script>

<style lang="scss" scoped>
#NumberInput {
  position: relative;
  .unit {
    margin: 0 5px;
  }
}
</style>
