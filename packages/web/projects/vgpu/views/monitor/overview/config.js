import { buildClusterTrendQueries } from '../../../metrics/query-contract.mjs';

export const getRangeConfigInit = (t) => {
  const queries = buildClusterTrendQueries();

  return [
    {
      title: t('dashboard.gpuComputeAllocUsageTrend'),
      dataSource: [
        {
          name: t('dashboard.allocRateLegend'),
          query: queries.computeAllocation,
          data: [],
          type: 'line',
          itemStyle: {
            color: 'rgb(84, 112, 198)',
            borderColor: 'rgb(84, 112, 198)',
          },
          lineStyle: {
            color: 'rgb(84, 112, 198)',
          },
        },
        {
          name: t('dashboard.usageRateLegend'),
          query: queries.computeUsage,
          data: [],
          itemStyle: {
            color: 'rgb(145, 204, 117)',
            borderColor: 'rgb(145, 204, 117)',
          },
          lineStyle: {
            color: 'rgb(145, 204, 117)',
          },
        },
      ],
    },
    {
      title: t('dashboard.gpuMemAllocUsageTrend'),
      dataSource: [
        {
          name: t('dashboard.allocRateLegend'),
          query: queries.memoryAllocation,
          data: [],
          itemStyle: {
            color: 'rgb(84, 112, 198)',
            borderColor: 'rgb(84, 112, 198)',
          },
          lineStyle: {
            color: 'rgb(84, 112, 198)',
          },
        },
        {
          name: t('dashboard.usageRateLegend'),
          query: queries.memoryUsage,
          data: [],
          itemStyle: {
            color: 'rgb(145, 204, 117)',
            borderColor: 'rgb(145, 204, 117)',
          },
          lineStyle: {
            color: 'rgb(145, 204, 117)',
          },
        },
      ],
    },
  ];
};
