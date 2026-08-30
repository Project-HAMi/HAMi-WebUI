export const REQUEST_STATUS = Object.freeze({
  LOADING: 'loading',
  READY: 'ready',
  MISSING: 'missing',
  INVALID: 'invalid',
  ERROR: 'error',
});

export const createRequestState = (data) => ({
  data,
  status: REQUEST_STATUS.LOADING,
  hasResolved: false,
  refreshing: false,
  error: null,
  refreshError: null,
  requestId: 0,
});

export const startRequest = (
  state,
  { hasResolved = state.hasResolved } = {},
) => {
  state.requestId += 1;
  if (hasResolved) {
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
  { hasResolved = state.hasResolved, requestId } = {},
) => {
  if (requestId !== undefined && !isLatestRequest(state, requestId)) {
    return false;
  }
  state.refreshing = false;
  if (hasResolved) {
    state.refreshError = error;
  } else {
    state.status = REQUEST_STATUS.ERROR;
    state.hasResolved = true;
    state.error = error;
    state.refreshError = null;
  }
  return true;
};
