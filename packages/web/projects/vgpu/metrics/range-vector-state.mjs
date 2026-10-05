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

// Values are drawn as read; precision belongs to the tooltip, where rounding cannot
// flatten a small value onto zero.
export function buildRangeLineData(values) {
  return normalizeRangeValues(values).map(({ value }) => value);
}

export function buildRangeLineSeries(series) {
  return {
    ...series,
    data: buildRangeLineData(series?.data),
    type: 'line',
    connectNulls: false,
  };
}

export function buildRangeDataZoom() {
  return [
    {
      type: 'inside',
      xAxisIndex: 0,
      // Keep only visible samples. With 'none', ECharts collapses every sample
      // onto one x coordinate when a category axis is zoomed to a single slot.
      filterMode: 'filter',
    },
  ];
}

export function formatRangeTooltipValue(
  rawValue,
  { digits = 1, unit = '', separator = '' } = {},
) {
  const value = toFiniteRangeValue(rawValue);
  if (value === null) return '-';
  const step = 10 ** -digits;
  // A non-zero value below the shown precision is not a zero.
  if (value !== 0 && Math.abs(value) < step / 2) {
    return `${value > 0 ? '<' : '>-'}${step.toFixed(digits)}${separator}${unit}`;
  }
  return `${value.toFixed(digits)}${separator}${unit}`;
}
