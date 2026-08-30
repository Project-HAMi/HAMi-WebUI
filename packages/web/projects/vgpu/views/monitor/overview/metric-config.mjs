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

export const createOverviewGaugeConfigs = () => [
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
    query:
      'avg(sum(hami_container_vcore_allocated) by (instance)) or (avg(sum(hami_core_size) by (instance)) * 0)',
    totalQuery: 'avg(sum(hami_core_size) by (instance))',
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
    query:
      '(avg(sum(hami_container_vmemory_allocated) by (instance)) or (avg(sum(hami_memory_size) by (instance)) * 0)) / 1024',
    totalQuery: 'avg(sum(hami_memory_size) by (instance)) / 1024',
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
    query: 'avg(sum(hami_memory_used) by (instance)) / 1024',
    totalQuery:
      'avg(sum by (instance) (hami_memory_size and on (instance, node, provider, device_uuid) hami_memory_used)) / 1024',
    total: 0,
    used: 0,
    unit: 'GiB',
  },
];

// Allocation series only exist for devices assigned to a workload. Sum all
// devices within each scrape target first, then average equivalent exporter
// replicas so idle devices remain in the capacity denominator. Capacity also
// supplies an explicit zero when an entire node has no allocation series.
const buildNodeAllocationTopQuery = (allocatedMetric, capacityMetric) => {
  const allocated = `avg by (node) (sum by (node, instance) (${allocatedMetric}))`;
  const capacity = `avg by (node) (sum by (node, instance) (${capacityMetric}))`;
  return `topk(5, (${allocated} / ${capacity} * 100) or on (node) (${capacity} * 0))`;
};

export const createNodeAllocationTopQueries = () => ({
  compute: buildNodeAllocationTopQuery(
    'hami_container_vcore_allocated',
    'hami_core_size',
  ),
  memory: buildNodeAllocationTopQuery(
    'hami_container_vmemory_allocated',
    'hami_memory_size',
  ),
});
