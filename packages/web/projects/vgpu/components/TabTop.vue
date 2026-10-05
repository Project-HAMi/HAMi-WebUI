<template>
  <block-box :title="title">
    <template #extra>
      <t-radio-group
        v-model="tabActive"
        theme="button"
        variant="outline"
        size="small"
        class="tab-top-radio"
        :options="radioOptions"
      >
      </t-radio-group>
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
          :key="item.name"
          class="tab-top-item"
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
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import cardApi from '~/vgpu/api/card';
import { cloneDeep } from 'lodash';
import {
  createRequestState,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from '@/hooks/request-state.mjs';
import {
  formatRankingValue,
  readRankingRows,
  readUncountedRows,
} from './tab-top-state.mjs';
import { isLowerBound, isNothingCounted, lowerBoundMessage, nothingCountedMessage } from '../metrics/uncounted.mjs';

const props = defineProps({
  title: String,
  itemKey: String,
  config: Array,
  onClick: Function,
});
const { t } = useI18n();
const createStatefulConfigs = (configs = []) =>
  cloneDeep(configs).map((item) => ({
    ...item,
    ...createRequestState(Array.isArray(item.data) ? item.data : []),
  }));
const currentConfig = ref(createStatefulConfigs(props.config));
const tabActive = ref('');

const radioOptions = computed(() =>
  (currentConfig.value || []).map(({ tab, key }) => ({
    label: tab,
    value: key,
  })),
);

const handleClick = (params) => {
  if (props.onClick) {
    props.onClick({ ...params, tabActive: tabActive.value });
  }
};

const displayItems = computed(() => {
  const config = currentConfig.value.find(
    (item) => item.key === tabActive.value,
  );
  const data = cloneDeep(config?.data) || [];
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

const activeConfig = computed(() =>
  currentConfig.value.find((item) => item.key === tabActive.value),
);
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

const fetchData = (configList) => {
  if (!configList?.length) return;
  if (!configList.some((item) => item.key === tabActive.value)) {
    tabActive.value = configList[0].key;
  }
  configList.forEach((v, i) => {
    const state = configList[i];
    const hasResolved = state.hasResolved;
    const requestId = startRequest(state, { hasResolved });
    // Rows whose rate leaves allocations out read as lower bounds, and every row
    // does when that count cannot be read; the list waits for it either way.
    const uncounted = v.uncountedQuery
      ? cardApi.getInstantVector({ query: v.uncountedQuery }).then(
        (res) => readUncountedRows(res, v.nameKey),
        () => null,
      )
      : Promise.resolve({});
    cardApi.getInstantVector({ query: v.query }).then(
      async (res) => {
        const result = readRankingRows(res, v.nameKey);
        const counts = await uncounted;
        if (requestId === state.requestId) state.uncounted = counts;
        resolveRequest(state, {
          ...result,
          requestId,
        });
      },
      (error) => rejectRequest(state, error, { hasResolved, requestId }),
    );
  });
};

onMounted(() => {
  fetchData(currentConfig.value);
});

watch(
  () => props.config,
  (val) => {
    const previous = new Map(
      currentConfig.value.map((item) => [`${item.key}:${item.query}`, item]),
    );
    currentConfig.value = createStatefulConfigs(val).map((item) => {
      const old = previous.get(`${item.key}:${item.query}`);
      if (!old) return item;
      return {
        ...item,
        data: old.data,
        uncounted: old.uncounted,
        status: old.status,
        hasResolved: old.hasResolved,
        refreshing: old.refreshing,
        error: old.error,
        refreshError: old.refreshError,
      };
    });
    fetchData(currentConfig.value);
  },
  { deep: true },
);
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

:deep(.tab-top-radio) {
  flex-wrap: nowrap;
  white-space: nowrap;
}

:deep(.tab-top-radio.t-radio-group__outline .t-radio-button) {
  padding: 12px;
  height: 32px;
  font-size: 12px;
  box-sizing: border-box;
}

:deep(.tab-top-radio.t-radio-group__outline .t-radio-button:first-child) {
  border-radius: 6px 0 0 6px;
}

:deep(.tab-top-radio.t-radio-group__outline .t-radio-button:last-child) {
  border-radius: 0 6px 6px 0;
}

:deep(.tab-top-radio.t-radio-group__outline .t-radio-button:only-child) {
  border-radius: 6px;
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

.tab-top-item:hover .tab-top-name {
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
