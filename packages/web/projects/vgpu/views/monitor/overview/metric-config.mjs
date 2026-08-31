import {
  buildComputeAllocationQueries,
  buildGroupedResourceTopQueries,
  buildMemoryAllocationQueries,
  buildMemoryUsageQueries,
} from '../../../metrics/query-contract.mjs';

export const createComputeUsageGaugeConfig = () => ({
  id: 'compute-utilization',
  kind: 'utilization',
  titleKey: 'dashboard.computeUsageRate',
  descriptionKey: 'dashboard.computeUsageRateDescription',
  detailLabelKey: 'dashboard.deviceAverage',
  detailMode: 'value',
  percent: 0,
  query: 'avg(avg(hami_core_util) by (node, device_uuid))',
  // hami_core_util is already a percentage in the 0-100 range. Dividing it by
  // cluster-wide hami_core_size would make the displayed value shrink as more
  // devices are added to the cluster.
  total: 100,
  used: 0,
  unit: '%',
});

export const createOverviewGaugeConfigs = () => {
  const computeAllocation = buildComputeAllocationQueries();
  const memoryAllocation = buildMemoryAllocationQueries();
  const memoryUsage = buildMemoryUsageQueries();

  return [
    {
      id: 'vgpu-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.vgpuAllocRate',
      descriptionKey: 'dashboard.vgpuAllocRateDescription',
      detailLabelKey: 'dashboard.allocated',
      detailMode: 'ratio',
      usedPrecision: 0,
      totalPrecision: 0,
      percent: 0,
      query:
        'avg(sum(hami_container_vgpu_allocated) by (instance)) or (avg(sum(hami_vgpu_count) by (instance)) * 0)',
      totalQuery: 'avg(sum(hami_vgpu_count) by (instance))',
      total: 0,
      used: 0,
      unitKey: 'dashboard.vgpuSlotUnit',
    },
    {
      id: 'compute-allocation',
      kind: 'allocation',
      titleKey: 'dashboard.computeAllocRate',
      descriptionKey: 'dashboard.computeAllocRateDescription',
      detailLabelKey: 'dashboard.allocated',
      detailMode: 'ratio',
      displayDivisor: 100,
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
      descriptionKey: 'dashboard.memAllocRateDescription',
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
    createComputeUsageGaugeConfig(),
    {
      id: 'memory-utilization',
      kind: 'utilization',
      titleKey: 'dashboard.memUsageRate',
      descriptionKey: 'dashboard.memUsageRateDescription',
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

export const createNodeTopQueries = () =>
  buildGroupedResourceTopQueries('node');

export const createNodeWorkloadDistributionQuery = () =>
  'count(count by (node, container_pod_uuid) (hami_container_vgpu_allocated)) by (node) or on (node) (count by (node) (hami_vgpu_count) * 0)';
