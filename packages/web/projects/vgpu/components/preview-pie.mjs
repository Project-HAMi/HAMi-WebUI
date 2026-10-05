import i18n from '@/locales';
import { buildDonutOptions } from '../metrics/chart-presets.mjs';

export const getPreviewBarPie = (statusConfig) => buildDonutOptions({
  unit: i18n.global.t('common.unitSheet'),
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
