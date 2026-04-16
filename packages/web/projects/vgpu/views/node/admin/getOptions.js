import { timeParse } from '@/utils';

const normalizePoints = (points = []) => {
  return points
    .map((point) => {
      if (Array.isArray(point)) {
        return { timestamp: Number(point[0]), value: Number(point[1]) };
      }
      return {
        timestamp: Number(point?.timestamp),
        value: Number(point?.value),
      };
    })
    .filter((item) => Number.isFinite(item.timestamp) && Number.isFinite(item.value));
};

const buildPercentTooltipFormatter = () => {
  return (params) => {
    if (!Array.isArray(params) || params.length === 0) return '';
    let result = `<div style="margin-bottom:5px;">${params[0]?.axisValueLabel || params[0]?.name || ''}</div>`;
    for (let i = 0; i < params.length; i++) {
      const item = params[i];
      const num = Number(item?.value);
      const value = Number.isFinite(num) ? `${num.toFixed(3)}%` : '-';
      result += `<div style="display:flex;align-items:center;font-size:14px;line-height:22px;">
          <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background-color:${item?.color || '#5B8FF9'};margin-right:5px;"></span>
          <span>${item?.seriesName || '-'}:&nbsp;</span>
          <span style="font-weight:bold;">${value}</span>
        </div>`;
    }
    return result;
  };
};

export const getRangeOptions = ({ allocation = [], usage = [] }, t = (v) => v) => {
  const normalizedAllocation = normalizePoints(allocation);
  const normalizedUsage = normalizePoints(usage);
  const xDataSource = normalizedAllocation.length ? normalizedAllocation : normalizedUsage;
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
      top: 20, // 上边距
      bottom: 60, // 下边距
      left: '7%', // 左边距
      right: 10, // 右边距
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
      {
        name: t('dashboard.allocRateLegend'),
        data: normalizedAllocation.map((item) => item.value),
        type: 'line',
        itemStyle: {
          color: '#5B8FF9',
        },
        lineStyle: {
          width: 3,
          color: '#5B8FF9',
        },
      },
      {
        name: t('dashboard.usageRateLegend'),
        data: normalizedUsage.map((item) => item.value),
        type: 'line',
        itemStyle: {
          color: '#42C090',
        },
        lineStyle: {
          width: 3,
          color: '#42C090',
        },
      },
    ],
  };
};
