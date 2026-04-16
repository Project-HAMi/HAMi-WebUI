<template>
  <svg viewBox="-3 0 160 144" class="workload-progress-ring">
    <path :d="backgroundPath" fill="none" stroke="#E4EBF1" stroke-width="18" />
    <path :d="progressPath" fill="none" stroke="#007BFF" stroke-width="18" />
  </svg>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps({
  percent: {
    type: Number,
    default: 0,
  },
});

const rx = 65;
const cx = 78;
const cy = 112;

const normalizedPercent = computed(() => {
  const val = Number(props.percent);
  if (!Number.isFinite(val)) return 0;
  return Math.max(0, Math.min(100, val));
});

const backgroundPath = computed(() => `M ${cx - rx},${cy} A ${rx},${rx} 0 0 1 ${cx + rx},${cy}`);

const progressPath = computed(() => {
  const angle = (normalizedPercent.value / 100) * 180;
  const rad = (angle * Math.PI) / 180;
  const x = cx + rx * Math.cos(Math.PI - rad);
  const y = cy - rx * Math.sin(Math.PI - rad);
  return `M ${cx - rx},${cy} A ${rx},${rx} 0 0 1 ${x},${y}`;
});
</script>

<style scoped>
.workload-progress-ring {
  width: 168px;
}
</style>
