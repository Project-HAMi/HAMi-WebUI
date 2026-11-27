<template>
  <block-box :title="title">
    <template #extra>
      <SegmentedControl
        v-model="tabActive"
        size="small"
        class="tab-top-switch"
        :options="tabOptions"
        :aria-label="title"
      />
    </template>

    <div class="tab-top-list" :aria-busy="activeStatus === 'loading'">
      <template v-if="activeStatus === 'loading'">
        <div
          v-for="index in 5"
          :key="`skeleton-${index}`"
          class="tab-top-skeleton"
          aria-hidden="true"
        >
          <t-skeleton
            animation="gradient"
            :row-col="[
              [
                { type: 'rect', width: '24px', height: '24px' },
                { type: 'text', width: 'calc(100% - 34px)', height: '18px' },
              ],
              { type: 'rect', width: 'calc(100% - 34px)', height: '4px', marginLeft: '34px' },
            ]"
          />
        </div>
        <span class="tab-top-sr-only" role="status">{{ t('common.loading') }}</span>
      </template>
      <template v-else>
        <div
          v-for="item in displayItems"
          :key="item.physicalKey ?? item.name"
          class="tab-top-item"
          :class="{ 'is-static': item.detailTo === null }"
          @click="handleItemClick(item)"
        >
          <div class="tab-top-rank">
            {{ item.index }}
          </div>
          <div class="tab-top-content">
            <div class="tab-top-header">
              <slot name="item-name" :item="item" :tab-active="tabActive">
                <span class="tab-top-name" :title="item.name">
                  {{ item.name }}
                </span>
              </slot>
              <t-tooltip v-if="item.note" :content="item.note">
                <span class="tab-top-value">{{ item.valueDisplay }}</span>
              </t-tooltip>
              <span v-else class="tab-top-value">
                {{ item.valueDisplay }}
              </span>
            </div>
            <span v-if="item.note" class="tab-top-sr-only">{{ item.note }}</span>
            <t-progress
              theme="line"
              :percentage="item.percentage"
              :label="false"
              track-color="#e5e7eb"
              color="#5b8ff9"
            />
          </div>
        </div>
      </template>
      <div
        v-if="activeStatus !== 'loading' && !displayItems.length"
        class="tab-top-empty"
      >
        {{ activeStateText }}
      </div>
    </div>
  </block-box>
</template>

<script setup>
import BlockBox from '@/components/BlockBox.vue';
import SegmentedControl from '@/components/SegmentedControl/index.vue';
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';
import { formatRankingValue } from './tab-top-state.mjs';
import { isLowerBound, isNothingCounted, lowerBoundMessage, nothingCountedMessage } from '../metrics/uncounted.mjs';

const props = defineProps({
  title: String,
  itemKey: String,
  config: Array,
  onClick: Function,
});
const { t } = useI18n();
const tabActive = ref('');

watch(
  () => props.config,
  (list) => {
    if (!list?.length) return;
    if (!list.some((item) => item.key === tabActive.value)) {
      tabActive.value = list[0].key;
    }
  },
  { immediate: true },
);

const tabOptions = computed(() =>
  (props.config || []).map(({ tab, key }) => ({
    label: tab,
    value: key,
  })),
);

const handleClick = (params) => {
  if (props.onClick) {
    props.onClick({ ...params, tabActive: tabActive.value });
  }
};

const activeConfig = computed(() =>
  (props.config || []).find((item) => item.key === tabActive.value),
);

const displayItems = computed(() => {
  const config = activeConfig.value;
  const data = Array.isArray(config?.data) ? config.data : [];
  const unit = config?.unit || '%';

  if (!data.length) return [];

  const values = data.map((item) => Number(item.value) || 0);
  const isPercent = !config?.unit || config.unit.trim() === '%';
  const maxValue = isPercent ? 100 : Math.max(...values, 0);

  const getPercentage = (val) => {
    const num = Number(val) || 0;
    if (!maxValue) return 0;
    const percent = isPercent ? num : (num / maxValue) * 100;
    return Math.max(0, Math.min(100, percent));
  };

  return data
    .slice()
    .sort((a, b) => Number(b.value) - Number(a.value))
    .map((item, index) => {
      const uncounted = config?.uncounted === null ? null : config?.uncounted?.[item.name] || 0;
      const nothingCounted = isNothingCounted(item.value, uncounted);
      const lowerBound = isLowerBound(uncounted);
      return {
        ...item,
        index: index + 1,
        percentage: getPercentage(item.value),
        valueDisplay: nothingCounted ? '--' : `${lowerBound ? '≥' : ''}${formatRankingValue(item.value, unit)}`,
        note: nothingCounted ? nothingCountedMessage(t, uncounted) : lowerBound ? lowerBoundMessage(t, uncounted) : '',
      };
    });
});

const activeStatus = computed(
  () => activeConfig.value?.status || REQUEST_STATUS.LOADING,
);
const activeStateText = computed(() => {
  if (activeStatus.value === REQUEST_STATUS.ERROR) {
    return t('dashboard.metricQueryFailed');
  }
  if (activeStatus.value === REQUEST_STATUS.INVALID) {
    return t('dashboard.metricInvalid');
  }
  return t('common.noData');
});

const handleItemClick = (item) => {
  handleClick({ data: item });
};
</script>

<style lang="scss" scoped>
// In a narrow card the switch moves under the title as a whole instead of splitting either.
:deep(.home-block-header) {
  padding-bottom: 10px;
  flex-wrap: wrap;
  gap: 8px 12px;
}

:deep(.home-block-header .title) {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-top-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 0 2px;
  min-height: 230px;
}

.tab-top-item {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  padding: 6px 8px;
  border-radius: 6px;
  transition: background-color 0.15s ease;

  &:hover {
    background-color: transparent;
    box-shadow: none;
  }
}

.tab-top-rank {
  width: 24px;
  height: 24px;
  border-radius: 4px;
  background: #e4ebf1;
  color: #324558;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}

.tab-top-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tab-top-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 14px;
  color: #324558;
}

.tab-top-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 400;
  color: #324558;
}

.tab-top-value {
  font-weight: 500;
  flex-shrink: 0;
  font-size: 13px;
  color: #324558;
}

.tab-top-item.is-static {
  cursor: default;
}

.tab-top-item:not(.is-static):hover .tab-top-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #2563eb;
  text-decoration: underline;
  text-decoration-skip-ink: none;
  text-underline-offset: 4px;
}

.tab-top-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 0 8px;
  color: #9ca3af;
  font-size: 12px;
}

.tab-top-skeleton {
  min-height: 40px;
}

.tab-top-sr-only {
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
</style>
