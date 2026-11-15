const isCount = (value) => Number.isSafeInteger(value) && value >= 0;

// An empty observed subset is not evidence that the cluster is empty. Preserve
// known zero only when the inventory source confirms a complete collection.
export const readCoverageCount = (coverage) => {
  if (!coverage || !isCount(coverage.observed) || typeof coverage.complete !== 'boolean') {
    throw new TypeError('Invalid resource coverage');
  }
  if (coverage.complete && coverage.total !== coverage.observed) {
    throw new TypeError('Complete inventory must confirm its observed total');
  }
  if (Object.hasOwn(coverage, 'total') && (!isCount(coverage.total) || coverage.total < coverage.observed)) {
    throw new TypeError('Inventory total cannot be smaller than its observed collection');
  }
  return {
    count: coverage.complete || coverage.observed > 0 ? coverage.observed : undefined,
    observed: coverage.observed,
    partial: !coverage.complete,
  };
};

const nonemptyString = value => typeof value === 'string' && value.length > 0;
const optionalFlag = value => value == null || typeof value === 'boolean';
const optionalRatio = value => value == null || (Number.isFinite(value) && value >= 0 && value <= 1);

export const physicalDeviceKey = ({ node, provider, deviceId }) => JSON.stringify([node, provider, deviceId]);

function checkRows(entity, rows, coverage, identity, valid) {
  readCoverageCount(coverage);
  if (coverage.entity !== entity || !Array.isArray(rows) || rows.length !== coverage.observed) {
    throw new TypeError(`Invalid ${entity} inventory coverage`);
  }
  const keys = new Set();
  for (const row of rows) {
    if (!row || !valid(row)) throw new TypeError(`Invalid ${entity} inventory row`);
    const key = identity(row);
    if (keys.has(key)) throw new TypeError(`Duplicate ${entity} inventory identity`);
    keys.add(key);
  }
}

export function readOverviewInventory(reply) {
  const { nodes, devices, nodeCoverage, deviceCoverage } = reply;
  checkRows('node', nodes, nodeCoverage, node => node.name, node =>
    nonemptyString(node.name) && optionalFlag(node.ready) && optionalFlag(node.schedulable)
      && (node.physicalDevices == null || isCount(node.physicalDevices)));
  checkRows('device', devices, deviceCoverage, physicalDeviceKey, device =>
    nonemptyString(device.node) && nonemptyString(device.provider) && nonemptyString(device.deviceId)
      && optionalFlag(device.healthy) && optionalRatio(device.utilizationRatio) && optionalRatio(device.memoryUsageRatio));
  return { nodes, devices, nodeCoverage, deviceCoverage };
}
