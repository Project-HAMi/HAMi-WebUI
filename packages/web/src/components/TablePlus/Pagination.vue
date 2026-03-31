<template>
  <div class="table-plus-pagination">
    <div class="table-plus-pagination-total">{{ totalText }}</div>
    <t-pagination
      class="pagination"
      :total="total"
      :current="current"
      :page-size="pageSize"
      :page-size-options="normalizedPageSizes"
      :show-jumper="showJumper"
      :show-page-size="true"
      @change="handleChange"
    />
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = defineProps({
  total: {
    type: Number,
    default: 0,
  },
  current: {
    type: Number,
    default: 1,
  },
  pageSize: {
    type: Number,
    default: 10,
  },
  pageSizes: {
    type: Array,
    default: () => [10, 20, 50, 100],
  },
  showJumper: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(['update:current', 'update:pageSize', 'change']);
const { t } = useI18n();

const totalText = computed(() => t('common.paginationTotal', { total: props.total }));
const normalizedPageSizes = computed(() =>
  (props.pageSizes || []).map((size) => ({
    value: Number(size),
    label: t('common.paginationPerPage', { size }),
  })),
);

const handleChange = (pageInfo = {}) => {
  const nextCurrent = Number(pageInfo.current || props.current || 1);
  const nextPageSize = Number(pageInfo.pageSize || props.pageSize || 10);
  emit('update:current', nextCurrent);
  emit('update:pageSize', nextPageSize);
  emit('change', { current: nextCurrent, pageSize: nextPageSize });
};
</script>

<style scoped lang="scss">
.table-plus-pagination {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 12px;

  .table-plus-pagination-total {
    color: #697886;
    font-size: 14px;
    line-height: 22px;
    white-space: nowrap;
  }

  :deep(.pagination) {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  :deep(.pagination .t-pagination__total) {
    display: none;
  }

  :deep(.pagination .t-pagination__size-swap) {
    margin-right: 0;
  }

  :deep(.pagination .t-pagination__item.t-is-current),
  :deep(.pagination .t-pagination__btn.t-is-current),
  :deep(.pagination .t-pagination__number.t-is-current),
  :deep(.pagination .t-pagination__number[aria-current='page']) {
    background: #eff6ff;
    color: #2563eb;
    border: none;
  }
}
</style>
