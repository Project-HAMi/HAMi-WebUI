import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createRequestState,
  isLatestRequest,
  rejectRequest,
  REQUEST_STATUS,
  resolveRequest,
  startRequest,
} from './request-state.mjs';

test('a request starts in loading and resolves a real empty collection', () => {
  const state = createRequestState([]);

  assert.equal(state.status, REQUEST_STATUS.LOADING);
  resolveRequest(state, { data: [], status: REQUEST_STATUS.READY });

  assert.equal(state.status, REQUEST_STATUS.READY);
  assert.deepEqual(state.data, []);
  assert.equal(state.hasResolved, true);
});

test('an initial failure is distinct from an empty successful response', () => {
  const state = createRequestState([]);
  const error = new Error('unavailable');

  rejectRequest(state, error, { hasResolved: false });

  assert.equal(state.status, REQUEST_STATUS.ERROR);
  assert.equal(state.error, error);
  assert.equal(state.hasResolved, true);
});

test('refresh keeps the last resolved content when the request fails', () => {
  const state = createRequestState([]);
  resolveRequest(state, {
    data: [{ value: 0 }],
    status: REQUEST_STATUS.READY,
  });

  startRequest(state, { hasResolved: true });
  assert.equal(state.status, REQUEST_STATUS.READY);
  assert.equal(state.refreshing, true);

  const error = new Error('refresh failed');
  rejectRequest(state, error, { hasResolved: true });

  assert.equal(state.status, REQUEST_STATUS.READY);
  assert.deepEqual(state.data, [{ value: 0 }]);
  assert.equal(state.refreshing, false);
  assert.equal(state.refreshError, error);
});

test('an older response cannot overwrite a newer request', () => {
  const state = createRequestState([]);
  const oldRequest = startRequest(state);
  const newRequest = startRequest(state);

  assert.equal(isLatestRequest(state, oldRequest), false);
  assert.equal(isLatestRequest(state, newRequest), true);
  assert.equal(
    resolveRequest(state, {
      data: [{ value: 1 }],
      requestId: oldRequest,
    }),
    false,
  );
  assert.deepEqual(state.data, []);

  resolveRequest(state, {
    data: [{ value: 2 }],
    requestId: newRequest,
  });
  assert.deepEqual(state.data, [{ value: 2 }]);
});

test('an older failure cannot overwrite a newer successful request', () => {
  const state = createRequestState([]);
  const oldRequest = startRequest(state);
  const newRequest = startRequest(state);

  resolveRequest(state, {
    data: [{ value: 2 }],
    requestId: newRequest,
  });
  assert.equal(
    rejectRequest(state, new Error('late failure'), {
      hasResolved: false,
      requestId: oldRequest,
    }),
    false,
  );

  assert.equal(state.status, REQUEST_STATUS.READY);
  assert.deepEqual(state.data, [{ value: 2 }]);
  assert.equal(state.error, null);
});

test('an invalid initial response is blocking but an invalid refresh preserves rows', () => {
  const state = createRequestState([]);
  const invalidInitial = new TypeError('invalid response');

  rejectRequest(state, invalidInitial, {
    hasResolved: false,
    status: REQUEST_STATUS.INVALID,
  });
  assert.equal(state.status, REQUEST_STATUS.INVALID);
  assert.equal(state.error, invalidInitial);

  resolveRequest(state, {
    data: [{ value: 1 }],
    status: REQUEST_STATUS.READY,
  });
  const requestId = startRequest(state, { hasResolved: true });
  const invalidRefresh = new TypeError('invalid refresh');
  rejectRequest(state, invalidRefresh, {
    hasResolved: true,
    requestId,
    status: REQUEST_STATUS.INVALID,
  });

  assert.equal(state.status, REQUEST_STATUS.READY);
  assert.deepEqual(state.data, [{ value: 1 }]);
  assert.equal(state.refreshError, invalidRefresh);
});

test('a failed refresh remains visible after an empty successful result', () => {
  const state = createRequestState([]);
  resolveRequest(state, { status: REQUEST_STATUS.MISSING });

  const requestId = startRequest(state);
  const error = new Error('refresh failed');
  rejectRequest(state, error, { requestId });

  assert.equal(state.status, REQUEST_STATUS.MISSING);
  assert.equal(state.refreshing, false);
  assert.equal(state.refreshError, error);
});
