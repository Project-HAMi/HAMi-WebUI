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

test('switching entities clears ready data and isolates late rejections', async () => {
  const source = ref('gpu-a');
  const requests = new Map();
  const state = useDetailResource({
    source,
    request: (key) => new Promise((resolve, reject) => {
      requests.set(key, { resolve, reject });
    }),
    classify: (payload, key) => classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: key },
    }),
  });

  requests.get('gpu-a').resolve({ uuid: 'gpu-a', label: 'A' });
  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.READY);
  assert.equal(state.data.value.uuid, 'gpu-a');

  source.value = 'gpu-b';
  await nextTick();
  assert.equal(state.status.value, REQUEST_STATUS.LOADING);
  assert.deepEqual(state.data.value, {});

  requests.get('gpu-b').reject(new Error('gpu-b failed'));
  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.ERROR);
  assert.deepEqual(state.data.value, {});
});

test('switching entities aborts the in-flight AbortSignal and suppresses abort rejections', async () => {
  const source = ref('gpu-a');
  const signals = new Map();
  const requests = new Map();
  const state = useDetailResource({
    source,
    request: (key, signal) => new Promise((resolve, reject) => {
      signals.set(key, signal);
      requests.set(key, { resolve, reject });
    }),
    classify: (payload, key) => classifyDetailPayload(payload, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: key },
    }),
  });

  assert.equal(signals.get('gpu-a').aborted, false);
  source.value = 'gpu-b';
  await nextTick();
  assert.equal(signals.get('gpu-a').aborted, true);
  assert.equal(signals.get('gpu-b').aborted, false);

  requests.get('gpu-b').resolve({ uuid: 'gpu-b' });
  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.READY);
  assert.equal(state.data.value.uuid, 'gpu-b');

  requests.get('gpu-a').reject(new DOMException('Aborted', 'AbortError'));
  await flushPromises();
  assert.equal(state.status.value, REQUEST_STATUS.READY);
  assert.equal(state.data.value.uuid, 'gpu-b');
});
