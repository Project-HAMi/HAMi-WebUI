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
