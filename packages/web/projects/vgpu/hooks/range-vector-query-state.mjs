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
  scope: null,
  pendingScope: null,
});

// What a group shows belongs to the queries it was read with, so a generation for
// other queries, such as another resource's, starts over instead of keeping it.
// Request ids keep counting, so a late reply for the old queries cannot land.
export const startRangeGroupGeneration = (group, range, scope = group.scope) => {
  const hasResolved = group.hasResolved && scope === group.scope;
  return {
    ...group,
    dataSource: hasResolved
      ? group.dataSource
      : group.dataSource.map((series) => createSeriesState(series)),
    hasResolved,
    requestId: group.requestId + 1,
    refreshing: hasResolved,
    refreshError: null,
    range: hasResolved ? group.range : null,
    pendingRange: { ...range },
    pendingScope: scope,
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

  // An optional series that fails settles as failed instead of holding the group back.
  const failedOutcome = outcomes.find((outcome, index) => outcome.failed && !group.dataSource[index]?.optional);
  // Only a chart that drew something, a line or a confirmed empty range, is kept through a
  // failed refresh; one that showed a failure shows the new failure rather than claim an earlier result.
  const required = group.dataSource.filter((series) => !series.optional);
  const hasResult = required.some((series) => series.status === REQUEST_STATUS.READY)
    || (required.length > 0 && required.every((series) => series.status === REQUEST_STATUS.MISSING));
  if (group.hasResolved && failedOutcome && hasResult) {
    return {
      ...group,
      refreshing: false,
      refreshError:
        failedOutcome.error || new Error('Range vector refresh failed'),
      pendingRange: null,
      pendingScope: null,
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
    scope: group.pendingScope,
    pendingScope: null,
  };
};
