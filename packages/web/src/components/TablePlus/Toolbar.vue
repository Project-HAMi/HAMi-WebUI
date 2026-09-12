<template>
  <div class="table-toolbar">
    <div class="table-toolbar-left">
      <slot />
    </div>
    <div class="table-toolbar-right">
      <t-popup trigger="click" placement="bottom-right">
        <t-button size="medium" variant="outline" theme="default">
          <template #icon>
            <svg-icon icon="eye" />
          </template>
        </t-button>
        <template #content>
          <div class="table-toolbar-column-popup" :style="{ minWidth: popupMinWidth }">
            <t-checkbox-group v-model="innerValue">
              <t-space direction="vertical" :size="8">
                <t-checkbox
                  v-for="item in columnOptions"
                  :key="item.value"
                  :value="item.value"
                >
                  {{ item.label }}
                </t-checkbox>
              </t-space>
            </t-checkbox-group>
          </div>
        </template>
      </t-popup>
      <t-button
        size="medium"
        variant="outline"
        theme="default"
        :aria-busy="refreshing ? 'true' : 'false'"
        @click="$emit('refresh')"
      >
        <template #icon>
          <refresh-icon
            class="table-toolbar-refresh-icon"
            :class="{ 'is-refreshing': refreshing }"
          />
        </template>
      </t-button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { RefreshIcon } from 'tdesign-icons-vue-next';

const props = defineProps({
  modelValue: {
    type: Array,
    default: () => [],
  },
  columnOptions: {
    type: Array,
    default: () => [],
  },
  popupMinWidth: {
    type: String,
    default: '220px',
  },
  refreshing: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(['update:modelValue', 'refresh']);

const innerValue = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
});
</script>

<style scoped lang="scss">
.table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.table-toolbar-left {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
}

.table-toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.table-toolbar-column-popup {
  max-height: 320px;
  overflow: auto;
  padding: 12px 14px;
}

.table-toolbar-refresh-icon.is-refreshing {
  color: var(--td-brand-color);
  animation: table-toolbar-refresh 900ms linear infinite;
}

@keyframes table-toolbar-refresh {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .table-toolbar-refresh-icon.is-refreshing {
    animation: none;
  }
}

:deep(.table-toolbar-left .t-input) {
  height: 36px;
  font: var(--td-font-body-medium);
  border-radius: 6px !important;
  border-color: #e4ebf1 !important;
}

:deep(.table-toolbar-left .t-input:hover),
:deep(.table-toolbar-left .t-select:hover .t-input),
:deep(.table-toolbar-left .t-input--focused),
:deep(.table-toolbar-left .t-is-focused .t-input) {
  border-color: #2563eb !important;
  box-shadow: none !important;
}

:deep(.table-toolbar .t-button) {
  height: 36px;
  border-radius: 6px;
  font: var(--td-font-body-medium);
}

:deep(.table-toolbar-right .t-button) {
  width: 38px;
  min-width: 38px;
  padding-left: 0;
  padding-right: 0;
}
</style>
