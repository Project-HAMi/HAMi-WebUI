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
