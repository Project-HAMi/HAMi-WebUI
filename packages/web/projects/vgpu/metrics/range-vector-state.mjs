export function toFiniteRangeValue(rawValue) {
  if (
    (typeof rawValue !== 'number' && typeof rawValue !== 'string') ||
    (typeof rawValue === 'string' && rawValue.trim() === '')
  ) {
    return null;
  }

  const value = Number(rawValue);
  return Number.isFinite(value) ? value : null;
}

export function normalizeRangePoint(point) {
  const isLegacyPair = Array.isArray(point);
  const rawTimestamp = isLegacyPair ? point[0] : point?.timestamp;
  const rawValue = isLegacyPair ? point[1] : point?.value;
  const timestamp = toFiniteRangeValue(rawTimestamp);
  const value =
    !isLegacyPair && point?.missing === true
      ? null
      : toFiniteRangeValue(rawValue);

  if (point && typeof point === 'object' && !isLegacyPair) {
    return { ...point, timestamp, value };
  }

  return { timestamp, value };
}

export function normalizeRangeValues(values) {
  return Array.isArray(values) ? values.map(normalizeRangePoint) : [];
}

export function normalizeRangeVectorResponse(response) {
  if (!response || !Array.isArray(response.data)) return response;

  return {
    ...response,
    data: response.data.map((stream) => ({
      ...stream,
      values: normalizeRangeValues(stream?.values),
    })),
  };
}

export function buildRangeLineData(values, { digits } = {}) {
  return normalizeRangeValues(values).map(({ value }) => {
    if (value === null) return null;
    return digits === undefined ? value : Number(value.toFixed(digits));
  });
}

export function buildRangeLineSeries(series, presentation) {
  return {
    ...series,
    data: buildRangeLineData(series?.data, presentation),
    type: 'line',
    connectNulls: false,
  };
}

export function formatRangeTooltipValue(
  rawValue,
  { digits = 1, unit = '', separator = '' } = {},
) {
  const value = toFiniteRangeValue(rawValue);
  return value === null ? '-' : `${value.toFixed(digits)}${separator}${unit}`;
}
