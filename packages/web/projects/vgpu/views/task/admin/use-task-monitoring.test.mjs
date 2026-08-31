import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { nextTick, ref } from 'vue';

import { REQUEST_STATUS } from '../../../../../src/hooks/request-state.mjs';
import useTaskMonitoring, {
  getTaskMonitoringAllocationShape,
} from './useTaskMonitoring.js';

const sourceFixture = (overrides = {}) => ({
  container: 'worker',
  expectedDeviceCount: 2,
  expectedVgpuCount: 3,
  namespace: 'research',
  pod: 'training-pod',
  podUid: 'pod-uid-current',
  ...overrides,
});

const rangeFixture = (overrides = {}) => ({
  end: '2026-09-01 11:00:00',
  start: '2026-09-01 10:00:00',
  step: '30s',
  ...overrides,
});

const deferred = () => {
  let reject;
  let resolve;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
};

const flushPromises = () => new Promise((resolve) => setImmediate(resolve));

test('task monitoring is driven by metric coverage rather than the first device model', () => {
  const detail = readFileSync(new URL('./Detail.vue', import.meta.url), 'utf8');

  assert.match(detail, /useTaskMonitoring/);
  assert.match(detail, /getTaskMonitoringAllocationShape/);
  assert.doesNotMatch(detail, /supportsTaskMonitoring|primaryDeviceType/);
  assert.doesNotMatch(detail, /startsWith\(['"](?:NVIDIA|MXC)/);
});

test('task monitoring distinguishes unique device identities from vGPU slots', () => {
  assert.deepEqual(
    getTaskMonitoringAllocationShape({
      allocatedDevices: 2,
      deviceIds: ['GPU-0', 'GPU-1'],
    }),
    { expectedDeviceCount: 2, expectedVgpuCount: 2 },
  );
  assert.deepEqual(
    getTaskMonitoringAllocationShape({
      allocatedDevices: 2,
      deviceIds: ['GPU-0', 'GPU-0'],
    }),
    { expectedDeviceCount: 1, expectedVgpuCount: 2 },
  );
  for (const detail of [
    { allocatedDevices: 2, deviceIds: ['GPU-0'] },
    { allocatedDevices: 1, deviceIds: [''] },
    { allocatedDevices: 0, deviceIds: [] },
  ]) {
    assert.equal(getTaskMonitoringAllocationShape(detail), null);
  }
});

test('task monitoring binds queries to the current Pod lifecycle and preserves real zeroes', async () => {
  const calls = [];
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture()),
    range: ref(rangeFixture()),
    request: ({ query, range }) => {
      calls.push({ query, range });
      return Promise.resolve({
        data: [{ values: [{ timestamp: 1_000, value: 0 }] }],
      });
    },
  });

  await flushPromises();

  assert.equal(calls.length, 2);
  for (const call of calls) {
    assert.match(call.query, /container_pod_uuid="worker:pod-uid-current"/);
    assert.match(call.query, /count\([^]*hami_container_vgpu_allocated[^]*\) == 2/);
    assert.match(call.query, /sum\([^]*hami_container_vgpu_allocated[^]*\) == 3/);
    assert.doesNotMatch(call.query, /\bbool\b/);
    assert.deepEqual(call.range, rangeFixture());
  }
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.READY);
    assert.equal(item.data[0].value, 0);
  }
});

test('an empty complete-coverage query result is missing instead of a synthetic zero', async () => {
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture()),
    range: ref(rangeFixture()),
    request: async () => ({ data: [] }),
  });

  await flushPromises();

  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.MISSING);
    assert.deepEqual(item.data, []);
  }
});

test('an inconsistent detail allocation shape does not issue monitoring queries', async () => {
  let calls = 0;
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture({ expectedVgpuCount: 1 })),
    range: ref(rangeFixture()),
    request: async () => {
      calls += 1;
      return { data: [] };
    },
  });

  await flushPromises();

  assert.equal(calls, 0);
  assert.ok(monitoring.data.value.every(
    (item) => item.status === REQUEST_STATUS.MISSING,
  ));
});

test('changing the time range clears old charts and exposes query failures', async () => {
  const range = ref(rangeFixture());
  const pending = [];
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture()),
    range,
    request: () => {
      const request = deferred();
      pending.push(request);
      return request.promise;
    },
  });

  assert.equal(pending.length, 2);
  for (const request of pending.slice(0, 2)) {
    request.resolve({
      data: [{ values: [{ timestamp: 1_000, value: 42 }] }],
    });
  }
  await flushPromises();
  assert.ok(monitoring.data.value.every((item) => item.status === REQUEST_STATUS.READY));

  range.value = rangeFixture({ start: '2026-08-25 11:00:00' });
  await nextTick();
  assert.equal(pending.length, 4);
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.LOADING);
    assert.deepEqual(item.data, []);
  }

  for (const request of pending.slice(2, 4)) {
    request.reject(new Error('Prometheus unavailable'));
  }
  await flushPromises();
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.ERROR);
    assert.deepEqual(item.data, []);
  }
});

test('a response for an older workload cannot overwrite the current workload', async () => {
  const source = ref(sourceFixture({ podUid: 'pod-old' }));
  const pending = [];
  const monitoring = useTaskMonitoring({
    source,
    range: ref(rangeFixture()),
    request: () => {
      const request = deferred();
      pending.push(request);
      return request.promise;
    },
  });

  source.value = sourceFixture({ podUid: 'pod-new' });
  await nextTick();
  assert.equal(pending.length, 4);

  for (const request of pending.slice(2, 4)) {
    request.resolve({
      data: [{ values: [{ timestamp: 2_000, value: 77 }] }],
    });
  }
  await flushPromises();
  assert.ok(monitoring.data.value.every((item) => item.data[0].value === 77));

  for (const request of pending.slice(0, 2)) {
    request.resolve({
      data: [{ values: [{ timestamp: 1_000, value: 12 }] }],
    });
  }
  await flushPromises();
  assert.ok(monitoring.data.value.every((item) => item.data[0].value === 77));
});
