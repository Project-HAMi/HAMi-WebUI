import { timeParse } from '@/utils';
import {
  buildRangeLineSeries,
  formatRangeTooltipValue,
  normalizeRangeValues,
} from '../../../metrics/range-vector-state.mjs';
import { formatCardTypeTooltip } from '../../../metrics/tooltip-html.mjs';
import nodeApi from '~/vgpu/api/node';
import { MessagePlugin } from 'tdesign-vue-next';
import i18n from '@/locales';
import { selectRangeAxisData } from './overview-state.mjs';

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

const CARD_PIE_COLORS = [
  '#76B900',
  '#9FCB98',
  '#F59E0B',
  '#4F8F87',
  '#14B8A6',
  '#6B7280',
];

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
        return formatCardTypeTooltip(params, unit);
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

export const getRangeOptions = (data) => {
  if (!Array.isArray(data) || !data.length) {
    return {
      animation: true,
      series: [],
    };
  }

  const normalizedData = data.map((item) => ({
    ...item,
    data: normalizeRangeValues(item.data),
  }));
  const axisData = selectRangeAxisData(normalizedData);

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

        let result = `<div style="margin-bottom:5px;">${
          params[0]?.name ?? ''
        }</div>`;
        for (let i = 0; i < params.length; i++) {
          const item = params[i];
          const raw = Array.isArray(item?.value)
            ? item.value[item.value.length - 1]
            : item?.value;
          const value = formatRangeTooltipValue(raw, {
            digits: 3,
            unit: '%',
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
      data: axisData.map((item) => timeParse(item.timestamp)),
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
    series: normalizedData.map((item) =>
      buildRangeLineSeries({
        ...item,
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
      }),
    ),
  };
};
