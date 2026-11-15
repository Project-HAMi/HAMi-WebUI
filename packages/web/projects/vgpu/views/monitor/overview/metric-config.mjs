import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';

export const createOverviewGaugeConfigs = () => {
  const { computeAllocation, memoryAllocation, computeUsage, memoryUsage } = buildPhysicalResourceQueries();

  return [
    {
      id: 'compute-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.computeAllocRate',
      descriptionKey: 'dashboard.computeAllocRateDescription',
      detailLabelKey: 'dashboard.allocated',
      detailMode: 'ratio',
      percent: 0,
      query: computeAllocation.query,
      totalQuery: computeAllocation.totalQuery,
      total: 0,
      used: 0,
      unitKey: 'dashboard.acceleratorEquivalentUnit',
    },
    {
      id: 'memory-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.memAllocRate',
      descriptionKey: 'dashboard.overviewMemAllocRateDescription',
      detailLabelKey: 'dashboard.allocated',
      detailMode: 'ratio',
      totalPrecision: 1,
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
      detailLabelKey: 'dashboard.deviceAverage',
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
      titleKey: 'dashboard.memUsageRate',
      descriptionKey: 'dashboard.overviewMemUsageRateDescription',
      detailLabelKey: 'dashboard.used',
      detailMode: 'ratio',
      totalPrecision: 1,
      percent: 0,
      query: memoryUsage.query,
      totalQuery: memoryUsage.totalQuery,
      total: 0,
      used: 0,
      unit: 'GiB',
    },
  ];
};

export const createNodeWorkloadDistributionQuery = () =>
  'count(count by (node, container_pod_uuid) (hami_container_vgpu_allocated)) by (node) or on (node) (count by (node) (hami_vgpu_count) * 0)';
