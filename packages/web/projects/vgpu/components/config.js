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

export const getLineOptions = ({ data = [], unit = '%' }) => {
  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
      formatter: function (params) {
        var res = params[0].name + '<br/>';
        for (var i = 0; i < params.length; i++) {
          res +=
            params[i].marker + (+params[i].value).toFixed(0) + ` ${unit}<br/>`;
        }
        return res;
      },
    },
    grid: {
      top: 7, // 上边距
      bottom: 20, // 下边距
      left: '7%', // 左边距
      right: 10, // 右边距
    },
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
        data: data.map((item) => {
          return item.value.toFixed(1);
        }),
        type: 'line',
      },
    ],
  };
};
