import { physicalModelDistribution } from '../../../api/model-distribution.mjs';
import { physicalDeviceKey, readCoverageCount, readOverviewInventory } from '../../../api/overview-inventory.mjs';
import { isNodeSchedulingEligible } from '../../node/node-status.mjs';
import { optionCountOf, readOptionCounts } from '../../task/admin/option-counts.mjs';
import { readWorkloadTotal } from './overview-state.mjs';

// Both cards describe the same physical inventory, including unnamed models.
export function readResourceInventory(reply) {
  const { nodes, devices, nodeCoverage, deviceCoverage, summary: resourceSummary } = readOverviewInventory(reply);
  const nodeInventory = readCoverageCount(nodeCoverage);
  const schedulable = nodes.filter(node => isNodeSchedulingEligible({
    isReady: node.ready, isSchedulable: node.schedulable,
  })).length;

  const health = { healthy: 0, unhealthy: 0, unknown: 0 };
  const nodeByName = new Map(nodes.map(node => [node.name, node]));
  const readings = [];
  const issues = [];
  for (const node of nodes) {
    const identity = { node: node.name, nodeUid: node.uid, deviceCount: node.physicalDevices,
      model: node.models?.length === 1 ? node.models[0] : '' };
    if (node.ready === false) issues.push({ ...identity, kind: 'nodeNotReady' });
  }
  for (const device of devices) {
    if (device.healthy === true) health.healthy++;
    else if (device.healthy === false) health.unhealthy++;
    else health.unknown++;
    // Keep exact, nonblank model names so drill-downs use the list's own filter.
    const model = device.model?.trim() ? device.model : '';
    const identity = { key: physicalDeviceKey(device), model, provider: device.provider,
      node: device.node, nodeUid: nodeByName.get(device.node)?.uid };
    if (device.healthy === false) issues.push({ ...identity, kind: 'deviceUnhealthy', deviceId: device.deviceId });
    readings.push({ ...identity, id: device.deviceId,
      compute: device.utilizationRatio == null ? null : device.utilizationRatio * 100,
      memory: device.memoryUsageRatio == null ? null : device.memoryUsageRatio * 100 });
  }
  const models = physicalModelDistribution(devices, deviceCoverage);
  return {
    nodes, models, devices: readings, issues, resourceSummary,
    issueStatus: nodes.length || devices.length ? 'ready' : 'empty',
    usageStatus: deviceCoverage.complete || devices.length ? 'ready' : 'error',
    summary: {
      nodes: { ...nodeInventory, schedulable: nodeInventory.count === undefined ? undefined : schedulable,
        unavailable: nodeInventory.count === undefined ? undefined : nodeInventory.count - schedulable },
      devices: { ...models.inventory, counts: models.inventory.count === undefined ? {} : health },
    },
  };
}

const BYTES_PER_GIB = 2 ** 30;

function readPairedGauge(base, used, total, divisor, inventoryStatus) {
  if (inventoryStatus === 'loading') return { ...base, status: 'loading', used: 0, total: 0, percent: 0 };
  if (inventoryStatus === 'error' || inventoryStatus === 'invalid') {
    return { ...base, status: 'error', used: 0, total: 0, percent: 0 };
  }
  if (used == null || total == null || !Number.isFinite(used) || !Number.isFinite(total)) {
    return { ...base, status: 'missing', used: 0, total: 0, percent: 0 };
  }
  if (total <= 0) return { ...base, status: 'no-capacity', used: 0, total: 0, percent: 0 };
  const scaledUsed = used / divisor;
  const scaledTotal = total / divisor;
  return {
    ...base,
    status: 'ready',
    used: scaledUsed,
    total: scaledTotal,
    percent: (scaledUsed / scaledTotal) * 100,
  };
}

function readUtilizationGauge(base, ratio, inventoryStatus) {
  if (inventoryStatus === 'loading') return { ...base, status: 'loading', used: 0, total: 100, percent: 0 };
  if (inventoryStatus === 'error' || inventoryStatus === 'invalid') {
    return { ...base, status: 'error', used: 0, total: 100, percent: 0 };
  }
  if (ratio == null || !Number.isFinite(ratio)) {
    return { ...base, status: 'missing', used: 0, total: 100, percent: 0 };
  }
  const percent = ratio * 100;
  return { ...base, status: 'ready', used: percent, total: 100, percent };
}

export function readOverviewGauges(summary, inventoryStatus = 'ready') {
  return [
    readPairedGauge(
      {
        id: 'compute-allocation',
        kind: 'allocation',
        titleKey: 'dashboard.computeAllocRate',
        descriptionKey: 'dashboard.computeAllocRateDescription',
        detailMode: 'value',
      },
      summary?.computeAllocatedRatio,
      summary?.computeCapacityRatio,
      1,
      inventoryStatus,
    ),
    readPairedGauge(
      {
        id: 'memory-allocation',
        kind: 'allocation',
        titleKey: 'dashboard.overviewMemAllocRate',
        descriptionKey: 'dashboard.overviewMemAllocRateDescription',
        detailMode: 'ratio',
        unit: 'GiB',
      },
      summary?.memoryAllocatedBytes,
      summary?.memoryCapacityBytes,
      BYTES_PER_GIB,
      inventoryStatus,
    ),
    readUtilizationGauge(
      {
        id: 'compute-utilization',
        kind: 'utilization',
        titleKey: 'dashboard.computeUsageRate',
        descriptionKey: 'dashboard.computeUsageRateDescription',
        detailMode: 'value',
        unit: '%',
      },
      summary?.utilizationRatio,
      inventoryStatus,
    ),
    readPairedGauge(
      {
        id: 'memory-utilization',
        kind: 'utilization',
        titleKey: 'dashboard.overviewMemUsageRate',
        descriptionKey: 'dashboard.overviewMemUsageRateDescription',
        detailMode: 'ratio',
        unit: 'GiB',
      },
      summary?.memoryUsedBytes,
      summary?.memoryPhysicalBytes,
      BYTES_PER_GIB,
      inventoryStatus,
    ),
  ];
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
