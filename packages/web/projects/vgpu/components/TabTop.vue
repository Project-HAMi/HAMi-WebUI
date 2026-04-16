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

    <div class="tab-top-list">
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
            <span class="tab-top-name" :title="item.name">
              {{ item.name }}
            </span>
            <span class="tab-top-value">
              {{ item.valueDisplay }}
            </span>
          </div>
          <t-progress
            theme="line"
            :percentage="item.percentage"
            :label="false"
            track-color="#e5e7eb"
            color="#5b8ff9"
          />
        </div>
      </div>
      <div v-if="!displayItems.length" class="tab-top-empty">
        {{ t('common.noData') }}
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

const props = defineProps({
  title: String,
  itemKey: String,
  config: Array,
  onClick: Function,
});
const { t } = useI18n();
const currentConfig = ref(props.config);
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

const formatValue = (value, unit) => {
  const num = Number(value) || 0;
  const fixed = num.toFixed(0);
  const unitText = unit && unit.trim() ? ` ${unit.trim()}` : '';
  return `${fixed}${unitText}`;
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
    .map((item, index) => ({
      ...item,
      index: index + 1,
      percentage: getPercentage(item.value),
      valueDisplay: formatValue(item.value, unit),
    }));
});

const handleItemClick = (item) => {
  handleClick({ data: item });
};

const fetchData = (configList) => {
  if (!configList?.length) return;
  tabActive.value = configList[0].key;
  configList.forEach((v, i) => {
    cardApi
      .getInstantVector({
        query: v.query,
      })
      .then((res) => {
        configList[i].data = res.data.map((item) => ({
          name: item.metric[v.nameKey],
          value: item.value,
        }));
      });
  });
};

onMounted(() => {
  fetchData(currentConfig.value);
});

watch(
  () => props.config,
  (val) => {
    currentConfig.value = cloneDeep(val);
    fetchData(currentConfig.value);
  },
  { deep: true },
);
</script>

<style lang="scss" scoped>
:deep(.home-block-header) {
  padding-bottom: 10px;
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
</style>
