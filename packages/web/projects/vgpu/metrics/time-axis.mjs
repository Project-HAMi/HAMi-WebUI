import { timeParse } from '../../../src/utils/index.js';

const asDate = (value) => {
  if (value === null || value === undefined || value === '') return null;
  const timestamp = Number(value);
  const date = new Date(timestamp);
  return Number.isFinite(timestamp) && Number.isFinite(date.getTime()) ? date : null;
};

const utcOffset = (date) => {
  const minutes = -date.getTimezoneOffset();
  const hours = String(Math.floor(Math.abs(minutes) / 60)).padStart(2, '0');
  const remainder = String(Math.abs(minutes) % 60).padStart(2, '0');
  return `${minutes < 0 ? '-' : '+'}${hours}:${remainder}`;
};

export const formatTimeAxisTooltip = (value) => {
  const date = asDate(value);
  return date ? `${timeParse(date)} UTC${utcOffset(date)}` : null;
};

export const buildTimeAxisLabels = (timestamps, interval = 60_000) => {
  const dates = timestamps.map(asDate).filter(Boolean);
  const spansDays = new Set(dates.map((date) => timeParse(date, 'YYYY-MM-DD'))).size > 1;
  const spansYears = new Set(dates.map((date) => date.getFullYear())).size > 1;
  const spansOffsets = new Set(dates.map((date) => date.getTimezoneOffset())).size > 1;
  return {
    multiline: spansOffsets && interval < 86_400_000,
    format(value) {
      const date = asDate(value);
      if (!date) return '-';
      // Format the generated tick, not the sampling interval. Daily ticks stay
      // compact, while zooming into the same range reveals hours and seconds.
      const midnight = date.getHours() === 0 && date.getMinutes() === 0 && date.getSeconds() === 0;
      if (interval >= 86_400_000 || (spansDays && midnight)) {
        return timeParse(date, spansYears ? 'YYYY-MM-DD' : 'MM-DD');
      }
      const clock = timeParse(date, interval < 60_000 ? 'HH:mm:ss' : 'HH:mm');
      return spansOffsets ? `${clock}\nUTC${utcOffset(date)}` : clock;
    },
  };
};

const DAY = 86_400_000;
const INTERVALS = [
  ...[1, 2, 5, 10, 15, 30].map((step) => ({ unit: 'clock', step: step * 1000, duration: step * 1000 })),
  ...[1, 2, 5, 10, 15, 20, 30].map((step) => ({ unit: 'clock', step: step * 60_000, duration: step * 60_000 })),
  ...[1, 2, 3, 6, 12].map((step) => ({ unit: 'clock', step: step * 3_600_000, duration: step * 3_600_000 })),
  ...[1, 2, 3, 5, 7, 10, 14].map((step) => ({ unit: 'day', step, duration: step * DAY })),
  ...[1, 2, 3, 6].map((step) => ({ unit: 'month', step, duration: step * 30 * DAY })),
  ...[1, 2, 5, 10, 20, 50, 100].map((step) => ({ unit: 'year', step, duration: step * 365 * DAY })),
];

const naturalTicks = (min, max, interval) => {
  const date = new Date(min);
  let cursor;
  let toTimestamp;
  if (interval.unit === 'clock') {
    // Step in elapsed time so a repeated DST hour includes both UTC offsets.
    const offset = date.getTimezoneOffset() * 60_000;
    cursor = Math.ceil((min - offset) / interval.step) * interval.step + offset;
    toTimestamp = (value) => value;
  } else if (interval.unit === 'day') {
    cursor = Math.floor(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / DAY / interval.step) * interval.step;
    toTimestamp = (value) => {
      const civil = new Date(value * DAY);
      return new Date(civil.getUTCFullYear(), civil.getUTCMonth(), civil.getUTCDate()).getTime();
    };
  } else if (interval.unit === 'month') {
    cursor = Math.floor((date.getFullYear() * 12 + date.getMonth()) / interval.step) * interval.step;
    toTimestamp = (value) => new Date(Math.floor(value / 12), value % 12, 1).getTime();
  } else {
    cursor = Math.floor(date.getFullYear() / interval.step) * interval.step;
    toTimestamp = (value) => new Date(value, 0, 1).getTime();
  }
  const ticks = [];
  // Normally fewer than twelve ticks. Bound the loop for invalid/extreme input.
  for (let index = 0; index < 100; index += 1, cursor += interval.step) {
    const timestamp = toTimestamp(cursor);
    if (!Number.isFinite(timestamp) || timestamp > max) break;
    if (timestamp >= min) ticks.push(timestamp);
  }
  return ticks;
};

