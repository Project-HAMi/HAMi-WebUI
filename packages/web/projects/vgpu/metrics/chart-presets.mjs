import { layoutTimeSeriesOptions } from './time-axis.mjs';
import {
  buildRangeDataZoom,
  buildRangeLineSeries,
  normalizeRangeValues,
} from './range-vector-state.mjs';
import {
  buildPieTooltipFormatter,
  buildTimeSeriesTooltipFormatter,
} from './tooltip-html.mjs';
import { CHART_COLORS } from './chart-colors.mjs';

export { CHART_COLORS };

const AXIS_POINTER = Object.freeze({
  type: 'line',
  lineStyle: { type: 'dashed', color: '#8A8A8A' },
});

const LINE_WIDTH = 2;

/**
 * Options for a metric over time. `series` is `[{ name, data, color }]`, where
 * data is a range vector's points; the legend appears once there are two.
 * `digits` applies to the tooltip only; the lines keep the values as read.
 */
export const buildTimeSeriesOptions = ({
  series = [],
  unit = '%',
  digits = 2,
  animation = false,
} = {}) => {
  const normalized = series.map((item) => ({
    ...item,
    data: normalizeRangeValues(item?.data).map((point) =>
      point.timestamp !== null && Number.isFinite(new Date(point.timestamp).getTime())
        ? point
        : { ...point, timestamp: null, value: null },
    ),
  }));
  const timestamps = normalized.flatMap((item) => item.data.map((point) => point.timestamp)).filter((timestamp) => timestamp !== null);
  const extent = timestamps.reduce((range, timestamp) => [Math.min(range[0], timestamp), Math.max(range[1], timestamp)], [Infinity, -Infinity]);
  const showLegend = normalized.length > 1;
  // A lone report has no duration; keep a small window around that instant.
  if (timestamps.length && extent[0] === extent[1]) {
    extent[0] -= 30_000;
    extent[1] += 30_000;
  }

  return layoutTimeSeriesOptions({
    animation,
    // The legend sits at the bottom edge, right under the axis labels.
    legend: showLegend ? { bottom: 0, left: 'center' } : { show: false },
    tooltip: {
      trigger: 'axis',
      axisPointer: AXIS_POINTER,
      confine: true,
      formatter: buildTimeSeriesTooltipFormatter({
        digits,
        unit,
        fallbackColor: CHART_COLORS.single,
      }),
    },
    grid: {
      top: 12,
      bottom: showLegend ? 50 : 30,
      left: '7%',
      right: 10,
    },
    dataZoom: buildRangeDataZoom().map((zoom) => ({
      ...zoom,
      minValueSpan: timestamps.length ? Math.min(10_000, extent[1] - extent[0]) : 10_000,
    })),
    xAxis: {
      type: 'time',
      ...(timestamps.length ? { min: extent[0], max: extent[1] } : {}),
      axisLabel: {
        hideOverlap: true,
        lineHeight: 16,
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: (value) => (unit ? `${value} ${unit}` : `${value}`),
      },
    },
    series: normalized.map((item) => ({
      ...buildRangeLineSeries({
        name: item.name,
        data: item.data,
        itemStyle: {
          color: item.color || CHART_COLORS.single,
          borderColor: item.color || CHART_COLORS.single,
        },
        lineStyle: {
          width: LINE_WIDTH,
          color: item.color || CHART_COLORS.single,
        },
      }),
      // Each series keeps its own timestamps, including missing-value gaps.
      data: item.data.map(({ timestamp, value }) => [timestamp, value]),
    })),
  });
};

/** The shared shape of the donut charts: only their labels differ. */
export const buildDonutOptions = ({
  data = [],
  unit = '',
  showLabels = false,
  labelLayout,
} = {}) => ({
  animation: true,
  color: CHART_COLORS.categorical,
  tooltip: {
    trigger: 'item',
    confine: true,
    formatter: buildPieTooltipFormatter({ unit }),
  },
  series: [
    {
      type: 'pie',
      radius: ['48%', '72%'],
      center: ['50%', '50%'],
      avoidLabelOverlap: showLabels,
      itemStyle: {
        borderRadius: 6,
        borderColor: '#fff',
        borderWidth: 2,
      },
      label: showLabels
        ? {
            alignTo: 'edge',
            formatter: (params) => {
              const suffix = unit ? ` ${unit}` : '';
              return `{name|${params.name}}\n{cnt|${params.value}${suffix}}`;
            },
            minMargin: 8,
            edgeDistance: 8,
            lineHeight: 18,
            rich: {
              name: { fontSize: 12, color: '#324558', fontWeight: 500 },
              cnt: { fontSize: 11, color: '#697886' },
            },
          }
        : { show: false },
      labelLine: showLabels
        ? {
            length: 12,
            length2: 8,
            smooth: true,
            lineStyle: { width: 1, color: '#cbd5e1' },
          }
        : { show: false },
      emphasis: {
        scale: true,
        scaleSize: 4,
        itemStyle: {
          shadowBlur: 12,
          shadowOffsetX: 0,
          shadowColor: 'rgba(0, 0, 0, 0.15)',
        },
      },
      ...(labelLayout ? { labelLayout } : {}),
      data,
    },
  ],
});
