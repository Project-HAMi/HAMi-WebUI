<template>
  <div class="gauge-card" :aria-busy="status === 'loading'">
    <div class="gauge-card__title">
      <span>{{ title }}</span>
      <t-tooltip
        v-if="description"
        :content="description"
        :visible="helpVisible"
        :overlay-inner-style="LONG_TEXT_TOOLTIP_STYLE"
      >
        <button
          type="button"
          class="gauge-card__help"
          :aria-label="metricHelpLabel || title"
          :aria-describedby="helpDescriptionId"
          @mouseenter="helpHovered = true"
          @mouseleave="helpHovered = false"
          @focus="helpFocused = true"
          @keydown.esc.stop="helpHovered = helpFocused = false"
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
      <t-skeleton
        v-if="status === 'loading'"
        animation="gradient"
        :row-col="[{ width: '44%', height: '28px' }]"
        aria-hidden="true"
      />
      <template v-else-if="isReady">
        <span class="gauge-card__reading">
          <span class="gauge-card__number">{{ numericPercent.toFixed(1) }}</span>
          <span class="gauge-card__unit">{{ gaugeUnit || '%' }}</span>
        </span>
        <span v-if="detailMode !== 'value'" class="gauge-card__aside">
          <b>{{ amount.used }}</b> / {{ amount.total }}<template v-if="amount.unit">&nbsp;{{ amount.unit }}</template>
        </span>
      </template>
      <template v-else>
        <span class="gauge-card__number gauge-card__number--empty">—</span>
        <span :class="status === 'missing' ? 'gauge-card__sr-only' : 'gauge-card__aside'">{{ $t(stateTextKey) }}</span>
      </template>
    </div>
    <t-skeleton
      v-if="status === 'loading'"
      animation="gradient"
      :row-col="[{ width: '100%', height: '4px' }]"
      class="gauge-card__progress gauge-card__progress-skeleton"
      aria-hidden="true"
    />
    <t-progress
      v-else-if="showProgress"
      theme="line"
      :percentage="isReady ? Math.min(100, Math.max(0, numericPercent)) : 0"
      :color="progressColor"
      :label="false"
      track-color="#e5e7eb"
      class="gauge-card__progress"
      aria-hidden="true"
    />
    <span
      v-if="status === 'loading'"
      class="gauge-card__sr-only"
      role="status"
    >
      {{ $t('common.loading') }}
    </span>
  </div>
</template>

<script setup>
import { computed, ref, useId } from 'vue';
import { HelpCircleIcon } from 'tdesign-icons-vue-next';
import { LONG_TEXT_TOOLTIP_STYLE } from './tooltip-policy.mjs';
import { formatAmountPair } from '../metrics/amount-format.mjs';

const props = defineProps({
  title: { type: String, required: true },
  description: { type: String, default: '' },
  metricHelpLabel: { type: String, default: '' },
  detailMode: { type: String, default: 'ratio' },
  kind: { type: String, default: 'allocation' },
  displayDivisor: { type: Number, default: 1 },
  status: { type: String, default: 'loading' },
  total: { type: [Number, String], default: 0 },
  used: { type: [Number, String], default: 0 },
  unit: { type: String, default: '' },
  gaugeUnit: { type: String, default: '%' },
  percent: { type: [Number, String], default: 0 },
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
const STATE_TEXT_KEYS = {
  error: 'dashboard.metricQueryFailed',
  invalid: 'dashboard.metricInvalid',
  'no-capacity': 'dashboard.metricNoCapacity',
};
const stateTextKey = computed(() => STATE_TEXT_KEYS[props.status] || 'dashboard.metricNoData');
const amount = computed(() => formatAmountPair(
  Number(props.used || 0) / props.displayDivisor,
  Number(props.total || 0) / props.displayDivisor,
  props.unit,
));

// One neutral color; only an allocation close to full asks for attention.
const progressColor = computed(() => (
  props.kind === 'allocation' && Number(props.percent) >= 80 ? '#DC2626' : '#2563EB'
));
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
    min-width: 0;
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

  // The denominator qualifies the number, so it shares the number's row.
  &__value {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: space-between;
    gap: 2px 8px;
    margin-bottom: 10px;
  }

  &__aside {
    font-size: 11px;
    line-height: 16px;
    color: #697886;
    white-space: nowrap;

    b {
      color: #1d2b3a;
      font-weight: 500;
    }
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

  // Pinned to the bottom, so a taller neighbor never leaves a gap under the bar.
  &__progress {
    margin-top: auto;

    // TDesign keeps an empty label with an 8px margin when label is false.
    :deep(.t-progress__info) {
      display: none;
    }
  }

  &__progress-skeleton {
    min-height: 4px;
  }

}
</style>
