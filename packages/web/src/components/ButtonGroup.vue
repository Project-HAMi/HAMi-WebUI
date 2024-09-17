<template>
  <el-space>
    <el-button
      v-for="{ name, type, icon, disabled, onClick } in currentData"
      :key="name"
      :type="type"
      :icon="icon"
      :disabled="disabled"
      @click="onClick(record)"
      >{{ name }}</el-button
    >
  </el-space>
</template>

<script setup lang="jsx">
import { defineProps, computed } from 'vue';
import { renderProps } from '@/utils';

const props = defineProps({
  items: {
    type: Array,
    default: () => [],
  },
  record: null,
});

const currentData = computed(() => {
  let actions = props.items.map((item) => ({
    ...item,
    hidden: renderProps(item.hidden, props.record),
    disabled: renderProps(item.disabled, props.record),
  }));

  return actions.filter((item) => !item.hidden);
});
</script>
