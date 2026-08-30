export const METRIC_STATUS = Object.freeze({
  LOADING: 'loading',
  READY: 'ready',
  MISSING: 'missing',
  INVALID: 'invalid',
  NO_CAPACITY: 'no-capacity',
  ERROR: 'error',
});

export const readInstantValue = (response) => {
  const rawValue = response?.data?.[0]?.value;
  if (rawValue === undefined || rawValue === null || rawValue === '') {
    return { status: METRIC_STATUS.MISSING, value: 0 };
  }

  const value = Number(rawValue);
  if (!Number.isFinite(value)) {
    return { status: METRIC_STATUS.INVALID, value: 0 };
  }

  return { status: METRIC_STATUS.READY, value };
};

export const calculateMetricPercent = (used, total) => {
  const numericUsed = Number(used);
  const numericTotal = Number(total);
  if (
    !Number.isFinite(numericUsed) ||
    !Number.isFinite(numericTotal) ||
    numericTotal <= 0
  ) {
    return undefined;
  }

  return (numericUsed / numericTotal) * 100;
};

export const requiresMetricCapacity = (config) =>
  Object.prototype.hasOwnProperty.call(config, 'percent');

export const setMetricErrorState = (
  metric,
  { hasTotal = false, requiresCapacity = false } = {},
) => {
  metric.hasData = undefined;
  metric.totalHasData = undefined;
  metric.count = 0;
  metric.used = 0;
  if (hasTotal) {
    metric.total = 0;
  }
  metric.status = METRIC_STATUS.ERROR;
  if (requiresCapacity) {
    metric.percent = 0;
  } else {
    delete metric.percent;
  }
  return metric;
};

export const resolveMetricStatus = ({
  usedStatus,
  totalStatus,
  total,
  requiresCapacity = false,
}) => {
  if (totalStatus && totalStatus !== METRIC_STATUS.READY) {
    return totalStatus;
  }
  if (usedStatus !== METRIC_STATUS.READY) {
    return usedStatus;
  }
  if (
    requiresCapacity &&
    (!Number.isFinite(Number(total)) || Number(total) <= 0)
  ) {
    return METRIC_STATUS.NO_CAPACITY;
  }
  return METRIC_STATUS.READY;
};
