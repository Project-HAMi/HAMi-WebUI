import { timeParse } from '@/utils';
import i18n from '@/locales';

export default ({ percent, title, unit = '%' }) => {
  const value = percent.toFixed(1);

  let thisColor = '';

  if (value < 30) {
    thisColor = '#16A34A';
  } else if (value >= 30 && value <= 80) {
    thisColor = '#2563EB';
  } else {
    thisColor = '#DC2626';
  }

  return {
    series: [
      {
        type: 'gauge',
        itemStyle: {
          color: thisColor,
          // shadowColor: thisColor,
          // shadowBlur: 5,
          // shadowOffsetX: 2,
          // shadowOffsetY: 2,
        },
        progress: {
          show: true,
          width: 8,
        },
        axisLine: {
          lineStyle: {
            width: 8,
            color: '#F5F7FA',
          },
        },
        axisTick: {
          show: false,
        },
        axisLabel: {
          show: false, // 隐藏刻度标签
        },
        // splitNumber: 0,
        splitLine: {
          length: 4,
          lineStyle: {
            width: 2,
            color: '#999',
          },
          distance: 5,
        },
        anchor: {
          show: false,
          showAbove: false,
          size: 25,
          itemStyle: {
            borderWidth: 10,
          },
        },
        pointer: {
          show: false,
        },
        title: {
          show: false,
        },
        detail: {
          valueAnimation: true,
          width: '60%',
          lineHeight: 40,
          borderRadius: 8,
          offsetCenter: [0, '0%'],
          // fontSize: 16,
          fontWeight: 'bolder',
          formatter: `{a|{value}}{b|${unit}}`,

          rich: {
            a: {
              color: '#1D2B3A',
              lineHeight: 10,
              fontSize: 20,
            },
            b: {
              color: '#1D2B3A',
            },
          },
        },
        data: [
          {
            value,
          },
        ],
      },
    ],
    graphic: [
      {
        type: 'text',
        left: 'center',
        bottom: 8,
        style: {
          text: title,
          fill: '#333', // 设置标题文字颜色
          fontSize: 14, // 设置标题文字大小
          borderRadius: 999,
          backgroundColor: '#F5F7FA',
          padding: [6, 16],
        },
      },
    ],
  };
};

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
      top: 1, // 上边距
      bottom: 1, // 下边距
      left: 1, // 左边距
      right: 1, // 右边距
    },
  };
};

export const getTopOptions = ({ core, memory }) => {
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'shadow',
      },
    },
    legend: {},
    grid: {
      left: '3%',
      right: '4%',
      bottom: '1%',
      top: '10%',
      containLabel: true,
    },
    xAxis: {
      type: 'value',
      boundaryGap: [0, 0.01],
      axisLabel: {
        formatter: function (value) {
          return `${value} %`;
        },
      },
    },
    yAxis: {
      type: 'category',
      data: core.data.map((item) =>
        item.name.length > 15 ? item.name.slice(0, 15) + '...' : item.name,
      ),
    },
    series: [
      {
        name: '算力',
        type: 'bar',
        data: core.data,
        itemStyle: {
          color: '#4F98CA',
        },
      },
      {
        name: '显存',
        type: 'bar',
        data: memory.data,
        itemStyle: {
          color: '#4F98CA',
        },
      },
    ],
  };
};

export const getLineOptions = ({ data = [], unit = '%', seriesName, animation = true }) => {
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

        let result = `<div style="margin-bottom:5px;">${params[0]?.name ?? ''}</div>`;
        for (let i = 0; i < params.length; i++) {
          const item = params[i];
          const raw = Array.isArray(item?.value) ? item.value[item.value.length - 1] : item?.value;
          const num = Number(raw);
          const value = Number.isFinite(num) ? `${num.toFixed(1)} ${unit}` : '-';
          result += `
            <div style="display:flex;align-items:center;font-size:14px;line-height:22px;">
              <span style="display:inline-block;width:10px;height:10px;border-radius:50%;background-color:${item?.color || '#5B8FF9'};margin-right:5px;"></span>
              <span>${item?.seriesName || '-'}:&nbsp;</span>
              <span style="font-weight:bold;">${value}</span>
            </div>
          `;
        }
        return result;
      },
    },
    grid: {
      top: 20, // 上边距
      bottom: 30, // 下边距
      left: 30, // 左边距
      right: 30, // 右边距
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
      data: data.map((item) => timeParse(+item.timestamp)),
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
      {
        name: seriesName || '',
        data: data.map((item) => {
          return item.value.toFixed(1);
        }),
        type: 'line',
        lineStyle: {
          width: 3,
          color: '#5B8FF9',
        },
        itemStyle: {
          color: '#5B8FF9',
          borderColor: '#5B8FF9',
        },
      },
    ],
  };
};
