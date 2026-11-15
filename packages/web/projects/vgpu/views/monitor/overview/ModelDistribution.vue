<template>
  <div
    class="model-distribution"
    :class="{ 'model-distribution--sparse': isSparse }"
    :role="isSparse ? 'region' : undefined"
    :aria-label="isSparse ? labels.listLabel : undefined"
    :aria-describedby="isSparse && partial ? coverageNoteId : undefined"
    :tabindex="isSparse ? 0 : undefined"
  >
    <span v-if="partial" :id="coverageNoteId" class="overview-sr-only">{{ labels.partial }}</span>
    <div v-if="rows.length === 1" class="model-distribution__single" :aria-label="labels.listLabel">
      <div class="model-distribution__name model-distribution__single-name">
        <ModelName :row="rows[0]" :unknown="labels.unknown" />
      </div>
      <div class="model-distribution__single-values">
        <span class="model-distribution__single-count">
          {{ formatCount(rows[0].count) }}<span v-if="labels.unit" class="model-distribution__unit">{{ labels.unit }}</span>
        </span>
        <span class="model-distribution__percent" :title="partial ? labels.partial : undefined">{{ formatShare(rows[0].share) }}</span>
      </div>
    </div>
    <ul
      v-else-if="isSparse"
      class="model-distribution__compact"
      :class="{ 'model-distribution__compact--four': rows.length === 4 }"
      :aria-label="labels.listLabel"
    >
      <li v-for="row in rows" :key="row.type === null ? 'unknown-model' : `model:${row.type}`">
        <div class="model-distribution__compact-heading">
          <span class="model-distribution__name">
            <ModelName :row="row" :unknown="labels.unknown" />
          </span>
          <span class="model-distribution__compact-values">
            <span class="model-distribution__number">
              {{ formatCount(row.count) }}<span v-if="labels.unit" class="model-distribution__unit">{{ labels.unit }}</span>
            </span>
            <span class="model-distribution__percent" :title="partial ? labels.partial : undefined">{{ formatShare(row.share) }}</span>
          </span>
        </div>
        <span class="model-distribution__track" aria-hidden="true">
          <span v-if="Number.isFinite(row.share)" :style="{ width: `${row.share * 100}%` }" />
        </span>
      </li>
    </ul>
    <div
      v-else
      class="model-distribution__scroll"
      role="region"
      :aria-label="labels.listLabel"
      tabindex="0"
    >
      <table class="model-distribution__table">
        <colgroup>
          <col class="model-distribution__model-col" />
          <col class="model-distribution__count-col" />
          <col class="model-distribution__share-col" />
        </colgroup>
        <thead>
          <tr>
            <th scope="col">{{ labels.model }}</th>
            <th scope="col" class="model-distribution__number">{{ labels.count }}</th>
            <th
              scope="col"
              class="model-distribution__number"
              :title="partial ? labels.partial : undefined"
              :aria-describedby="partial ? coverageNoteId : undefined"
            >{{ labels.share }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.type === null ? 'unknown-model' : `model:${row.type}`">
            <th scope="row" class="model-distribution__name">
              <ModelName :row="row" :unknown="labels.unknown" />
            </th>
            <td class="model-distribution__number">
              {{ formatCount(row.count) }}<span v-if="labels.unit" class="model-distribution__unit">{{ labels.unit }}</span>
            </td>
            <td class="model-distribution__share">
              <div class="model-distribution__share-value">
                <span class="model-distribution__track" aria-hidden="true">
                  <span v-if="Number.isFinite(row.share)" :style="{ width: `${row.share * 100}%` }" />
                </span>
                <span class="model-distribution__percent" :title="partial ? labels.partial : undefined">{{ formatShare(row.share) }}</span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup>
import { computed, useId } from 'vue';
import ModelName from './ModelName.vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  models: { type: Array, required: true },
  partial: Boolean,
  labels: { type: Object, required: true },
});
const { locale } = useI18n();
const coverageNoteId = useId();
const rows = computed(() => props.models);
// A short comparison benefits from full-width bars; denser inventories retain
// a table with aligned quantities and a bounded scroll area.
const isSparse = computed(() => rows.value.length > 0 && rows.value.length <= 4);
const countFormat = computed(() => new Intl.NumberFormat(locale.value));
// Significant digits keep a nonzero small category from displaying as 0%.
const shareFormat = computed(() => new Intl.NumberFormat(locale.value, {
  style: 'percent',
  maximumSignificantDigits: 3,
}));
const formatCount = (value) => countFormat.value.format(value);
const formatShare = (value) => Number.isFinite(value) ? shareFormat.value.format(value) : '—';
</script>

