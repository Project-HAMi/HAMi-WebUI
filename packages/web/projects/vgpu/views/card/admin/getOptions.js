import { timeParse } from '@/utils';
import i18n from '@/locales';

export const getRangeOptions = ({ core = [], memory = [] }) => {
  return {
    legend: {
      // data: [],
      bottom: 0,
      left: 'center',
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
      // max: 100,
      axisLabel: {
        formatter: function (value) {
          return `${value} %`;
        },
      },
    },
    series: [
      {
        name: i18n.global.t('dashboard.compute'),
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
        name: i18n.global.t('dashboard.memory'),
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
