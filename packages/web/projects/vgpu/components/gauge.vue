<template>
  <div class="gauge-card">
    <div class="gauge-card__title">
      <span>{{ title }}</span>
      <t-tooltip
        v-if="description"
        :content="description"
        :visible="helpVisible"
      >
        <button
          type="button"
          class="gauge-card__help"
          :aria-label="metricHelpLabel || title"
          :aria-describedby="helpDescriptionId"
          @mouseenter="helpHovered = true"
          @mouseleave="helpHovered = false"
          @focus="helpFocused = true"
          @blur="helpFocused = false"
        >
          <help-circle-icon aria-hidden="true" />
        </button>
      </t-tooltip>
      <span
        v-if="description"
        :id="helpDescriptionId"
        class="gauge-card__sr-only"
      >
        {{ description }}
      </span>
    </div>
    <div class="gauge-card__value">
      <template v-if="isReady">
        <span class="gauge-card__number">{{ numericPercent.toFixed(1) }}</span>
        <span class="gauge-card__unit">{{ gaugeUnit || '%' }}</span>
      </template>
      <span v-else class="gauge-card__number gauge-card__number--empty">—</span>
    </div>
    <t-progress
      v-if="showProgress && isReady"
      theme="line"
      :percentage="Math.min(100, Math.max(0, numericPercent))"
      :color="progressColor"
      :label="false"
      track-color="#e5e7eb"
      class="gauge-card__progress"
      aria-hidden="true"
    />
    <div class="gauge-card__detail" v-if="!hideInfo">
      <template v-if="isReady">
        <span>{{ detailLabel }}</span>
        <span>: </span>
        <b v-if="detailMode === 'value'">{{
          displayUsed.toFixed(usedPrecision)
        }}</b>
        <b v-else>
          {{ displayUsed.toFixed(usedPrecision) }} /
          {{ displayTotal.toFixed(totalPrecision) }}
        </b>
        <span v-if="unit" class="gauge-card__detail-unit"
          >&nbsp;{{ unit }}</span
        >
      </template>
      <span v-else-if="status === 'error'">{{
        $t('dashboard.metricQueryFailed')
      }}</span>
      <span v-else-if="status === 'loading'">{{ $t('common.loading') }}</span>
      <span v-else-if="status === 'invalid'">{{
        $t('dashboard.metricInvalid')
      }}</span>
      <span v-else-if="status === 'no-capacity'">{{
        $t('dashboard.metricNoCapacity')
      }}</span>
      <span v-else>{{ $t('dashboard.metricNoData') }}</span>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, useId } from 'vue';
import { HelpCircleIcon } from 'tdesign-icons-vue-next';

const props = defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  metricHelpLabel: { type: String, default: '' },
  detailLabel: { type: String, default: '' },
  detailMode: { type: String, default: 'ratio' },
  kind: { type: String, default: 'allocation' },
  displayDivisor: { type: Number, default: 1 },
  usedPrecision: { type: Number, default: 1 },
  totalPrecision: { type: Number, default: 0 },
  status: { type: String, default: 'loading' },
  total: { type: [Number, String], default: 0 },
  used: { type: [Number, String], default: 0 },
  unit: { type: String, default: '' },
  gaugeUnit: { type: String, default: '%' },
  percent: { type: [Number, String], default: 0 },
  hideInfo: { type: Boolean, default: false },
  showProgress: { type: Boolean, default: true },
});

const helpDescriptionId = useId();
const helpHovered = ref(false);
const helpFocused = ref(false);
const helpVisible = computed(() => helpHovered.value || helpFocused.value);
const showProgress = computed(() => props.showProgress !== false);
const numericPercent = computed(() => Number(props.percent));
const isReady = computed(
  () => props.status === 'ready' && Number.isFinite(numericPercent.value),
);
const displayUsed = computed(
  () => Number(props.used || 0) / props.displayDivisor,
);
const displayTotal = computed(
  () => Number(props.total || 0) / props.displayDivisor,
);

const progressColor = computed(() => {
  if (props.kind === 'utilization') return '#2563EB';

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
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: #939ea9;
    margin-bottom: 8px;
    line-height: 20px;
  }

  &__help {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: 0;
    color: #939ea9;
    background: transparent;
    cursor: help;

    svg {
      width: 14px;
      height: 14px;
    }

    &:hover {
      color: var(--el-color-primary);
    }

    &:focus-visible {
      border-radius: 4px;
      outline: 2px solid var(--el-color-primary);
      outline-offset: 1px;
    }
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

  &__value {
    margin-bottom: 10px;
  }

  &__number {
    font-size: 22px;
    font-weight: 600;
    color: #1d2b3a;
    line-height: 28px;
  }

  &__number--empty {
    color: #939ea9;
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
