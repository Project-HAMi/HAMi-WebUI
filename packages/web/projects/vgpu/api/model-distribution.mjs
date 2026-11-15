import { readCoverageCount } from './overview-inventory.mjs';

const isCount = (value) => Number.isSafeInteger(value) && value >= 0;

export const readModelDistribution = (reply) => {
  const inventory = readCoverageCount(reply?.coverage);
  if (!Array.isArray(reply.items)) throw new TypeError('Invalid model distribution');
  const seen = new Set();
  const rows = reply.items.map((item) => {
    if (typeof item?.model !== 'string' || !isCount(item.physicalDevices) || item.physicalDevices === 0 || seen.has(item.model)) {
      throw new TypeError('Invalid or duplicate model count');
    }
    seen.add(item.model);
    return { type: item.model || null, count: item.physicalDevices, share: item.physicalDevices / inventory.observed };
  });
  if (rows.reduce((sum, row) => sum + row.count, 0) !== inventory.observed) {
    throw new TypeError('Model counts do not match the physical inventory');
  }
  rows.sort((a, b) => b.count - a.count || (a.type === null ? 1 : b.type === null ? -1
    : a.type.localeCompare(b.type, 'en', { numeric: true, sensitivity: 'base' })));
  return { rows, inventory };
};
