import assert from 'node:assert/strict';
import test from 'node:test';

import { nextTick, ref } from 'vue';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import { classifyDetailPayload } from './detail-resource-state.mjs';
import useDetailResource from './useDetailResource.js';

const deferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
};

const flushPromises = () => new Promise((resolve) => setImmediate(resolve));

test('route changes reset loading and ignore an older detail response', async () => {
  const source = ref('gpu-old');
  const requests = new Map();
  const state = useDetailResource({
    source,
    request: (key) => {
      const pending = deferred();
      requests.set(key, pending);
      return pending.promise;
    },
    classify: (payload, key) => classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: key },
    }),
  });

  assert.equal(state.status.value, REQUEST_STATUS.LOADING);

  source.value = 'gpu-new';
  await nextTick();
  assert.deepEqual(state.data.value, {});
  assert.equal(state.status.value, REQUEST_STATUS.LOADING);

  requests.get('gpu-new').resolve({ uuid: 'gpu-new' });
  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.READY);
  assert.equal(state.data.value.uuid, 'gpu-new');

  requests.get('gpu-old').resolve({ uuid: 'gpu-old' });
  await flushPromises();
  assert.equal(state.data.value.uuid, 'gpu-new');
});

test('an initial error can be retried without retaining stale content', async () => {
  const source = ref('gpu-1');
  let attempts = 0;
  const state = useDetailResource({
    source,
    request: async () => {
      attempts += 1;
      if (attempts === 1) throw new Error('unavailable');
      return { uuid: 'gpu-1' };
    },
    classify: (payload, key) => classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: key },
    }),
  });

  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.ERROR);

  const retry = state.retry();
  assert.equal(state.status.value, REQUEST_STATUS.LOADING);
  assert.deepEqual(state.data.value, {});
  await retry;

  assert.equal(state.status.value, REQUEST_STATUS.READY);
  assert.equal(state.data.value.uuid, 'gpu-1');
});

test('an invalid source becomes invalid without issuing a request', async () => {
  const source = ref('');
  let calls = 0;
  const state = useDetailResource({
    source,
    request: async () => {
      calls += 1;
      return { uuid: 'unexpected' };
    },
    classify: (payload, key) => classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: key },
    }),
  });

  await nextTick();
  assert.equal(calls, 0);
  assert.equal(state.status.value, REQUEST_STATUS.INVALID);
});
