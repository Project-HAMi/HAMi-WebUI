import { timeParse } from '@/utils';
import { cloneDeep } from 'lodash';
import nodeApi from '~/vgpu/api/node';
import { MessagePlugin } from 'tdesign-vue-next';
import i18n from '@/locales';

export const getResourceStatus = (statusConfig) => {
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
        radius: ['50%', '70%'],
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
          text: '节点', // 设置文本内容
          fill: '#333', // 文字颜色
          fontSize: 12, // 文字大小
        },
      },
    ],
  };
};

export const getPressure = ({ percent = 0, title }) => {
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
          length: 3,
          lineStyle: {
            width: 2,
            color: '#999',
          },
          distance: 3,
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
          formatter: '{a|{value}}{b|%}',

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

export const getAlarm = ({ timeKeys, data }) => {
  return {
    legend: {
      // data: [],
    },
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
      },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: function (value) {
          return `${value} 次`;
        },
      },
    },
    series: [
      {
        data,
        type: 'line',
        smooth: true,
        showSymbol: true,
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
                  color: 'rgba(37, 99, 235, 0.16)', // 渐变起始颜色
                },
                {
                  offset: 1,
                  color: 'rgba(37, 99, 235, 0.00)', // 渐变结束颜色
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
    ],
    grid: {
      top: '3%', // 上边距
      bottom: '8%', // 下边距
      left: '8%', // 左边距
      right: '3%', // 右边距
    },
    graphic: [
      {
        type: 'text', // 添加文本标签
        left: '23%', // 文本标签水平居中
        top: 'center', // 文本标签垂直居中
        style: {
          text: '显卡', // 设置文本内容
          fill: '#333', // 文字颜色
          fontSize: 12, // 文字大小
        },
      },
      // {
      //   type: 'text', // 添加文本标签
      //   left: 'center', // 文本标签水平居中
      //   top: '170', // 文本标签垂直居中
      //   style: {
      //     text: list.length + '个', // 设置文本内容
      //     fill: '#333', // 文字颜色
      //     fontSize: 16, // 文字大小
      //   },
      // },
    ],
  };
};

export const handleChartClick = async (params, router) => {
  const name = params.data.name;
  const { list } = await nodeApi.getNodes({ filters: {} });
  const node = list.find((node) => node.name === name);

  if (node) {
    const uuid = node.uid;
    router.push(`/admin/vgpu/node/admin/${uuid}?nodeName=${name}`);
  } else {
    MessagePlugin.error(i18n.global.t('node.nodeNotFound'));
  }
};

const CARD_PIE_COLORS = ['#76B900', '#9FCB98', '#F59E0B', '#4F8F87', '#14B8A6', '#6B7280'];

