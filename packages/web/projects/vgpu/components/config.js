import { timeParse } from '@/utils';

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
  return {
    tooltip: {
      show: false,
    },
    // legend: {
    //   top: '5%',
    //   left: 'center',
    // },

    series: [
      {
        name: 'Access From',
        type: 'pie',
        radius: ['50%', '65%'],
        avoidLabelOverlap: false,
        itemStyle: {
          borderRadius: 3,
          borderColor: '#fff',
        },
        label: {
          show: false,
          position: 'center',
        },
        emphasis: {
          label: {
            show: false,
            fontSize: 40,
            fontWeight: 'bold',
          },
        },
        labelLine: {
          show: false,
        },
        // data: [
        //   { value: data.running, itemStyle: {color: '#ff0000'} },
        //   { value: data.stopped , itemStyle: {color: '#ff0000'}},
        //   { value: data.error , itemStyle: {color: '#ff0000'}},
        // ],
        data: statusConfig.map((item) => ({
          ...item,
          itemStyle: { color: item.color },
        })),
      },
    ],
    grid: {
      top: 1, // 上边距
      bottom: 1, // 下边距
      left: 1, // 左边距
      right: 1, // 右边距
    },
    graphic: [
      {
        type: 'text', // 添加文本标签
        left: 'center', // 文本标签水平居中
        top: 'center', // 文本标签垂直居中
        style: {
          text: title, // 设置文本内容
          fill: '#333', // 文字颜色
          fontSize: 12, // 文字大小
        },
      },
    ],
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
      },
      {
        name: '显存',
        type: 'bar',
        data: memory.data,
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
