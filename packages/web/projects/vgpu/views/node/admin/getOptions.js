import { timeParse } from '@/utils';
import {
  buildRangeLineSeries,
  formatRangeTooltipValue,
  normalizeRangeValues,
} from '../../../metrics/range-vector-state.mjs';

const buildPercentTooltipFormatter = () => {
  return (params) => {
    if (!Array.isArray(params) || params.length === 0) return '';
    let result = `<div style="margin-bottom:5px;">${
      params[0]?.axisValueLabel || params[0]?.name || ''
    }</div>`;
    for (let i = 0; i < params.length; i++) {
      const item = params[i];
      const value = formatRangeTooltipValue(item?.value, {
        digits: 3,
        unit: '%',
      });
      result += `<div style="display:flex;align-items:center;font-size:14px;line-height:22px;">
          <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background-color:${
            item?.color || '#5B8FF9'
          };margin-right:5px;"></span>
          <span>${item?.seriesName || '-'}:&nbsp;</span>
          <span style="font-weight:bold;">${value}</span>
        </div>`;
    }
    return result;
  };
};

export const getRangeOptions = (
  { allocation = [], usage = [] },
  t = (v) => v,
) => {
  const normalizedAllocation = normalizeRangeValues(allocation);
  const normalizedUsage = normalizeRangeValues(usage);
  const xDataSource = normalizedAllocation.length
    ? normalizedAllocation
    : normalizedUsage;
  return {
    animation: false,
    legend: {
      bottom: 10,
      left: 'center',
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
      formatter: buildPercentTooltipFormatter(),
    },
    grid: {
      top: 20,
      bottom: 60,
      left: '7%',
      right: 10,
    },
    dataZoom: [
      {
        type: 'inside',
        xAxisIndex: 0,
        filterMode: 'none',
      },
    ],
    xAxis: {
      type: 'category',
      data: xDataSource.map((item) => timeParse(item.timestamp)),
      axisLabel: {
        formatter: function (value) {
          return timeParse(value, 'HH:mm');
        },
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: function (value) {
          return `${value}`;
        },
      },
    },
    series: [
      buildRangeLineSeries({
        name: t('dashboard.allocRateLegend'),
        data: normalizedAllocation,
        itemStyle: {
          color: '#5B8FF9',
        },
        lineStyle: {
          width: 3,
          color: '#5B8FF9',
        },
      }),
      buildRangeLineSeries({
        name: t('dashboard.usageRateLegend'),
        data: normalizedUsage,
        itemStyle: {
          color: '#42C090',
        },
        lineStyle: {
          width: 3,
          color: '#42C090',
        },
      }),
    ],
  };
};