export const getCardOptions = (list, chartWidth) => {
  const data = list.reduce((all, current) => {
    const name = current.type;
    if (all[name]) {
      all[name]++;
    } else {
      all[name] = 1;
    }

    return all;
  }, {});

  const dataList = Object.entries(data).map(([key, value], index) => ({
    name: key,
    value,
    itemStyle: {
      color: CARD_PIE_COLORS[index % CARD_PIE_COLORS.length],
    },
  }));

  return {
    tooltip: {
      trigger: 'item',
      confine: true,
      formatter: (params) => {
        const unit = i18n.global.t('common.unitSheet');
        const suffix = unit ? ` ${unit}` : '';
        return `${params.name}: ${params.value}${suffix}`;
      },
    },
    color: CARD_PIE_COLORS,
    series: [
      {
        type: 'pie',
        radius: ['48%', '72%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        itemStyle: {
          borderRadius: 6,
          borderColor: '#fff',
          borderWidth: 2,
        },
        label: {
          alignTo: 'edge',
          formatter: (params) => {
            const unit = i18n.global.t('common.unitSheet');
            const suffix = unit ? ` ${unit}` : '';
            return `{name|${params.name}}\n{cnt|${params.value}${suffix}}`;
          },
          minMargin: 8,
          edgeDistance: 8,
          lineHeight: 18,
          rich: {
            name: {
              fontSize: 12,
              color: '#324558',
              fontWeight: 500,
            },
            cnt: {
              fontSize: 11,
              color: '#697886',
            },
          },
        },
        labelLine: {
          length: 12,
          length2: 8,
          smooth: true,
          lineStyle: {
            width: 1,
            color: '#cbd5e1',
          },
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
        labelLayout: function (params) {
          const isLeft = params.labelRect.x < chartWidth / 2;
          const points = params.labelLinePoints;
          points[2][0] = isLeft
            ? params.labelRect.x
            : params.labelRect.x + params.labelRect.width;
          return {
            labelLinePoints: points,
          };
        },
        data: dataList,
      },
    ],
  };
};

export const getLineOptions = ({
  title,
  usedData = [],
  totalData = [],
  unit = '%',
}) => {
  const xData = usedData;

  let yData = cloneDeep(totalData);

  const xAxisData = usedData.length ? usedData : totalData;

  return {
    legend: {
      data: ['分配率', usedData.length && '使用率'],
      // left: 0,
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross',
      },
      formatter: function (params) {
        var res = timeParse(+params[0].name) + '<br/>';
        for (var i = 0; i < params.length; i++) {
          res +=
            params[i].marker +
            params[i].seriesName +
            ' : ' +
            (+params[i].value).toFixed(0) +
            `${unit}<br/>`;
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
      data: xAxisData.map((item) => timeParse(+item.timestamp)),
      axisLabel: {
        formatter: function (value) {
          return timeParse(value, 'HH:mm');
          // return timeParse(value, 'MM-DD');
        },
        // interval: function (index, value) {
        //   var date = new Date(value);
        //
        //   return date.getHours() % 2 === 0 && date.getMinutes() === 0;
        // },
      },
    },
    yAxis: {
      type: 'value',
      max: 100,
      axisLabel: {
        formatter: function (value) {
          return `${value} ${unit}`;
        },
      },
    },
    series: [
      {
        name: '使用率',
        data: usedData.map((item) => {
          if (title === 'GPU 内存使用') {
            return (item.value / 1024).toFixed(1);
          }
          return { value: item.value.toFixed(1), name: item.timestamp };
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
      {
        name: '分配率',
        data: yData.map((item) => {
          if (!item) return item;
          return { value: item.value.toFixed(1), name: item.timestamp };
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
      },
    ],
  };
};

export const getRangeOptions = (data) => {
  if (!data || !data[0] || !data[0].data) {
    return {
      animation: true,
      series: [],
    };
  }

  return {
    animation: true,
    legend: {
      // data: [],
      bottom: 10,
      left: 'center',
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'line',
      },
      formatter: function (params) {
        if (!Array.isArray(params) || params.length === 0) return '';

        let result = `<div style="margin-bottom:5px;">${params[0]?.name ?? ''}</div>`;
        for (let i = 0; i < params.length; i++) {
          const item = params[i];
          const raw = Array.isArray(item?.value) ? item.value[item.value.length - 1] : item?.value;
          const num = Number(raw);
          const value = Number.isFinite(num) ? `${num.toFixed(3)}%` : '-';
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
      data: data[0].data.map((item) => timeParse(+item.timestamp)),
      axisLabel: {
        formatter: function (value) {
          return timeParse(value, 'HH:mm');
          // return timeParse(value, 'MM-DD');
        },
        // interval: function (index, value) {
        //   var date = new Date(value);
        //
        //   return date.getHours() % 2 === 0 && date.getMinutes() === 0;
        // },
      },
    },
    yAxis: {
      type: 'value',
      // max: 100,
      axisLabel: {
        formatter: function (value) {
          return `${value}`;
        },
      },
    },
    series: data.map((item) => ({
      ...item,
      type: 'line',
      // areaStyle: {
      //   normal: {
      //     color: {
      //       type: 'linear',
      //       x: 0, // 渐变起始点 0%
      //       y: 0, // 渐变起始点 0%
      //       x2: 0, // 渐变结束点 100%
      //       y2: 1, // 渐变结束点 100%
      //       colorStops: [
      //         {
      //           offset: 0,
      //           color: 'rgba(34, 139, 34, 0.16)', // 渐变起始颜色
      //         },
      //         {
      //           offset: 1,
      //           color: 'rgba(34, 139, 34, 0.00)', // 渐变结束颜色
      //         },
      //       ],
      //       global: false, // 缺省为 false
      //     },
      //   },
      // },
    })),
  };
};
