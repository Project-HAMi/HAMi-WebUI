import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';

const toFiniteValue = (value) => {
  if (
    value === null ||
    value === undefined ||
    (typeof value === 'string' && !value.trim())
  ) {
    return undefined;
  }
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
};

export const formatRankingValue = (value, unit = '%') => {
  const number = toFiniteValue(value) ?? 0;
  const rounded = Math.round((number + Number.EPSILON) * 100) / 100;
  const display = number !== 0 && rounded === 0
    ? number > 0 ? '<0.01' : '>-0.01'
    : String(rounded);
  const normalizedUnit = typeof unit === 'string' ? unit.trim() : '';
  return normalizedUnit ? `${display} ${normalizedUnit}` : display;
};

export const buildRankingItems = (rows = [], unit = '%') => {
  const displayUnit = unit ?? '%';
  const isPercent = displayUnit.trim() === '%';
  const values = rows.map((item) => Number(item.value) || 0);
  const maxValue = isPercent ? 100 : Math.max(...values, 0);

  return rows
    .slice()
    .sort((a, b) => Number(b.value) - Number(a.value))
    .map((item, index) => {
      const value = Number(item.value) || 0;
      const percentage = isPercent ? value : maxValue ? (value / maxValue) * 100 : 0;
      return {
        ...item,
        index: index + 1,
        percentage: Math.max(0, Math.min(100, percentage)),
        valueDisplay: formatRankingValue(item.value, displayUnit),
      };
    });
};

export const readRankingRows = (response, nameKey) => {
  if (!Array.isArray(response?.data)) {
    return { data: [], status: REQUEST_STATUS.INVALID };
  }

  const rows = response.data.flatMap((item) => {
    const value = toFiniteValue(item?.value);
    if (value === undefined) return [];
    return [{ name: item?.metric?.[nameKey] || '-', value }];
  });

  if (rows.length) return { data: rows, status: REQUEST_STATUS.READY };
  return {
    data: [],
    status: response.data.length
      ? REQUEST_STATUS.INVALID
      : REQUEST_STATUS.MISSING,
  };
};
