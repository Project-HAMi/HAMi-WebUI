<template>
  <section
    class="stateful-table"
    :aria-busy="isBusy ? 'true' : 'false'"
    :data-table-state="status"
  >
    <div
      v-if="status === REQUEST_STATUS.LOADING"
      class="stateful-table__skeleton"
      data-testid="stateful-table-skeleton"
    >
      <span class="stateful-table__sr-only" role="status">
        {{ $t('common.loading') }}
      </span>
      <t-table
        class="stateful-table__skeleton-table vgpu-table-skin"
        row-key="skeletonKey"
        :data="skeletonRows"
        :columns="skeletonColumns"
        :table-layout="tableLayout"
        aria-hidden="true"
        inert
      />
    </div>

    <div
      v-else-if="status !== REQUEST_STATUS.READY"
      class="stateful-table__feedback"
      data-testid="stateful-table-error"
      role="alert"
    >
      <el-empty :description="blockingMessage">
        <el-button
          type="primary"
          data-testid="stateful-table-retry"
          @click="$emit('retry')"
        >
          {{ $t('common.retry') }}
        </el-button>
      </el-empty>
    </div>

    <template v-else>
      <div
        v-if="refreshing"
        class="stateful-table__sr-only"
        data-testid="stateful-table-refreshing"
        role="status"
      >
        {{ $t('common.refreshing') }}
      </div>
      <div
        v-else-if="refreshError"
        class="stateful-table__notice stateful-table__notice--error"
        data-testid="stateful-table-refresh-error"
        role="alert"
      >
        <span>{{ $t('common.refreshFailedShowingPreviousResult') }}</span>
        <t-button size="small" variant="text" theme="primary" @click="$emit('retry')">
          {{ $t('common.retry') }}
        </t-button>
      </div>

      <div
        v-if="!hasRows"
        class="stateful-table__feedback stateful-table__feedback--empty"
        data-testid="stateful-table-empty"
        role="status"
      >
        <el-empty :description="$t(filtered ? 'common.noMatchingResults' : 'common.noData')">
          <t-button v-if="filtered" variant="outline" @mousedown.prevent @click="$emit('clearFilters')">
            {{ $t('common.clearFilters') }}
          </t-button>
        </el-empty>
      </div>
      <template v-else>
        <slot />
        <slot name="footer" />
      </template>
    </template>
  </section>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Skeleton } from 'tdesign-vue-next';
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

const props = defineProps({
  columnCount: {
    type: Number,
    default: 4,
  },
  filtered: {
    type: Boolean,
    default: false,
  },
  columns: {
    type: Array,
    default: () => [],
  },
  tableLayout: {
    type: String,
    default: 'auto',
  },
  hasRows: {
    type: Boolean,
    default: false,
  },
  refreshError: {
    type: [Object, String, Boolean],
    default: null,
  },
  refreshing: {
    type: Boolean,
    default: false,
  },
  status: {
    type: String,
    required: true,
  },
});

defineEmits(['retry', 'clearFilters']);

const { t } = useI18n();
// Sample rows reserve the usual row rhythm without claiming a result count.
const skeletonRows = Array.from({ length: 5 }, (_, skeletonKey) => ({ skeletonKey }));
const skeletonColumns = computed(() => {
  const columns = props.columns.length ? props.columns : Array.from(
    { length: Math.max(1, props.columnCount) },
    (_, index) => ({ colKey: `skeleton-${index}` }),
  );
  return columns.map((column, index) => ({
    colKey: column.colKey,
    width: column.width,
    minWidth: column.minWidth,
    // Headers and column geometry are known before any records arrive.
    title: column.title ?? ((h) => h(Skeleton, {
      animation: 'gradient',
      rowCol: [{ width: '58%', height: '14px' }],
    })),
    cell: (h) => h('div', { class: 'stateful-table__skeleton-cell' }, [
      h(Skeleton, {
        animation: 'gradient',
        rowCol: [{ width: index === 0 ? '72%' : '58%', height: '18px' }],
      }),
    ]),
  }));
});
const isBusy = computed(() => (
  props.status === REQUEST_STATUS.LOADING || props.refreshing
));
const blockingMessage = computed(() => (
  props.status === REQUEST_STATUS.INVALID
    ? t('common.invalidResponse')
    : t('common.loadFailed')
));
</script>

<style scoped lang="scss">
.stateful-table {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 8px;
}

.stateful-table__skeleton {
  min-width: 0;
  margin-top: 8px;
}

.stateful-table__skeleton-table {
  min-width: 0;
}

:deep(.stateful-table__skeleton-cell) {
  display: flex;
  min-height: 40px;
  align-items: center;

  > .t-skeleton {
    width: 100%;
  }
}

.stateful-table__feedback {
  display: flex;
  min-height: 360px;
  align-items: center;
  justify-content: center;
  border-radius: 8px;
  background: #fff;
}

.stateful-table__feedback--empty {
  min-height: 300px;
}

.stateful-table__notice {
  display: flex;
  min-height: 32px;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border-radius: 6px;
  background: #f5f7fa;
  color: #697886;
  font-size: 13px;
}

.stateful-table__notice--error {
  justify-content: space-between;
  background: #fff4f2;
  color: #c6473a;
}

.stateful-table__sr-only {
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
