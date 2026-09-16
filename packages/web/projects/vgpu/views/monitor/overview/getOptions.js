import {
  CHART_COLORS,
  buildDonutOptions,
} from '../../../metrics/chart-presets.mjs';
import nodeApi from '~/vgpu/api/node';
import { MessagePlugin } from 'tdesign-vue-next';
import i18n from '@/locales';

export const handleChartClick = async (params, router) => {
  const name = params.data.name;
  const { list } = await nodeApi.getNodes({ filters: {} });
  const node = list.find((node) => node.name === name);

  if (node) {
    const uuid = node.uid;
    router.push(`/nodes/${uuid}?nodeName=${name}`);
  } else {
    MessagePlugin.error(i18n.global.t('node.nodeNotFound'));
  }
};

export const getCardOptions = (list, chartWidth) => {
  const counts = list.reduce((all, current) => {
    all[current.type] = (all[current.type] || 0) + 1;
    return all;
  }, {});
  const data = Object.entries(counts).map(([name, value], index) => ({
    name,
    value,
    itemStyle: {
      color: CHART_COLORS.categorical[index % CHART_COLORS.categorical.length],
    },
  }));

  return buildDonutOptions({
    data,
    unit: i18n.global.t('common.unitSheet'),
    showLabels: true,
    // Pull each label back to the side of the pie it belongs to.
    labelLayout: (params) => {
      const isLeft = params.labelRect.x < chartWidth / 2;
      const points = params.labelLinePoints;
      points[2][0] = isLeft
        ? params.labelRect.x
        : params.labelRect.x + params.labelRect.width;
      return { labelLinePoints: points };
    },
  });
};
