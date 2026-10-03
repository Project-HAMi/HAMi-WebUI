import { isLowerBound } from '../../../metrics/uncounted.mjs';

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
