import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';
import { CHART_COLORS } from '../../../metrics/chart-colors.mjs';

export const getRangeConfigInit = (t) => {
  const queries = buildPhysicalResourceQueries();
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
      key: 'compute',
      title: t('dashboard.gpuComputeAllocUsageTrend'),
      dataSource: [
        allocation('compute-allocation', queries.computeAllocation.percentQuery),
        usage('compute-usage', queries.computeUsage.percentQuery),
      ],
    },
    {
      key: 'memory',
      title: t('dashboard.gpuMemAllocUsageTrend'),
      dataSource: [
        allocation('memory-allocation', queries.memoryAllocation.percentQuery),
        usage('memory-usage', queries.memoryUsage.percentQuery),
      ],
    },
  ];
};
