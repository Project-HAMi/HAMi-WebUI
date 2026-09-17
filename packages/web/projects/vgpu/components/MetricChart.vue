<template>
  <div class="metric-chart" :aria-busy="isLoading || refreshing">
    <div v-if="note || refreshing || refreshError" class="metric-chart__status">
      <span v-if="note" class="metric-chart__note">{{ note }}</span>
      <span v-if="refreshing" class="metric-chart__refresh" role="status">
        {{ $t('common.loading') }}
      </span>
      <span
        v-else-if="refreshError"
        class="metric-chart__refresh metric-chart__refresh--error"
        role="status"
      >
        {{ $t('common.refreshFailedShowingPreviousResult') }}
      </span>
    </div>

    <div class="metric-chart__body" :style="{ height: `${height}px` }">
      <template v-if="isLoading">
        <t-skeleton
          animation="gradient"
          :row-col="skeletonRows"
          class="metric-chart__skeleton"
          aria-hidden="true"
        />
        <span class="metric-chart__sr-only" role="status">{{ $t('common.loading') }}</span>
      </template>
      <VChart
        v-else-if="isReady"
        :option="option"
        :autoresize="true"
        class="metric-chart__canvas"
      />
      <div v-else class="metric-chart__state">
        <span>{{ stateText }}</span>
        <slot name="action" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import VChart from 'vue-echarts';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

const props = defineProps({
  status: { type: String, default: REQUEST_STATUS.LOADING },
  option: { type: Object, default: () => ({}) },
  height: { type: Number, default: 250 },
  // Why the chart is empty, or what its values leave out. Kept above the
  // chart: a badge floating over the plot hid data and matched nothing else.
  note: { type: String, default: '' },
  stateText: { type: String, default: '' },
  refreshing: { type: Boolean, default: false },
  refreshError: { type: [Boolean, Object], default: false },
});

const isLoading = computed(() => props.status === REQUEST_STATUS.LOADING);
const isReady = computed(() => props.status === REQUEST_STATUS.READY);
const skeletonRows = computed(() => [
  { width: '100%', height: `${Math.max(props.height - 50, 80)}px` },
  { width: '42%', height: '16px', margin: '16px auto 0' },
]);
</script>

<style lang="scss" scoped>
.metric-chart {
  &__status {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: 4px;
    margin-bottom: 8px;
    font-size: 12px;
    line-height: 18px;
  }

  &__note {
    color: #697886;
  }

  &__refresh {
    margin-left: auto;
    color: #697886;

    &--error {
      color: #d54941;
    }
  }

  &__body {
    position: relative;
    min-height: 0;
  }

  &__canvas {
    height: 100%;
  }

  &__skeleton {
    padding-top: 12px;
  }

  &__state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    height: 100%;
    color: #939ea9;
    font-size: 12px;
    text-align: center;
  }

  &__sr-only {
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
