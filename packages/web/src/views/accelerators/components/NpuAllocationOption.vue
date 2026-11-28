<template>
  <div class="npu-option">
    <div class="npu-option-header">
      <div class="npu-option-icon"><svg-icon :icon="option.whole ? 'vgpu-card' : 'vgpu-core'" aria-hidden="true" /></div>
      <div class="npu-option-header-info">
        <div class="npu-option-name">{{ option.whole ? $t('card.deviceConfig.wholeCard') : option.name }}</div>
        <div class="npu-option-sub-title">
          {{ option.computeShare === undefined
            ? $t('card.deviceConfig.shareUnknown')
            : $t('card.deviceConfig.computeShareOf', { share: option.computeShare }) }}
        </div>
      </div>
    </div>
    <div class="npu-option-rows">
      <div v-for="row in rows" :key="row.key" class="npu-option-row">
        <span class="npu-option-label">{{ row.label }}</span>
        <span class="npu-option-value">
          <span class="npu-option-metric">{{ row.used }}</span>
          <span class="npu-option-sep">/</span>
          <span class="npu-option-total">{{ row.total }}</span>
          <t-progress
            v-if="row.percent !== undefined"
            theme="circle"
            size="24"
            :percentage="row.percent"
            :color="SHARE_COLOR"
            :label="false"
          />
        </span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { roundToDecimal } from '@/utils';

const SHARE_COLOR = '#2563EB';

const props = defineProps({
  option: { type: Object, required: true },
});
const { t } = useI18n();

const text = (value) => (value === undefined ? '--' : `${value}`);
const gib = (mib) => (mib === undefined ? '--' : `${roundToDecimal(mib / 1024, 2)} GiB`);
const rows = computed(() => {
  const { aiCore, aiCpu, memory } = props.option;
  return [
    { key: 'core', label: t('card.deviceConfig.aiCore'), used: text(aiCore.used), total: text(aiCore.total), percent: aiCore.percent },
    { key: 'cpu', label: t('card.deviceConfig.aiCpu'), used: text(aiCpu.used), total: text(aiCpu.total), percent: aiCpu.percent },
    { key: 'memory', label: t('card.deviceConfig.memory'), used: gib(memory.used), total: gib(memory.total), percent: memory.percent },
  ];
});
</script>

<style lang="scss" scoped>
.npu-option {
  display: flex;
  flex-direction: column;
  gap: 15px;
  min-width: 0;
  padding: 15px 20px;
  border-radius: 8px;
  background: #f5f7fa;
}

.npu-option-header {
  display: flex;
  align-items: center;
  gap: 20px;
}

.npu-option-icon {
  display: flex;
  flex: 0 0 40px;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 4px 10px rgb(2 5 8 / 6%);
  font-size: 20px;
}

.npu-option-header-info {
  min-width: 0;
}

.npu-option-name {
  overflow: hidden;
  color: #1d2b3a;
  font-size: 16px;
  font-weight: 500;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.npu-option-sub-title {
  margin-top: 4px;
  color: #939ea9;
  font-size: 12px;
  line-height: 20px;
}

.npu-option-rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.npu-option-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 6px;
  background: #fff;
  box-shadow: 0 4px 10px rgb(2 5 8 / 6%);
}

.npu-option-label {
  color: #939ea9;
  font-size: 12px;
  line-height: 20px;
  white-space: nowrap;
}

.npu-option-value {
  display: flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.npu-option-metric,
.npu-option-total {
  font-size: 14px;
  font-weight: 500;
}

.npu-option-metric {
  color: #324558;
}

.npu-option-total {
  color: #939ea9;
}

.npu-option-sep {
  color: #b6c2cd;
  font-size: 12px;
}
</style>