<style scoped lang="scss">
.model-distribution {
  container-type: inline-size;
  min-width: 0;
  height: var(--model-list-height, auto);

  &--sparse,
  &__scroll {
    scrollbar-width: thin;
    scrollbar-color: var(--app-scrollbar-thumb, #cbd5e1) transparent;

    @supports selector(::-webkit-scrollbar) {
      // Non-auto standard properties override the pixel-width scrollbar rules.
      scrollbar-width: auto;
      scrollbar-color: auto;

      &::-webkit-scrollbar {
        width: 4px;
        height: 4px;
      }

      &::-webkit-scrollbar-track { background: transparent; }

      &::-webkit-scrollbar-thumb {
        border: 0;
        background: var(--app-scrollbar-thumb, #cbd5e1);
        border-radius: 999px;
        box-shadow: none;
      }
    }
  }

  &--sparse {
    display: flex;
    flex-direction: column;
    justify-content: safe center;
    overflow-y: auto;
    border-radius: 4px;

    &:focus-visible {
      outline: 2px solid var(--el-color-primary);
      outline-offset: -2px;
    }
  }

  &__single {
    flex-shrink: 0;
    min-width: 0;
    color: #324558;
  }

  &__single &__single-name {
    padding-right: 0;
    text-align: center;
    font-size: 15px;
    line-height: 22px;
  }

  &__single-values {
    display: flex;
    align-items: baseline;
    justify-content: center;
    gap: 20px;
    margin-top: 12px;

    .model-distribution__percent {
      min-width: 0;
    }
  }

  &__single-count {
    color: #1d2b3a;
    font-size: 30px;
    line-height: 36px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;

    .model-distribution__unit {
      margin-left: 6px;
      font-weight: 400;
    }
  }

  &__compact {
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: 20px;
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: 13px;
    line-height: 20px;
    color: #324558;

    &--four { gap: 12px; }

    > li {
      min-width: 0;
    }

    .model-distribution__track {
      display: block;
      margin-top: 8px;
      height: 6px;
    }
  }

  &__compact-heading {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 16px;
  }

  &__compact-values {
    display: inline-flex;
    flex-shrink: 0;
    align-items: baseline;
    gap: 16px;
  }

  &__scroll {
    box-sizing: border-box;
    height: var(--model-list-height, auto);
    max-height: var(--model-list-max-height, 320px);
    padding-right: 12px;
    overflow-y: auto;
    scrollbar-gutter: stable;
    border-radius: 4px;

    &:focus-visible {
      outline: 2px solid var(--el-color-primary);
      outline-offset: -2px;
    }
  }

  &__table {
    width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
    font-size: 13px;
    line-height: 20px;
    color: #324558;

    :where(th, td) {
      padding: 4px 0;
      vertical-align: middle;
    }

    thead th {
      position: sticky;
      top: 0;
      z-index: 1;
      padding-top: 0;
      padding-bottom: 8px;
      background: #fff;
      border-bottom: 1px solid #e4ebf1;
      color: #697886;
      font-size: 12px;
      font-weight: 400;
      text-align: left;
    }

    tbody tr + tr {
      border-top: 1px solid #f0f3f6;
    }

    thead .model-distribution__number {
      text-align: right;
    }
  }

  &__count-col { width: 68px; }
  &__share-col { width: 34%; }

  &__name {
    padding-right: 12px;
    text-align: left;
    font-weight: 400;
    overflow-wrap: anywhere;

  }

  &__number {
    text-align: right;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  &__unit {
    margin-left: 4px;
    color: #697886;
    font-size: 12px;
  }

  &__share {
    padding-left: 20px;
  }

  &__share-value {
    display: grid;
    grid-template-columns: minmax(0, 1fr) max-content;
    align-items: center;
    gap: 10px;
  }

  &__track {
    height: 5px;
    overflow: hidden;
    border-radius: 3px;
    background: #edf1f5;

    > span {
      display: block;
      height: 100%;
      border-radius: inherit;
      background: var(--el-color-primary);
    }
  }

  &__percent {
    min-width: 46px;
    text-align: right;
    white-space: nowrap;
    color: #697886;
    font-size: 12px;
    font-variant-numeric: tabular-nums;
  }
}

@container (max-width: 420px) {
  .model-distribution__compact-heading {
    flex-wrap: wrap;
    gap: 4px 12px;
  }
  .model-distribution__compact-values {
    margin-left: auto;
  }
  .model-distribution__count-col { width: 54px; }
  .model-distribution__share-col { width: 78px; }
  .model-distribution__share { padding-left: 16px; }
  .model-distribution__share-value {
    grid-template-columns: minmax(0, 1fr);
    gap: 4px;
  }
  .model-distribution__percent { grid-row: 1; }
}
</style>
