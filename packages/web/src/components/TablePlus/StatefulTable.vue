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
      <div
        v-for="row in skeletonRows"
        :key="row"
        class="stateful-table__skeleton-row"
        :class="{ 'stateful-table__skeleton-row--header': row === 1 }"
        :style="skeletonGridStyle"
        aria-hidden="true"
      >
        <t-skeleton
          v-for="column in normalizedColumnCount"
          :key="column"
          animation="gradient"
          :row-col="[{ width: column === 1 ? '72%' : '58%', height: row === 1 ? '14px' : '18px' }]"
        />
      </div>
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
        class="stateful-table__notice"
        data-testid="stateful-table-refreshing"
        role="status"
      >
        <t-loading size="small" />
        <span>{{ $t('common.refreshing') }}</span>
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
        <el-empty :description="$t('common.noData')" />
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
import { REQUEST_STATUS } from '@/hooks/request-state.mjs';

const props = defineProps({
  columnCount: {
    type: Number,
    default: 4,
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

defineEmits(['retry']);

const { t } = useI18n();
const skeletonRows = 6;
const normalizedColumnCount = computed(() => Math.max(1, props.columnCount));
const skeletonGridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${normalizedColumnCount.value}, minmax(0, 1fr))`,
}));
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
  min-height: 360px;
  overflow: hidden;
  border-radius: 8px;
}

.stateful-table__skeleton-row {
  display: grid;
  min-width: 720px;
  align-items: center;
  gap: 24px;
  min-height: 64px;
  padding: 0 16px;
}

.stateful-table__skeleton-row:nth-child(odd) {
  background: #fafbfc;
}

.stateful-table__skeleton-row--header {
  min-height: 48px;
  background: #f5f7fa;
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
