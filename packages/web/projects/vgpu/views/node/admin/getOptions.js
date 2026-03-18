import { timeParse } from '@/utils';

export const getRangeOptions = ({ core = [], memory = [] }, t = (v) => v) => {
  return {
    legend: {
      bottom: 0,
      orient: 'horizontal',
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
      formatter: function (params) {
        var res = params[0].name + '<br/>';
        for (var i = 0; i < params.length; i++) {
          res +=
            params[i].marker +
            params[i].seriesName +
            ' : ' +
            (+params[i].value).toFixed(0) +
            `%<br/>`;
        }

        return res;
      },
    },
    grid: {
      top: 20, // 上边距
      bottom: 60, // 下边距
      left: '8%', // 左边距
      right: 10, // 右边距
    },
    xAxis: {
      type: 'category',
      data: core.map((item) => timeParse(+item.timestamp)),
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
          return `${value} %`;
        },
      },
    },
    series: [
      {
        name: t('dashboard.compute'),
        data: core,
        type: 'line',
        itemStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
      },
      {
        name: t('dashboard.memory'),
        data: memory,
        type: 'line',
        itemStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)', // 设置线条颜色为橙色
        },
      },
    ],
  };
};
