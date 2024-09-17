import { timeParse } from '@/utils';

export const getRangeOptions = ({ core = [], memory = [] }) => {
  return {
    legend: {
      // data: [],
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
      top: 37, // 上边距
      bottom: 20, // 下边距
      left: '7%', // 左边距
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
        name: '算力',
        data: core,
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
        itemStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
        lineStyle: {
          color: 'rgb(84, 112, 198)', // 设置线条颜色为橙色
        },
      },
      {
        name: '显存',
        data: memory,
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
                  color: 'rgba(34, 139, 34, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(34, 139, 34, 0.00)', // 渐变结束颜色
                },
              ],
              global: false, // 缺省为 false
            },
          },
        },
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
