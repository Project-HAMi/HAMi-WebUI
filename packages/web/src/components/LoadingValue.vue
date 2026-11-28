<template>
  <t-skeleton
    animation="gradient"
    :row-col="[{ width: '100%', height: `${height}px` }]"
    class="loading-value"
    :style="{ width, height: `${height}px` }"
    aria-hidden="true"
  />
</template>

<script setup>
defineProps({
  height: { type: Number, default: 20 },
  width: { type: String, default: '100%' },
});
</script>

<style scoped>
.loading-value {
  font: inherit;
  display: block;
  max-width: 100%;
  min-width: 0;
}
.loading-value :deep(.t-skeleton__row) {
  margin: 0;
}
</style>
