<template>
  <el-slider v-model="value" v-bind="props" @change="changeValue"/>
</template>

<script setup>
import { defineProps, defineEmits, ref } from 'vue';

const value = ref(0)

const props = defineProps({
  modelValue: Number,
  min: Number,
  max: Number,
  step: Number,
  showInput: {
    Type: Boolean,
    default() {
        return true
      }
  }
});

const emits = defineEmits(['update:modelValue']);

const changeValue = () => {
  emits('update:modelValue', value.value);
}
</script>

<style lang="scss" scoped>
#NumberInput {
  position: relative;
  .unit {
    margin: 0 5px;
  }
}
</style>
