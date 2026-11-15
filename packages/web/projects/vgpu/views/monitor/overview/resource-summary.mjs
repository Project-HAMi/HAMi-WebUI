import { readModelDistribution } from '../../../api/model-distribution.mjs';
import { readCoverageCount, readOverviewInventory } from '../../../api/overview-inventory.mjs';
import { isNodeSchedulingEligible } from '../../node/node-status.mjs';
import { optionCountOf, readOptionCounts } from '../../task/admin/option-counts.mjs';
import { readWorkloadTotal } from './overview-state.mjs';

// Both cards describe the same physical inventory, including unnamed models.
export function readResourceInventory(reply) {
  const { nodes, devices, nodeCoverage, deviceCoverage } = readOverviewInventory(reply);
  const nodeInventory = readCoverageCount(nodeCoverage);
  const schedulable = nodes.filter(node => isNodeSchedulingEligible({
    isReady: node.ready, isSchedulable: node.schedulable,
  })).length;

  const health = { healthy: 0, unhealthy: 0, unknown: 0 };
  const modelCounts = new Map();
  for (const device of devices) {
    if (device.healthy === true) health.healthy++;
    else if (device.healthy === false) health.unhealthy++;
    else health.unknown++;
    // Keep exact, nonblank model names so drill-downs use the list's own filter.
    const model = device.model?.trim() ? device.model : '';
    modelCounts.set(model, (modelCounts.get(model) ?? 0) + 1);
  }
  const models = readModelDistribution({ coverage: deviceCoverage,
    items: Array.from(modelCounts, ([model, physicalDevices]) => ({ model, physicalDevices })),
  });
  return {
    nodes, models,
    summary: {
      nodes: { ...nodeInventory, schedulable: nodeInventory.count === undefined ? undefined : schedulable,
        unavailable: nodeInventory.count === undefined ? undefined : nodeInventory.count - schedulable },
      devices: { ...models.inventory, counts: models.inventory.count === undefined ? {} : health },
    },
  };
}

// The unfiltered workload list counts these groups before it paginates items.
export function readContainerSummary(reply) {
  const total = readWorkloadTotal(reply);
  if (total === undefined) throw new TypeError('Invalid workload list total');
  if (reply.statusCounts === undefined) return { total, status: 'missing', counts: null };
  const counts = readOptionCounts(reply.statusCounts);
  if (counts && Object.values(counts).every(n => n >= 0 && n <= total) && optionCountOf(counts, 'all') === total) {
    const success = optionCountOf(counts, 'success');
    const abnormal = optionCountOf(counts, 'abnormal');
    const other = total - success - abnormal;
    if (other >= optionCountOf(counts, 'pending') + optionCountOf(counts, 'waiting')) {
      return { total, status: 'ready', counts: { success, abnormal, other } };
    }
  }
  return { total, status: 'invalid', counts: null };
}
