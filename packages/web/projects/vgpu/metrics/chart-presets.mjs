import { timeParse } from '@/utils';
import {
  buildRangeDataZoom,
  buildRangeLineSeries,
  normalizeRangeValues,
} from './range-vector-state.mjs';
import { selectRangeAxisData } from './metric-state.mjs';
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
 */
export const buildTimeSeriesOptions = ({
  series = [],
  unit = '%',
  digits = 1,
  animation = false,
} = {}) => {
  const normalized = series.map((item) => ({
    ...item,
    data: normalizeRangeValues(item?.data),
  }));
  const axisData = selectRangeAxisData(normalized);
  const showLegend = normalized.length > 1;

  return {
    animation,
    legend: showLegend ? { bottom: 10, left: 'center' } : { show: false },
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
      top: 20,
      bottom: showLegend ? 60 : 30,
      left: '7%',
      right: 10,
    },
    dataZoom: buildRangeDataZoom(),
    xAxis: {
      type: 'category',
      data: axisData.map((item) => timeParse(item.timestamp)),
      axisLabel: {
        formatter: (value) => timeParse(value, 'HH:mm'),
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: (value) => (unit ? `${value} ${unit}` : `${value}`),
      },
    },
    series: normalized.map((item) =>
      buildRangeLineSeries(
        {
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
        },
        { digits },
      ),
    ),
  };
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
