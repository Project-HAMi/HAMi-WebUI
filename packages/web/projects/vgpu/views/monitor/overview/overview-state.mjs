// The compute allocation rate leaves out the allocations whose share HAMi does
// not state, so it is a lower bound and says how many it left out. Gauges carry
// an id, trend series a key.
export const applyUncountedShares = (metrics = [], uncounted = 0) => (
  metrics.map((metric) => (
    (metric?.id ?? metric?.key) === 'compute-allocation' && uncounted > 0
      ? { ...metric, uncounted }
      : metric
  ))
);