const POINT_SIZE = 4;

const layoutPointSymbols = (series, min, max, plotWidth) => {
  if (series.type !== 'line') return series;
  const values = series.data.map((point) => Array.isArray(point) ? point : point.value);
  const visible = values.map(([timestamp, value]) =>
    Number.isFinite(timestamp) && Number.isFinite(value) && timestamp >= min && timestamp <= max,
  );
  const marked = new Set();
  let segmentStart = -1;
  for (let index = 0; index < values.length; index += 1) {
    if (!visible[index]) continue;
    if (segmentStart < 0) segmentStart = index;
    if (visible[index + 1]) continue;
    const span = Math.abs(values[index][0] - values[segmentStart][0]) / (max - min) * plotWidth;
    // Keep the last real sample visible when a whole segment is too short to
    // read as a line. A lone report is the same case with a zero-length span.
    if (span <= POINT_SIZE) marked.add(index);
    segmentStart = -1;
  }
  return {
    ...series,
    showSymbol: true,
    symbol: 'emptyCircle',
    symbolSize: 6,
    emphasis: { ...series.emphasis, itemStyle: { ...series.emphasis?.itemStyle, opacity: 1 } },
    data: values.map((value, index) => ({
      value,
      symbol: 'circle',
      symbolSize: POINT_SIZE,
      // Keep geometry for hover; use small filled points for sparse reports.
      itemStyle: { opacity: marked.has(index) ? 1 : 0 },
    })),
  };
};

/** Lay out the visible time window independently of the sample timestamps. */
export const layoutTimeSeriesOptions = (option, width = 600, visibleExtent) => {
  const axis = option?.xAxis;
  if (axis?.type !== 'time') return option;
  const showLegend = option.legend?.show !== false;
  const verticalLegend = width < 360;
  const legendRows = showLegend ? (verticalLegend ? option.series.length : 1) : 0;
  // The series labels and card width are already known while samples load.
  // Reserve the same legend geometry without inventing a time extent or ticks.
  const frame = {
    ...option,
    legend: {
      ...option.legend,
      orient: verticalLegend ? 'vertical' : 'horizontal',
      textStyle: { ...option.legend?.textStyle, width: verticalLegend ? Math.max(40, width - 40) : null, overflow: 'truncate' },
      tooltip: { show: true },
    },
    grid: { ...option.grid, bottom: 30 + legendRows * 20 },
  };
  const [min, max] = visibleExtent ?? [axis.min, axis.max];
  if (!asDate(min) || !asDate(max) || max <= min) return frame;
  const plotWidth = Math.max(1, width * 0.93 - 10);
  const target = (max - min) / Math.max(2, Math.min(10, Math.floor(plotWidth / 90)));
  const interval = INTERVALS.reduce((best, item) =>
    Math.abs(Math.log(item.duration / target)) < Math.abs(Math.log(best.duration / target)) ? item : best,
  );
  const labels = buildTimeAxisLabels([min, max], interval.duration);
  const candidates = naturalTicks(min, max, interval);
  const interiorTicks = candidates.filter((value) => {
    // Do not clip a naturally occurring tick close to a canvas edge, and do not
    // replace it with the arbitrary query boundary to force an endpoint label.
    const halfWidth = Math.max(...labels.format(value).split('\n').map((line) => line.length)) * 3.6;
    const x = width * 0.07 + (value - min) / (max - min) * plotWidth;
    return x >= halfWidth && x + halfWidth <= width;
  });
  // A one-second query may have only boundary ticks. Let ECharts' outer-bounds
  // layout make room for those labels instead of leaving the axis blank.
  const ticks = interiorTicks.length ? interiorTicks : candidates;
  return {
    ...frame,
    series: option.series.map((series) => layoutPointSymbols(series, min, max, plotWidth)),
    grid: { ...frame.grid, bottom: frame.grid.bottom + (labels.multiline ? 16 : 0) },
    xAxis: {
      ...axis,
      axisTick: { ...axis.axisTick, customValues: ticks },
      axisLabel: { ...axis.axisLabel, customValues: ticks, formatter: labels.format },
    },
  };
};
