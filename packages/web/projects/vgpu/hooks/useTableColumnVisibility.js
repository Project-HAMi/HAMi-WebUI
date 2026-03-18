import { computed, ref, watch } from 'vue';

const normalizeCellValue = (value) => {
  if (value === undefined || value === null || value === '') return '--';
  if (Array.isArray(value) && value.length === 0) return '--';
  return value;
};

const getColKey = (col, index) => col.key || `${col.dataIndex || col.title || 'col'}__${index}`;

export default function useTableColumnVisibility(baseColumns) {
  const eyeColumnKeys = ref([]);

  const columnOptions = computed(() =>
    baseColumns.value.map((col, index) => ({
      label: String(col.title),
      value: getColKey(col, index),
    })),
  );

  const visibleColumns = computed(() =>
    baseColumns.value
      .map((col, index) => ({ col, index }))
      .filter(({ col, index }) => eyeColumnKeys.value.includes(getColKey(col, index)))
      .map(({ col, index }) => ({
        title: col.title,
        colKey: getColKey(col, index),
        minWidth: col.minWidth,
        width: col.width,
        cell: (_h, { row }) => {
          const rawValue = col.render ? col.render(row) : row[col.dataIndex];
          return normalizeCellValue(rawValue);
        },
      })),
  );

  watch(
    () => baseColumns.value,
    (val) => {
      const allKeys = val.map((col, index) => getColKey(col, index));
      eyeColumnKeys.value = eyeColumnKeys.value.length
        ? eyeColumnKeys.value.filter((key) => allKeys.includes(key))
        : allKeys;
      if (!eyeColumnKeys.value.length) eyeColumnKeys.value = allKeys;
    },
    { immediate: true },
  );

  return {
    eyeColumnKeys,
    columnOptions,
    visibleColumns,
  };
}
