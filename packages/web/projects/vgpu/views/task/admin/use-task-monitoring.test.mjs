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

test('changing the time range keeps the last result until each chart settles', async () => {
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
  const resolveWith = (request, value) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value }] }] });

  pending.slice(0, 2).forEach((request) => resolveWith(request, 42));
  await flushPromises();
  assert.ok(monitoring.data.value.every((item) => item.status === REQUEST_STATUS.READY && !item.refreshing));

  range.value = rangeFixture({ start: '2026-08-25 11:00:00' });
  await nextTick();
  assert.equal(pending.length, 4);
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.READY);
    assert.equal(item.data[0].value, 42);
    assert.equal(item.refreshing, true);
  }

  // One chart's refresh fails and keeps its last result; the other updates on its own.
  pending[2].reject(new Error('Prometheus unavailable'));
  resolveWith(pending[3], 55);
  await flushPromises();
  const [failed, updated] = monitoring.data.value;
  assert.equal(failed.status, REQUEST_STATUS.READY);
  assert.equal(failed.data[0].value, 42);
  assert.equal(failed.refreshing, false);
  assert.match(failed.refreshError.message, /Prometheus unavailable/);
  assert.equal(updated.data[0].value, 55);
  assert.equal(updated.refreshError, null);

  range.value = rangeFixture({ start: '2026-08-25 12:00:00' });
  await nextTick();
  pending.slice(4, 6).forEach((request) => resolveWith(request, 60));
  await flushPromises();
  assert.ok(monitoring.data.value.every((item) => item.data[0].value === 60 && item.refreshError === null));
});

test('a first load that fails still shows the error with nothing to keep', async () => {
  const pending = [];
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture()),
    range: ref(rangeFixture()),
    request: () => {
      const request = deferred();
      pending.push(request);
      return request.promise;
    },
  });

  pending.forEach((request) => request.reject(new Error('Prometheus unavailable')));
  await flushPromises();
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.ERROR);
    assert.deepEqual(item.data, []);
    assert.equal(item.refreshError, null);
  }
});

test('a refresh with unusable data keeps the last result, and an empty result replaces it', async () => {
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
  pending.slice(0, 2).forEach((request) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value: 42 }] }] }));
  await flushPromises();

  range.value = rangeFixture({ start: '2026-08-25 11:00:00' });
  await nextTick();
  pending[2].resolve({ data: [{ values: [{ timestamp: 'bad', value: 'not a number' }] }] });
  pending[3].resolve({ data: [] });
  await flushPromises();
  const [invalid, empty] = monitoring.data.value;
  assert.equal(invalid.status, REQUEST_STATUS.READY);
  assert.equal(invalid.data[0].value, 42);
  assert.ok(invalid.refreshError);
  assert.equal(empty.status, REQUEST_STATUS.MISSING);
  assert.deepEqual(empty.data, []);
  assert.equal(empty.refreshError, null);
});

test('returning to a workload before the other one settles starts over', async () => {
  const source = ref(sourceFixture({ podUid: 'pod-a' }));
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
  pending.slice(0, 2).forEach((request) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value: 42 }] }] }));
  await flushPromises();

  source.value = sourceFixture({ podUid: 'pod-b' });
  await nextTick();
  source.value = sourceFixture({ podUid: 'pod-a' });
  await nextTick();
  assert.equal(pending.length, 6);
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.LOADING);
    assert.equal(item.refreshing, false);
  }

  // B's late replies are dropped, and A's failure is a first-load failure, not a kept skeleton.
  pending.slice(2, 4).forEach((request) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value: 7 }] }] }));
  pending.slice(4, 6).forEach((request) => request.reject(new Error('Prometheus unavailable')));
  await flushPromises();
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.ERROR);
    assert.equal(item.refreshError, null);
  }
});

test('a quick second range change drops the replies to the first', async () => {
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
  const resolveWith = (request, value) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value }] }] });
  pending.slice(0, 2).forEach((request) => resolveWith(request, 42));
  await flushPromises();

  range.value = rangeFixture({ start: '2026-08-25 11:00:00' });
  await nextTick();
  range.value = rangeFixture({ start: '2026-08-25 12:00:00' });
  await nextTick();
  pending.slice(4, 6).forEach((request) => resolveWith(request, 60));
  await flushPromises();
  pending.slice(2, 4).forEach((request) => resolveWith(request, 11));
  await flushPromises();
  for (const item of monitoring.data.value) {
    assert.equal(item.data[0].value, 60);
    assert.equal(item.refreshing, false);
  }
});

test('retrying a first load that failed does not claim an earlier result', async () => {
  const pending = [];
  const monitoring = useTaskMonitoring({
    source: ref(sourceFixture()),
    range: ref(rangeFixture()),
    request: () => {
      const request = deferred();
      pending.push(request);
      return request.promise;
    },
  });
  pending.slice(0, 2).forEach((request) => request.reject(new Error('Prometheus unavailable')));
  await flushPromises();

  void monitoring.refresh();
  await nextTick();
  pending.slice(2, 4).forEach((request) => request.reject(new Error('still unavailable')));
  await flushPromises();
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.ERROR);
    assert.match(item.error.message, /still unavailable/);
    assert.equal(item.refreshError, null);
    assert.equal(item.refreshing, false);
  }
});

test('another workload starts over instead of showing the previous one', async () => {
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
  pending.forEach((request) => request.resolve({ data: [{ values: [{ timestamp: 1_000, value: 42 }] }] }));
  await flushPromises();

  source.value = sourceFixture({ podUid: 'pod-new' });
  await nextTick();
  for (const item of monitoring.data.value) {
    assert.equal(item.status, REQUEST_STATUS.LOADING);
    assert.deepEqual(item.data, []);
    assert.equal(item.refreshing, false);
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
