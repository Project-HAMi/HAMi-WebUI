import { computed, reactive } from 'vue';

export default function useLocalPagination(tableData, initial = {}) {
  const pagination = reactive({
    current: 1,
    pageSize: 10,
    total: 0,
    showJumper: false,
    pageSizeOptions: [5, 10, 20, 50, 100],
    ...initial,
  });

  const pagedTableData = computed(() => {
    const start = (pagination.current - 1) * pagination.pageSize;
    const end = start + pagination.pageSize;
    return tableData.value.slice(start, end);
  });

  const syncTotalAndClamp = () => {
    pagination.total = tableData.value.length;
    const maxPage = Math.max(1, Math.ceil(pagination.total / pagination.pageSize));
    if (pagination.current > maxPage) pagination.current = maxPage;
  };

  const resetToFirstPage = () => {
    pagination.current = 1;
  };

  return {
    pagination,
    pagedTableData,
    syncTotalAndClamp,
    resetToFirstPage,
  };
}
