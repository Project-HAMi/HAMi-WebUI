export const createComputeUsageGaugeConfig = () => ({
  title: 'computeUsageRate',
  percent: 0,
  query: 'avg(avg(hami_core_util) by (node, device_uuid))',
  percentQuery: 'avg(avg(hami_core_util_avg) by (node, device_uuid))',
  // hami_core_util is already a percentage in the 0-100 range. Dividing it by
  // cluster-wide hami_core_size would make the displayed value shrink as more
  // devices are added to the cluster.
  total: 100,
  used: 0,
  unit: ' ',
});

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
