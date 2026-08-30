import assert from 'node:assert/strict';
import test from 'node:test';

import { createRequestState, REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  classifyDetailPayload,
  hasDetailIdentity,
  isEmptyDetailPayload,
  rejectDetailResourceRequest,
  resolveDetailResourceRequest,
  startDetailResourceRequest,
} from './detail-resource-state.mjs';

test('detail identity requires every supported field to be non-empty', () => {
  assert.equal(hasDetailIdentity({ uuid: 'gpu-1' }, ['uuid']), true);
  assert.equal(hasDetailIdentity({ name: 'worker' }, ['name', 'uid']), false);
  assert.equal(
    hasDetailIdentity({ name: 'worker', uid: 'pod-1' }, ['name', 'uid']),
    true,
  );
  assert.equal(hasDetailIdentity({ uuid: '  ' }, ['uuid']), false);
  assert.equal(hasDetailIdentity({}, ['uuid']), false);
  assert.equal(hasDetailIdentity(null, ['uuid']), false);
});

test('protobuf zero-value replies are empty but partial entities are not', () => {
  assert.equal(isEmptyDetailPayload({}), true);
  assert.equal(isEmptyDetailPayload({ code: 0 }), true);
  assert.equal(
    isEmptyDetailPayload({
      code: 0,
      uuid: '',
      health: false,
      vgpuTotal: 0,
      type: '',
    }),
    true,
  );
  assert.equal(isEmptyDetailPayload({ code: 0, uuid: '' }), true);
  assert.equal(isEmptyDetailPayload({ code: 0, type: 'NVIDIA' }), false);
  assert.equal(isEmptyDetailPayload([]), false);
  assert.equal(isEmptyDetailPayload(null), false);
});

test('detail payloads distinguish missing, ready, and invalid responses', () => {
  assert.equal(
    classifyDetailPayload({ code: 0 }, { identityKeys: ['uuid'] }),
    REQUEST_STATUS.MISSING,
  );
  assert.equal(
    classifyDetailPayload(
      { code: 0, uuid: 'gpu-1' },
      { identityKeys: ['uuid'], expectedIdentity: { uuid: 'gpu-1' } },
    ),
    REQUEST_STATUS.READY,
  );
  assert.equal(
    classifyDetailPayload(
      {
        code: 0,
        uuid: 'gpu-1',
        health: false,
        vgpuUsed: 0,
        vgpuTotal: 0,
      },
      { identityKeys: ['uuid'], expectedIdentity: { uuid: 'gpu-1' } },
    ),
    REQUEST_STATUS.READY,
  );
  assert.equal(
    classifyDetailPayload(
      { code: 0, type: 'NVIDIA' },
      { identityKeys: ['uuid'] },
    ),
    REQUEST_STATUS.INVALID,
  );
  assert.equal(
    classifyDetailPayload(
      { code: 0, uuid: 'gpu-other' },
      { identityKeys: ['uuid'], expectedIdentity: { uuid: 'gpu-1' } },
    ),
    REQUEST_STATUS.INVALID,
  );
});

test('a successful empty detail is missing rather than ready', () => {
  const state = createRequestState({});
  const requestId = startDetailResourceRequest(state, {});

  resolveDetailResourceRequest(state, {
    data: {},
    initialData: {},
    status: REQUEST_STATUS.MISSING,
    requestId,
  });

  assert.equal(state.status, REQUEST_STATUS.MISSING);
  assert.deepEqual(state.data, {});
});

test('retry returns an error state to a clean loading state', () => {
  const state = createRequestState({});
  const firstRequest = startDetailResourceRequest(state, {});
  rejectDetailResourceRequest(state, new Error('unavailable'), firstRequest);

  assert.equal(state.status, REQUEST_STATUS.ERROR);

  startDetailResourceRequest(state, {});
  assert.equal(state.status, REQUEST_STATUS.LOADING);
  assert.equal(state.error, null);
  assert.equal(state.hasResolved, false);
});

test('an old detail response cannot replace the latest resource', () => {
  const state = createRequestState({});
  const oldRequest = startDetailResourceRequest(state, {});
  const latestRequest = startDetailResourceRequest(state, {});

  resolveDetailResourceRequest(state, {
    data: { uuid: 'gpu-new' },
    initialData: {},
    status: REQUEST_STATUS.READY,
    requestId: latestRequest,
  });
  assert.equal(state.data.uuid, 'gpu-new');

  assert.equal(
    resolveDetailResourceRequest(state, {
      data: { uuid: 'gpu-old' },
      initialData: {},
      status: REQUEST_STATUS.READY,
      requestId: oldRequest,
    }),
    false,
  );
  assert.equal(state.data.uuid, 'gpu-new');
});
