export const METRIC_STATUS = Object.freeze({
  LOADING: 'loading',
  READY: 'ready',
  MISSING: 'missing',
  INVALID: 'invalid',
  NO_CAPACITY: 'no-capacity',
  ERROR: 'error',
});

export const PARSED_QUERY_STATUS = Object.freeze({
  READY: 'ready',
  PENDING: 'pending',
  INVALID: 'invalid',
});

export const classifyParsedQuery = (query) => {
  if (typeof query !== 'string' || !query.trim()) {
    return PARSED_QUERY_STATUS.INVALID;
  }
  if (query.includes('undefined')) {
    return PARSED_QUERY_STATUS.PENDING;
  }
  return PARSED_QUERY_STATUS.READY;
};

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

export const readReadyMetricField = (metric, field) => {
  if (metric?.status !== METRIC_STATUS.READY) {
    return undefined;
  }

  const value = Number(metric?.[field]);
  return Number.isFinite(value) ? value : undefined;
};

const METRIC_SAMPLE_FIELDS = ['count', 'used', 'total', 'percent'];

export const resetMetricQueryState = (
  metric,
  config,
  { requestId, status },
) => {
  metric.data = config.data;
  for (const field of METRIC_SAMPLE_FIELDS) {
    if (Object.prototype.hasOwnProperty.call(config, field)) {
      metric[field] = config[field];
    } else {
      delete metric[field];
    }
  }
  delete metric.hasData;
  delete metric.totalHasData;
  metric.status = status;
  metric.hasResolved = status !== METRIC_STATUS.LOADING;
  metric.refreshing = false;
  metric.error = null;
  metric.refreshError = null;
  metric.requestId = requestId;
  return metric;
};

export const snapshotMetricState = (metric) => ({
  status: metric.status,
  hasData: metric.hasData,
  totalHasData: metric.totalHasData,
  count: metric.count,
  used: metric.used,
  total: metric.total,
  percent: metric.percent,
});

export const restoreMetricState = (metric, snapshot) => {
  Object.assign(metric, snapshot);
  if (snapshot.percent === undefined) delete metric.percent;
  return metric;
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
