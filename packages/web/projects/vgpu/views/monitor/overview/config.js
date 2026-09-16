import { buildClusterTrendQueries } from '../../../metrics/query-contract.mjs';
import { CHART_COLORS } from '../../../metrics/chart-colors.mjs';

export const getRangeConfigInit = (t) => {
  const queries = buildClusterTrendQueries();
  const allocation = (key, query) => ({
    key,
    name: t('dashboard.allocRateLegend'),
    color: CHART_COLORS.allocation,
    data: [],
    query,
  });
  const usage = (key, query) => ({
    key,
    name: t('dashboard.usageRateLegend'),
    color: CHART_COLORS.usage,
    data: [],
    query,
  });

  return [
    {
      title: t('dashboard.gpuComputeAllocUsageTrend'),
      dataSource: [
        allocation('compute-allocation', queries.computeAllocation),
        usage('compute-usage', queries.computeUsage),
      ],
    },
    {
      title: t('dashboard.gpuMemAllocUsageTrend'),
      dataSource: [
        allocation('memory-allocation', queries.memoryAllocation),
        usage('memory-usage', queries.memoryUsage),
      ],
    },
  ];
};
