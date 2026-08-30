const METRICS = Object.freeze({
  computeAllocated: 'hami_container_vcore_allocated',
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
  const reportingCapacitySeries = `${capacitySeries} and ${REPORTING_DEVICE_MATCH} ${usedSeries}`;
  const used = aggregateAcrossExporterReplicas(usedSeries, groupLabel);
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

export const buildGroupedResourceTopQueries = (groupLabel) => {
  validateGroupLabel(groupLabel);
  if (!groupLabel) {
    throw new TypeError('A Prometheus group label is required');
  }

  const computeAllocation = buildComputeAllocationQueries({ groupLabel });
  const memoryAllocation = buildMemoryAllocationQueries({ groupLabel });
  const memoryUsage = buildMemoryUsageQueries({ groupLabel });

  return {
    computeAllocation: `topk(5, ${computeAllocation.percentQuery})`,
    computeUsage: `topk(5, avg by (${groupLabel}) (${METRICS.computeUsageAverage}))`,
    memoryAllocation: `topk(5, ${memoryAllocation.percentQuery})`,
    memoryUsage: `topk(5, ${memoryUsage.percentQuery})`,
  };
};

export const buildClusterTrendQueries = () => ({
  computeAllocation: buildComputeAllocationQueries().percentQuery,
  computeUsage: `avg(${METRICS.computeUsageAverage})`,
  memoryAllocation: buildMemoryAllocationQueries().percentQuery,
  memoryUsage: buildMemoryUsageQueries().percentQuery,
});
