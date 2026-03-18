<template>
  <div class="gauge-card">
    <div class="gauge-card__title">{{ title }}</div>
    <div class="gauge-card__value">
      <span class="gauge-card__number">{{ percent.toFixed(1) }}</span>
      <span class="gauge-card__unit">{{ gaugeUnit || '%' }}</span>
    </div>
    <t-progress
      v-if="showProgress"
      theme="line"
      :percentage="Math.min(100, Math.max(0, Number(percent)))"
      :color="progressColor"
      :label="false"
      track-color="#e5e7eb"
      class="gauge-card__progress"
    />
    <div class="gauge-card__detail" v-if="!hideInfo">
      <span>{{ title.includes('使用') || title.includes('Usage') ? $t('dashboard.usage') : $t('dashboard.allocation') }}</span>
      <span v-if="unit && !title.includes('算力') && !title.includes('Compute')"> ({{ unit }})</span>
      <span> : </span>
      <b>{{ used.toFixed(1) }} / {{ total.toFixed() }}</b>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';

const props = defineProps([
  'title',
  'total',
  'used',
  'unit',
  'gaugeUnit',
  'percent',
  'hideInfo',
  'showProgress',
]);

const showProgress = computed(() => props.showProgress !== false);

const progressColor = computed(() => {
  const value = Number(props.percent);
  if (value < 50) return '#16A34A';
  if (value < 80) return '#2563EB';
  return '#DC2626';
});
</script>

<style lang="scss" scoped>
.gauge-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  padding: 12px;
  background: #f5f7fa;
  border-radius: 8px;

  &__title {
    font-size: 12px;
    color: #939ea9;
    margin-bottom: 8px;
    line-height: 20px;
  }

  &__value {
    margin-bottom: 10px;
  }

  &__number {
    font-size: 22px;
    font-weight: 600;
    color: #1d2b3a;
    line-height: 28px;
  }

  &__unit {
    font-size: 12px;
    color: #697886;
    margin-left: 2px;
  }

  &__progress {
    margin-bottom: 8px;
  }

  &__detail {
    font-size: 11px;
    color: #697886;
    line-height: 16px;

    b {
      color: #1d2b3a;
      font-weight: 500;
    }
  }
}
</style>
