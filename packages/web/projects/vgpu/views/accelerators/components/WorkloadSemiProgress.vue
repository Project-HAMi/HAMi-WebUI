<template>
  <svg viewBox="0 34 156 90" class="workload-progress-ring" aria-hidden="true">
    <path :d="backgroundPath" fill="none" stroke="#E4EBF1" stroke-width="18" stroke-linecap="round" />
    <path v-if="normalizedPercent > 0" :d="progressPath" fill="none" stroke="#007BFF" stroke-width="18" stroke-linecap="round" />
  </svg>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  percent: {
    type: Number,
    default: undefined,
  },
});

const rx = 65;
const cx = 78;
const cy = 112;

const normalizedPercent = computed(() => {
  if (!Number.isFinite(props.percent)) return undefined;
  return Math.max(0, Math.min(100, props.percent));
});

const backgroundPath = computed(() => `M ${cx - rx},${cy} A ${rx},${rx} 0 0 1 ${cx + rx},${cy}`);

const progressPath = computed(() => {
  if (normalizedPercent.value === undefined) return '';
  const angle = (normalizedPercent.value / 100) * 180;
  const rad = (angle * Math.PI) / 180;
  const x = cx + rx * Math.cos(Math.PI - rad);
  const y = cy - rx * Math.sin(Math.PI - rad);
  return `M ${cx - rx},${cy} A ${rx},${rx} 0 0 1 ${x},${y}`;
});
</script>

<style scoped>
.workload-progress-ring {
  display: block;
  width: 100%;
}
</style>
