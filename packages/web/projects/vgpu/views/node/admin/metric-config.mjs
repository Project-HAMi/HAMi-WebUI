import { buildPhysicalAllocationQueries, buildPhysicalUsageQueries } from '../../../metrics/physical-resource-queries.mjs';

// A same-name replacement node owns a different history.
const selector = 'node=$node,node_uid=$uid';
export const nodeResourceQueries = {
  ...buildPhysicalAllocationQueries({ selector }),
  ...buildPhysicalUsageQueries({ selector, byNode: true }),
};

const { computeUsage, memoryUsage } = nodeResourceQueries;
export const nodeUsageGaugeConfigs = [
  { query: computeUsage.query, total: 100, percent: 0 },
  { query: memoryUsage.query, totalQuery: memoryUsage.totalQuery, percent: 0 },
];
