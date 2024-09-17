import { timeParse } from '@/utils';
import { cloneDeep } from 'lodash';

export const getLineOptions = ({ title, data = [], unit }) => {
  return {
    title: { text: title, left: 'center' },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
    },
    xAxis: {
      type: 'category',
      data: data.map((item) => timeParse(+item.timestamp)),
      axisLabel: {
        formatter: function (value) {
          return timeParse(value, 'HH:mm');
        },
        interval: function (index, value) {
          var date = new Date(value);

          return date.getMinutes() === 0;
        },
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: function (value) {
          return `${value} ${unit}`;
        },
      },
    },
    series: [
      {
        data: data.map((item) => {
          if (title === 'GPU 内存使用') {
            return (item.value / 1024).toFixed(1);
          }
          return item.value;
        }),
        type: 'line',
        areaStyle: {
          normal: {
            color: {
              type: 'linear',
              x: 0, // 渐变起始点 0%
              y: 0, // 渐变起始点 0%
              x2: 0, // 渐变结束点 100%
              y2: 1, // 渐变结束点 100%
              colorStops: [
                {
                  offset: 0,
                  color: 'rgba(84, 112, 198, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(84, 112, 198, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
      },
    ],
  };
};

export const getGaugeOptions = ({ percent = 0, title, unit }) => {
  let thisColor = '';

  if (percent < 30) {
    thisColor = '#16A34A';
  } else if (percent >= 30 && percent <= 80) {
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
            backgroundColor: '#F5F7FA',
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
          show: false,
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
            value: percent.toFixed(1),
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

export const getTopOptions = (dataSource) => {
  const data = cloneDeep(dataSource).reverse();

  return {
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'shadow',
      },
    },
    legend: {
      show: false,
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: '0',
      containLabel: true,
    },
    xAxis: {
      type: 'value',
      boundaryGap: [0, 0.01],
    },
    yAxis: {
      type: 'category',
      data: data.map((item) => {
        return item.name;
      }),
    },
    series: [
      {
        name: '',
        type: 'bar',
        data: data,
      },
    ],
  };
};
