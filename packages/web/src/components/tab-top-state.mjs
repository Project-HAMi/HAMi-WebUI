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
