import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createWorkloadRowKey,
  formatWorkloadName,
} from './workload-identity.mjs';

test('containers in the same Pod have distinct workload row keys', () => {
  const worker = createWorkloadRowKey({ podUid: 'pod-uid', name: 'worker' });
  const sidecar = createWorkloadRowKey({ podUid: 'pod-uid', name: 'sidecar' });

  assert.equal(worker, 'pod-uid/worker');
  assert.equal(sidecar, 'pod-uid/sidecar');
  assert.notEqual(worker, sidecar);
});

test('workload name exposes both Pod and container identity', () => {
  assert.equal(
    formatWorkloadName({ appName: 'training-pod', name: 'worker' }),
    'training-pod / worker',
  );
  assert.equal(
    formatWorkloadName({ appName: 'worker', name: 'worker' }),
    'worker',
  );
  assert.equal(formatWorkloadName(), '--');
});
