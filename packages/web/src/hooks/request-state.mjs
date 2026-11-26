export const REQUEST_STATUS = Object.freeze({
  LOADING: 'loading',
  READY: 'ready',
  MISSING: 'missing',
  INVALID: 'invalid',
  ERROR: 'error',
});

const cloneFallbackData = (value) => {
  if (Array.isArray(value)) return [];
  if (value && typeof value === 'object') return {};
  return value;
};

export const createRequestState = (data, { entityKey } = {}) => ({
  data,
  initialData: cloneFallbackData(data),
  entityKey,
  status: REQUEST_STATUS.LOADING,
  hasResolved: false,
  refreshing: false,
  error: null,
  refreshError: null,
  requestId: 0,
});

export const startRequest = (
  state,
  { hasResolved = state.hasResolved, entityKey } = {},
) => {
  state.requestId += 1;
  const entityChanged =
    entityKey !== undefined &&
    state.entityKey !== undefined &&
    entityKey !== state.entityKey;
  if (entityKey !== undefined) {
    state.entityKey = entityKey;
  }
  if (entityChanged) {
    state.data = cloneFallbackData(state.initialData ?? state.data);
    state.hasResolved = false;
    state.refreshing = false;
    state.status = REQUEST_STATUS.LOADING;
  } else if (hasResolved) {
    state.refreshing = true;
  } else {
    state.status = REQUEST_STATUS.LOADING;
  }
  state.error = null;
  state.refreshError = null;
  return state.requestId;
};

export const isLatestRequest = (state, requestId) =>
  requestId === state.requestId;

export const resolveRequest = (state, options = {}) => {
  if (
    options.requestId !== undefined &&
    !isLatestRequest(state, options.requestId)
  ) {
    return false;
  }
  if (
    options.entityKey !== undefined &&
    state.entityKey !== undefined &&
    options.entityKey !== state.entityKey
  ) {
    return false;
  }
  if (Object.prototype.hasOwnProperty.call(options, 'data')) {
    state.data = options.data;
  }
  state.status = options.status || REQUEST_STATUS.READY;
  state.hasResolved = true;
  state.refreshing = false;
  state.error = null;
  state.refreshError = null;
  return true;
};

export const rejectRequest = (
  state,
  error,
  {
    hasResolved,
    requestId,
    entityKey,
    status = REQUEST_STATUS.ERROR,
  } = {},
) => {
  if (requestId !== undefined && !isLatestRequest(state, requestId)) {
    return false;
  }
  if (
    entityKey !== undefined &&
    state.entityKey !== undefined &&
    entityKey !== state.entityKey
  ) {
    return false;
  }
  const keepResolved =
    hasResolved !== undefined ? hasResolved : state.hasResolved;
  state.refreshing = false;
  if (keepResolved) {
    state.refreshError = error;
  } else {
    state.status = status;
    state.hasResolved = true;
    state.error = error;
    state.refreshError = null;
  }
  return true;
};
