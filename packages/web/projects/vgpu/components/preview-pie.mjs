import { buildDonutOptions } from '../metrics/chart-presets.mjs';

export const getPreviewBarPie = (statusConfig, unit) => buildDonutOptions({
  unit,
  data: statusConfig.map((item) => ({
    ...item,
    value: Number(item.value || 0),
    itemStyle: {
      color: item.color,
      borderRadius: 6,
      borderColor: '#fff',
      borderWidth: 2,
    },
  })),
});
