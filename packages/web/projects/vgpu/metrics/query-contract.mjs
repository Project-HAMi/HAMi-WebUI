const METRICS = Object.freeze({
  computeAllocated: 'hami_container_vcore_allocated',
  computeAllocationKnown: 'hami_container_vcore_allocation_known',
  computeCapacity: 'hami_core_size',
  computeUsageAverage: 'hami_core_util_avg',
  memoryAllocated: 'hami_container_vmemory_allocated',
  memorySchedulableCapacity: 'hami_vmemory_size',
  memoryPhysicalCapacity: 'hami_memory_size',
  memoryUsed: 'hami_memory_used',
});

const PROMETHEUS_LABEL_NAME = /^[a-zA-Z_][a-zA-Z0-9_]*$/;
const REPORTING_DEVICE_MATCH = 'on (instance, node, provider, device_uuid)';

const metricSeries = (metric, selector = '') =>
  selector ? `${metric}{${selector}}` : metric;

const validateGroupLabel = (groupLabel) => {
  if (groupLabel && !PROMETHEUS_LABEL_NAME.test(groupLabel)) {
    throw new TypeError(`Invalid Prometheus group label: ${groupLabel}`);
  }
};

// Every WebUI backend replica exports the same cluster-wide device series.
// Sum within one scrape target first, then average equivalent replicas.
const aggregateAcrossExporterReplicas = (series, groupLabel = '') => {
  validateGroupLabel(groupLabel);
  const labels = groupLabel ? `${groupLabel}, instance` : 'instance';
  const perReplica = `sum by (${labels}) (${series})`;
  return groupLabel
    ? `avg by (${groupLabel}) (${perReplica})`
    : `avg(${perReplica})`;
};

const divideForDisplay = (expression, divisor) =>
  divisor === 1 ? expression : `${expression} / ${divisor}`;

const buildAllocationQueries = ({
  allocatedMetric,
  capacityMetric,
  selector = '',
  groupLabel = '',
  displayDivisor = 1,
}) => {
  const allocated = aggregateAcrossExporterReplicas(
    metricSeries(allocatedMetric, selector),
    groupLabel,
  );
  const capacity = aggregateAcrossExporterReplicas(
    metricSeries(capacityMetric, selector),
    groupLabel,
  );
  const join = groupLabel ? ` or on (${groupLabel}) ` : ' or ';
  const allocatedOrIdle = `(${allocated}${join}(${capacity} * 0))`;
  const percent = `(${allocated} / ${capacity} * 100)${join}(${capacity} * 0)`;

  return {
    query: divideForDisplay(allocatedOrIdle, displayDivisor),
    totalQuery: divideForDisplay(capacity, displayDivisor),
    percentQuery: percent,
  };
};

// Allocations whose compute share HAMi does not state, so a page can say how
// many its allocation rate leaves out. Counted per container and device, which
// every exporter replica reports identically.
export const buildUnknownComputeShareQuery = ({
  selector = '',
  groupLabel = '',
} = {}) => {
  validateGroupLabel(groupLabel);
  const identity = [...new Set([groupLabel, 'container_pod_uuid', 'device_uuid'].filter(Boolean))].join(', ');
  const unknown = `max by (${identity}) (${metricSeries(
    METRICS.computeAllocationKnown,
    selector,
  )} == 0)`;

  return groupLabel ? `count by (${groupLabel}) (${unknown})` : `count(${unknown})`;
};

// The exporter omits hami_container_vcore_allocated for an allocation whose
// compute share it cannot state, so these sums are a lower bound of what is
// allocated. They never overstate it, but free capacity read from them is only
// an upper bound and does not establish what can still be scheduled. Pair them
// with buildUnknownComputeShareQuery, and present the result as a lower bound
// whenever it counts anything.
export const buildComputeAllocationQueries = (options = {}) =>
  buildAllocationQueries({
    allocatedMetric: METRICS.computeAllocated,
    capacityMetric: METRICS.computeCapacity,
    ...options,
  });

export const buildMemoryAllocationQueries = (options = {}) =>
  buildAllocationQueries({
    allocatedMetric: METRICS.memoryAllocated,
    capacityMetric: METRICS.memorySchedulableCapacity,
    displayDivisor: 1024,
    ...options,
  });

export const buildMemoryUsageQueries = ({
  selector = '',
  groupLabel = '',
  displayDivisor = 1024,
} = {}) => {
  const usedSeries = metricSeries(METRICS.memoryUsed, selector);
  const capacitySeries = metricSeries(METRICS.memoryPhysicalCapacity, selector);
  const reportingUsedSeries = `${usedSeries} and ${REPORTING_DEVICE_MATCH} ${capacitySeries}`;
  const reportingCapacitySeries = `${capacitySeries} and ${REPORTING_DEVICE_MATCH} ${usedSeries}`;
  const used = aggregateAcrossExporterReplicas(
    reportingUsedSeries,
    groupLabel,
  );
  const capacity = aggregateAcrossExporterReplicas(
    reportingCapacitySeries,
    groupLabel,
  );

  return {
    query: divideForDisplay(used, displayDivisor),
    totalQuery: divideForDisplay(capacity, displayDivisor),
    // Missing provider data must remain absent. Unlike allocation, usage has no
    // capacity-derived zero fallback.
    percentQuery: `${used} / ${capacity} * 100`,
  };
};

// Count a node once per vendor, including nodes that report both Hygon device families.
export const nodeVendorDistributionQuery = 'count by (provider) ('
  + 'group by (node, provider) (hami_vgpu_count{provider!~"DCU|HCU|Hygon"}) or '
  + 'label_replace(group by (node) (hami_vgpu_count{provider=~"DCU|HCU|Hygon"}), "provider", "Hygon", "node", ".*"))';
