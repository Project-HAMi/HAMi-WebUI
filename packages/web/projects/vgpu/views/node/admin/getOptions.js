import { timeParse } from '@/utils';

export const getRangeOptions = ({ allocation = [], usage = [] }, t = (v) => v) => {
  const xDataSource = allocation?.length ? allocation : usage;
  return {
    legend: {
      bottom: 10,
      left: 'center',
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
      formatter: function (params) {
        if (!params || params.length === 0) return '';
        var res = params[0].name + '<br/>';
        for (var i = 0; i < params.length; i++) {
          res +=
            params[i].marker +
            params[i].seriesName +
            ' : ' +
            (+params[i].value).toFixed(0) +
            '<br/>';
        }

        return res;
      },
    },
    grid: {
      top: 20, // 上边距
      bottom: 60, // 下边距
      left: '7%', // 左边距
      right: 10, // 右边距
    },
    xAxis: {
      type: 'category',
      data: xDataSource.map((item) => timeParse(+item.timestamp)),
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
        data: allocation,
        type: 'line',
        itemStyle: {
          color: 'rgb(84, 112, 198)',
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)',
        },
      },
      {
        name: t('dashboard.usageRateLegend'),
        data: usage,
        type: 'line',
        itemStyle: {
          color: 'rgb(145, 204, 117)',
        },
        lineStyle: {
          color: 'rgb(145, 204, 117)',
        },
      },
    ],
  };
};
