import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import { normalizeRangeValues } from '../metrics/range-vector-state.mjs';

export const readRangeVector = (response) => {
  if (!Array.isArray(response?.data)) {
    return { data: [], status: REQUEST_STATUS.INVALID };
  }

  const values = normalizeRangeValues(response.data[0]?.values);
  if (!values.length) {
    return { data: [], status: REQUEST_STATUS.MISSING };
  }

  if (values.some(({ value }) => Number.isFinite(value))) {
    return { data: values, status: REQUEST_STATUS.READY };
  }

  const hasInvalidValue = values.some(
    ({ value, missing }) => value === null && missing !== true,
  );
  return {
    data: values,
    status: hasInvalidValue
      ? REQUEST_STATUS.INVALID
      : REQUEST_STATUS.MISSING,
  };
};

const createSeriesState = (series, options = {}) => ({
  ...series,
  data: options.data || [],
  status: options.status || REQUEST_STATUS.LOADING,
  hasResolved: options.hasResolved || false,
  error: options.error || null,
});

export const createRangeVectorOutcome = (state, error = null) => ({
  state,
  error,
  failed:
    state?.status === REQUEST_STATUS.ERROR ||
    state?.status === REQUEST_STATUS.INVALID,
});

export const createRangeGroupState = (dataSource, key) => ({
  key,
  dataSource: dataSource.map((series) => createSeriesState(series)),
  requestId: 0,
  hasResolved: false,
  refreshing: false,
  refreshError: null,
  range: null,
  pendingRange: null,
});

export const startRangeGroupGeneration = (group, range) => {
  const hasResolved = group.hasResolved;
  return {
    ...group,
    dataSource: hasResolved
      ? group.dataSource
      : group.dataSource.map((series) => createSeriesState(series)),
    requestId: group.requestId + 1,
    refreshing: hasResolved,
    refreshError: null,
    pendingRange: { ...range },
  };
};

export const publishInitialRangeOutcome = (
  group,
  requestId,
  seriesIndex,
  outcome,
) => {
  if (group.requestId !== requestId || group.hasResolved) return group;

  return {
    ...group,
    dataSource: group.dataSource.map((series, index) =>
      index === seriesIndex
        ? createSeriesState(series, {
            ...outcome.state,
            hasResolved: true,
            error: outcome.error,
          })
        : series,
    ),
  };
};

export const settleRangeGroupGeneration = (
  group,
  requestId,
  outcomes,
) => {
  if (group.requestId !== requestId) return group;

  const failedOutcome = outcomes.find((outcome) => outcome.failed);
  if (group.hasResolved && failedOutcome) {
    return {
      ...group,
      refreshing: false,
      refreshError:
        failedOutcome.error || new Error('Range vector refresh failed'),
      pendingRange: null,
    };
  }

  return {
    ...group,
    dataSource: group.dataSource.map((series, index) => {
      const outcome = outcomes[index];
      return createSeriesState(series, {
        ...outcome.state,
        hasResolved: true,
        error: outcome.error,
      });
    }),
    hasResolved: true,
    refreshing: false,
    refreshError: null,
    range: group.pendingRange,
    pendingRange: null,
  };
};
