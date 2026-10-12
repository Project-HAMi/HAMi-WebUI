import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';

// How many allocations a compute allocation rate leaves out: a number once the
// count is read (a count that matches nothing returns no series, a confirmed
// zero), undefined while it loads, and null when it cannot be read.
export const readUncounted = (status, value) => {
  if (status === REQUEST_STATUS.READY) {
    const count = Number(value);
    return Number.isFinite(count) && count > 0 ? count : 0;
  }
  if (status === REQUEST_STATUS.MISSING) return 0;
  if (status === REQUEST_STATUS.LOADING) return undefined;
  return null;
};

// A current value cannot rest on the count its last refresh failed to replace.
export const readUncountedMetric = (metric) => (metric?.refreshError
  ? null
  : readUncounted(metric?.status, metric?.count));

// One sample that left allocations out makes the whole series a lower bound.
export const readUncountedRange = ({ status, data } = {}) => readUncounted(
  status,
  Math.max(0, ...(data || []).map((point) => Number(point?.value)).filter(Number.isFinite)),
);

// A trend reads as exact only once its own range, fetched with it, has confirmed
// zero; until then, and when that cannot be read, it stays a lower bound.
export const readTrendUncounted = (series) => readUncountedRange(series) ?? null;

// The rate never overstates, so it reads as a lower bound unless zero is confirmed.
export const isLowerBound = (uncounted) => uncounted === null || uncounted > 0;

// Nothing measured while allocations are left out: a bound of zero says nothing.
export const isNothingCounted = (measured, uncounted) => uncounted > 0 && Number(measured) === 0;

export const lowerBoundMessage = (t, uncounted) => (uncounted === null
  ? t('dashboard.metricLowerBoundUnknown')
  : t('dashboard.metricLowerBound', { count: uncounted }, uncounted));

export const nothingCountedMessage = (t, uncounted) => t('dashboard.metricNothingCounted', { count: uncounted }, uncounted);
