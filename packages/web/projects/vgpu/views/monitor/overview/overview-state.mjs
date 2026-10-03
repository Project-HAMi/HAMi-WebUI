import { REQUEST_STATUS } from '../../../../../src/hooks/request-state.mjs';
import { isLowerBound } from '../../../metrics/uncounted.mjs';

export const aggregateStatuses = (items = []) => {
  const statuses = items.map((item) => item?.status).filter(Boolean);
  if (statuses.includes(REQUEST_STATUS.READY)) return REQUEST_STATUS.READY;
  if (statuses.includes(REQUEST_STATUS.LOADING)) return REQUEST_STATUS.LOADING;
  if (statuses.includes(REQUEST_STATUS.ERROR)) return REQUEST_STATUS.ERROR;
  if (statuses.includes(REQUEST_STATUS.INVALID)) return REQUEST_STATUS.INVALID;
  if (statuses.includes('no-capacity')) return 'no-capacity';
  return REQUEST_STATUS.MISSING;
};

// The compute allocation rate leaves out the allocations whose share HAMi does
// not state, so it is a lower bound and says how many it left out (null when
// that count cannot be read). Gauges carry an id, trend series a key.
export const applyUncountedShares = (metrics = [], uncounted, { pendingStatus } = {}) => (
  metrics.map((metric) => {
    if ((metric?.id ?? metric?.key) !== 'compute-allocation') return metric;
    if (uncounted === undefined) return pendingStatus ? { ...metric, status: pendingStatus } : metric;
    return isLowerBound(uncounted) ? { ...metric, uncounted } : metric;
  })
);

export const stateTextKey = (status, { metric = true } = {}) => {
  if (status === REQUEST_STATUS.LOADING) return 'common.loading';
  if (status === REQUEST_STATUS.ERROR) {
    return metric ? 'dashboard.metricQueryFailed' : 'common.requestError';
  }
  if (status === REQUEST_STATUS.INVALID) {
    return metric ? 'dashboard.metricInvalid' : 'common.requestError';
  }
  if (status === 'no-capacity') return 'dashboard.metricNoCapacity';
  return metric ? 'dashboard.metricNoData' : 'common.noData';
};

export const getPartialRangeStates = (items = []) => {
  if (aggregateStatuses(items) !== REQUEST_STATUS.READY) return [];
  return items.filter((item) => item?.status !== REQUEST_STATUS.READY);
};

export const selectRangeAxisData = (items = []) => {
  const hasData = (item) => Array.isArray(item?.data) && item.data.length > 0;
  return (
    items.find(
      (item) => item?.status === REQUEST_STATUS.READY && hasData(item),
    )?.data ||
    items.find(hasData)?.data ||
    []
  );
};
