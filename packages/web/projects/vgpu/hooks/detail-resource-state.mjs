import {
  isLatestRequest,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from '../../../src/hooks/request-state.mjs';

export const hasDetailIdentity = (payload, identityKeys) => {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return false;
  }

  return identityKeys.every((key) => {
    const value = payload[key];
    return value !== undefined && value !== null && String(value).trim() !== '';
  });
};

const RESPONSE_METADATA_KEYS = new Set(['code', 'message', 'reason']);

const isEmptyProtoValue = (value) => {
  if (value === undefined || value === null || value === '' || value === false) {
    return true;
  }
  if (typeof value === 'number') return value === 0;
  if (Array.isArray(value)) return value.length === 0;
  return false;
};

export const isEmptyDetailPayload = (payload) => {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    return false;
  }

  return Object.entries(payload).every(
    ([key, value]) => RESPONSE_METADATA_KEYS.has(key) || isEmptyProtoValue(value),
  );
};

export const classifyDetailPayload = (
  payload,
  { identityKeys, expectedIdentity = {} },
) => {
  if (hasDetailIdentity(payload, identityKeys)) {
    const matchesExpectedIdentity = Object.entries(expectedIdentity).every(
      ([key, value]) => String(payload[key]) === String(value),
    );
    return matchesExpectedIdentity
      ? REQUEST_STATUS.READY
      : REQUEST_STATUS.INVALID;
  }
  if (isEmptyDetailPayload(payload)) return REQUEST_STATUS.MISSING;
  return REQUEST_STATUS.INVALID;
};

export const startDetailResourceRequest = (state, initialData) => {
  state.data = initialData;
  state.hasResolved = false;
  state.refreshing = false;
  return startRequest(state, { hasResolved: false });
};

export const resolveDetailResourceRequest = (
  state,
  { data, status, requestId, initialData },
) => {
  if (!isLatestRequest(state, requestId)) return false;

  return resolveRequest(state, {
    data: status === REQUEST_STATUS.READY ? data : initialData,
    status,
    requestId,
  });
};

export const rejectDetailResourceRequest = (state, error, requestId) =>
  rejectRequest(state, error, { hasResolved: false, requestId });
