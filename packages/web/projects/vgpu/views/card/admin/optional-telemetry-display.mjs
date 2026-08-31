export const formatOptionalTelemetry = (value, unit) => {
  if (value === undefined || value === null || value === '') return '--';

  const numericValue = Number(value);
  if (!Number.isFinite(numericValue)) return '--';

  return `${numericValue} ${unit}`;
};
