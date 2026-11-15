const device = 'device_id, node, provider';
const deviceMatch = `on (${device})`;
const bytesPerGiB = 2 ** 30;
const physical = (field, selector = '') =>
  `hami_webui_device_${field}{device_kind="physical"${selector ? ',' + selector : ''}}`;

// WebUI replicas describe the same hardware. Drop a device when the replicas
// disagree instead of choosing a value or counting the device twice.
const agreed = (series) =>
  `(min by (${device}) (${series}) == max by (${device}) (${series}))`;

const sumResources = (values, byNode) => byNode ? `sum by (node) (${values})` : `sum(${values})`;

function singleSource(series) {
  const sources = `group by (${device}, source) (${series})`;
  return `(count by (${device}) (${sources}) == 1)`;
}

function pairedAmounts(numerator, capacity, sum, { divisor, eligibility } = {}) {
  // Before dropping target labels, require at least one complete pair with
  // matching labels from the same scrape target. Neither half alone is a pair.
  const fullPair = `(${numerator} >= 0) and (${capacity} > 0)`;
  const evidence = eligibility ? `${fullPair} and ${eligibility}` : fullPair;
  const paired = `group by (${device}) (${evidence})`;
  const used = `(${agreed(numerator)} >= 0)`;
  const total = `(${agreed(capacity)} > 0)`;
  const amount = sum(`${used} and ${total} and ${paired}`);
  const size = sum(`${total} and ${used} and ${paired}`);
  return {
    query: divisor ? `${amount} / ${divisor}` : amount,
    totalQuery: divisor ? `${size} / ${divisor}` : size,
    percentQuery: `100 * ${amount} / ${size}`,
  };
}

export function buildPhysicalAllocationQueries({ byNode = false, selector = '' } = {}) {
  const sum = (values) => sumResources(values, byNode);
  // The identity metric adds configuration labels to the shared target labels.
  const sameTarget = 'ignoring (configured, mode, resource, device_no, driver_version)';
  const unknown = physical('compute_unknown_occupancies', selector);
  const configured = physical('info', [selector, 'configured="true"'].filter(Boolean).join(','));
  const complete = `(${unknown} == 0) and ${sameTarget} ${configured} and ${deviceMatch} (${agreed(unknown)} == 0)`;

  return {
    computeAllocation: pairedAmounts(
      physical('compute_allocated_ratio', selector), physical('compute_capacity_ratio', selector), sum, { eligibility: complete },
    ),
    memoryAllocation: pairedAmounts(
      physical('memory_allocated_bytes', selector), physical('memory_capacity_bytes', selector), sum, { divisor: bytesPerGiB },
    ),
  };
}

export function buildPhysicalUsageQueries({ byNode = false, selector = '' } = {}) {
  const sum = (values) => sumResources(values, byNode);
  const usage = physical('utilization_ratio', selector);
  const observed = `(${agreed(usage)} and ${singleSource(usage)})`;
  const count = `count(${observed})`;
  // One vote per measured card at each 30-second tick. Missing ticks are
  // omitted; an idle card contributes zero. A wholly stale fleet stays empty.
  const computeUsage = byNode
    ? `100 * avg by (node) (${observed})`
    : `100 * sum_over_time(sum(${observed})[5m:30s]) / sum_over_time(${count}[5m:30s]) and on () ${count}`;

  const memoryUsed = physical('memory_used_bytes', selector);
  const memoryPhysical = physical('memory_physical_bytes', selector);
  // Count sources before discarding unpaired or conflicting samples. A second
  // source on either field is enough to make the card's authority ambiguous.
  // A physical card cannot occupy more memory than it has, even if its
  // schedulable allocation is oversubscribed.
  const validOccupancy = `(${memoryUsed} <= ${memoryPhysical})`;
  const memorySource = singleSource(`${memoryUsed} or ${memoryPhysical}`);

  return {
    computeUsage: { query: computeUsage, percentQuery: computeUsage },
    memoryUsage: pairedAmounts(memoryUsed, memoryPhysical, sum, {
      divisor: bytesPerGiB, eligibility: `${validOccupancy} and ${deviceMatch} ${memorySource}`,
    }),
  };
}

export function buildPhysicalResourceQueries(scope) {
  return {
    ...buildPhysicalAllocationQueries(scope),
    ...buildPhysicalUsageQueries(scope),
  };
}
