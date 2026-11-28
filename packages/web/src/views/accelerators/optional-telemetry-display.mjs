export const formatOptionalTelemetry = (value, unit) => {
  if (value === undefined || value === null || value === '') return '--';

  const numericValue = Number(value);
  if (!Number.isFinite(numericValue)) return '--';

  const rounded = Number(numericValue.toFixed(1));
  return `${rounded === 0 ? 0 : rounded} ${unit}`;
};
