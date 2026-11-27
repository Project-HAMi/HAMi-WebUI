// The rate never overstates, so it reads as a lower bound unless zero is confirmed.
export const isLowerBound = (uncounted) => uncounted === null || uncounted > 0;

// Nothing measured while allocations are left out: a bound of zero says nothing.
export const isNothingCounted = (measured, uncounted) => uncounted > 0 && Number(measured) === 0;

export const lowerBoundMessage = (t, uncounted) => (uncounted === null
  ? t('dashboard.metricLowerBoundUnknown')
  : t('dashboard.metricLowerBound', { count: uncounted }, uncounted));

export const nothingCountedMessage = (t, uncounted) => t('dashboard.metricNothingCounted', { count: uncounted }, uncounted);
