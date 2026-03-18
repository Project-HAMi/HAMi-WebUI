const getTrimValue = (value) => (typeof value === 'string' ? value.trim() : value);

export default function useTableFilters({ fetchTableData, resetBeforeApply } = {}) {
  const applyFilters = () => {
    if (typeof resetBeforeApply === 'function') {
      resetBeforeApply();
    }
    fetchTableData();
  };

  const refreshTable = () => {
    fetchTableData();
  };

  return {
    getTrimValue,
    applyFilters,
    refreshTable,
  };
}
