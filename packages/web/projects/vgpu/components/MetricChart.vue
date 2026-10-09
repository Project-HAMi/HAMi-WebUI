<template>
  <div class="metric-chart" :aria-busy="isLoading || refreshing">
    <!-- One live region for both, so a missing line and a failed refresh are announced once each. -->
    <div v-if="note || refreshError" class="metric-chart__status" role="status">
      <span v-if="note" class="metric-chart__note">{{ note }}</span>
      <span v-if="refreshError" class="metric-chart__refresh metric-chart__refresh--error">
        {{ $t('common.refreshFailedShowingPreviousResult') }}
      </span>
    </div>

    <div ref="bodyRef" class="metric-chart__body" :style="{ height: `${height}px` }">
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
        ref="chartRef"
        :option="displayOption"
        :autoresize="true"
        class="metric-chart__canvas"
        @datazoom="updateZoom"
      />
      <div v-else class="metric-chart__state">
        <span>{{ stateText }}</span>
        <slot name="action" />
      </div>
      <!-- Over the previous chart while it refreshes: blocks hover and zoom at once, shows itself after a delay. -->
      <div
        v-if="blocking"
        class="metric-chart__updating"
        :class="{ 'is-visible': updatingVisible }"
        :role="updatingVisible ? 'status' : undefined"
      >
        <div class="metric-chart__plot" :style="plotInsets">
          <t-loading v-if="updatingVisible" size="small" :text="$t('common.refreshing')" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import VChart from 'vue-echarts';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import { layoutTimeSeriesOptions } from '../metrics/time-axis.mjs';

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

const bodyRef = ref(null);
const chartWidth = ref(600);
const zoomRange = ref([0, 100]);
const displayOption = computed(() => {
  const axis = props.option?.xAxis;
  if (axis?.type !== 'time') return props.option;
  const span = axis.max - axis.min;
  const extent = zoomRange.value.map((percent) => axis.min + span * percent / 100);
  return layoutTimeSeriesOptions(props.option, chartWidth.value, extent);
});
const updateZoom = () => {
  const zoom = chartRef.value?.getOption()?.dataZoom?.[0];
  if (zoom) zoomRange.value = [zoom.start, zoom.end];
};
// Card width changes with both the viewport and the sidebar. Updating only the
// ticks keeps the current zoom while adapting their density to available space.
watch(bodyRef, (element, _, onCleanup) => {
  if (!element) return;
  const observer = new ResizeObserver(([entry]) => {
    chartWidth.value = entry.contentRect.width;
  });
  observer.observe(element);
  onCleanup(() => observer.disconnect());
});

// The indicator centres on the plotting area, not on the axis labels and legend around it.
const plotInsets = computed(() => {
  const grid = displayOption.value?.grid;
  if (!grid || Array.isArray(grid)) return undefined;
  const inset = (value) => (typeof value === 'number' ? `${value}px` : value ?? 0);
  return { top: inset(grid.top), right: inset(grid.right), bottom: inset(grid.bottom), left: inset(grid.left) };
});

// Quick refreshes finish before this, so they never flash the indicator.
const UPDATING_DELAY_MS = 250;
const chartRef = ref(null);
watch(chartRef, (chart) => {
  if (!chart) zoomRange.value = [0, 100];
});
const blocking = computed(() => props.refreshing && !isLoading.value);
const updatingVisible = ref(false);
let updatingTimer;
const clearUpdatingTimer = () => {
  clearTimeout(updatingTimer);
  updatingTimer = undefined;
};
watch(blocking, (active) => {
  clearUpdatingTimer();
  updatingVisible.value = false;
  if (!active) return;
  chartRef.value?.chart?.dispatchAction({ type: 'hideTip' });
  updatingTimer = setTimeout(() => {
    updatingVisible.value = true;
  }, UPDATING_DELAY_MS);
}, { immediate: true });
onBeforeUnmount(clearUpdatingTimer);
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

  &__updating {
    position: absolute;
    inset: 0;
    z-index: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 4px;

    &.is-visible {
      background: rgba(255, 255, 255, 0.6);
    }
  }

  &__plot {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
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
