import { timeParse } from '@/utils';
import i18n from '@/locales';
import {
  buildRangeLineSeries,
  formatRangeTooltipValue,
  normalizeRangeValues,
} from '../metrics/range-vector-state.mjs';

export const getPreviewBarPie = (statusConfig, { title }) => {
  const unit = i18n.global.t('common.unitSheet');
  const suffix = unit ? ` ${unit}` : '';

  const dataList = statusConfig.map((item) => ({
    ...item,
    value: Number(item.value || 0),
    itemStyle: {
      color: item.color,
      borderRadius: 6,
      borderColor: '#fff',
      borderWidth: 2,
    },
  }));

  return {
    tooltip: {
      trigger: 'item',
      confine: true,
      formatter: (params) => {
        return `${params.name}: ${params.value}${suffix}`;
      },
    },
    series: [
      {
        name: title,
        type: 'pie',
        radius: ['48%', '72%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        label: {
          show: false,
        },
        labelLine: {
          show: false,
        },
        emphasis: {
          scale: true,
          scaleSize: 4,
          itemStyle: {
            shadowBlur: 12,
            shadowOffsetX: 0,
            shadowColor: 'rgba(0, 0, 0, 0.15)',
          },
        },
        data: dataList,
      },
    ],
    grid: {
      top: 1,
      bottom: 1,
      left: 1,
      right: 1,
    },
  };
};

export const getLineOptions = ({
  data = [],
  unit = '%',
  seriesName,
  animation = true,
}) => {
  const normalizedData = normalizeRangeValues(data);

  return {
    animation,
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'line',
        lineStyle: {
          type: 'dashed',
          color: '#8A8A8A',
        },
      },
      formatter: function (params) {
        if (!Array.isArray(params) || params.length === 0) return '';

        let result = `<div style="margin-bottom:5px;">${
          params[0]?.name ?? ''
        }</div>`;
        for (let i = 0; i < params.length; i++) {
          const item = params[i];
          const raw = Array.isArray(item?.value)
            ? item.value[item.value.length - 1]
            : item?.value;
          const value = formatRangeTooltipValue(raw, {
            digits: 1,
            unit,
            separator: ' ',
          });
          result += `
            <div style="display:flex;align-items:center;font-size:14px;line-height:22px;">
              <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background-color:${
                item?.color || '#5B8FF9'
              };margin-right:5px;"></span>
              <span>${item?.seriesName || '-'}:&nbsp;</span>
              <span style="font-weight:bold;">${value}</span>
            </div>
          `;
        }
        return result;
      },
    },
    grid: {
      top: 20,
      bottom: 30,
      left: 30,
      right: 30,
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
      data: normalizedData.map((item) => timeParse(item.timestamp)),
      axisLabel: {
        formatter: function (value) {
          return timeParse(value, 'HH:mm');
        },
      },
    },
    yAxis: {
      type: 'value',
    },
    series: [
      buildRangeLineSeries(
        {
          name: seriesName || '',
          data: normalizedData,
          lineStyle: {
            width: 3,
            color: '#5B8FF9',
          },
          itemStyle: {
            color: '#5B8FF9',
            borderColor: '#5B8FF9',
          },
        },
        { digits: 1 },
      ),
    ],
  };
};
