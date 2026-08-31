export const createNodeComputeUsageGaugeConfig = ({ selector }) => ({
  titleKey: 'dashboard.computeUsageRate',
  percent: 0,
  query: `avg(avg(hami_core_util{${selector}}) by (node, device_uuid))`,
  percentQuery: `avg(avg(hami_core_util_avg{${selector}}) by (node, device_uuid))`,
  total: 100,
  used: 0,
  unit: ' ',
});
