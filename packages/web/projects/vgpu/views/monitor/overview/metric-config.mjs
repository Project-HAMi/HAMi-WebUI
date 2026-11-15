import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';

export const createOverviewGaugeConfigs = () => {
  const { computeAllocation, memoryAllocation, computeUsage, memoryUsage } = buildPhysicalResourceQueries();

  return [
    {
      id: 'compute-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.computeAllocRate',
      descriptionKey: 'dashboard.computeAllocRateDescription',
      detailMode: 'value',
      percent: 0,
      query: computeAllocation.query,
      totalQuery: computeAllocation.totalQuery,
      total: 0,
      used: 0,
    },
    {
      id: 'memory-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.overviewMemAllocRate',
      descriptionKey: 'dashboard.overviewMemAllocRateDescription',
      detailMode: 'ratio',
      percent: 0,
      query: memoryAllocation.query,
      totalQuery: memoryAllocation.totalQuery,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
    {
      id: 'compute-utilization',
      kind: 'utilization',
      titleKey: 'dashboard.computeUsageRate',
      descriptionKey: 'dashboard.computeUsageRateDescription',
      detailMode: 'value',
      percent: 0,
      query: computeUsage.query,
      total: 100,
      used: 0,
      unit: '%',
    },
    {
      id: 'memory-utilization',
      kind: 'utilization',
      titleKey: 'dashboard.overviewMemUsageRate',
      descriptionKey: 'dashboard.overviewMemUsageRateDescription',
      detailMode: 'ratio',
      percent: 0,
      query: memoryUsage.query,
      totalQuery: memoryUsage.totalQuery,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
  ];
};
