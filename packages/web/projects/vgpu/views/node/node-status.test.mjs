import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  getNodeReadinessStatus,
  getNodeSchedulingStatus,
} from './node-status.mjs';

test('readiness and scheduling remain independent node states', () => {
  assert.deepEqual(
    getNodeReadinessStatus({ isReady: true, isSchedulable: false }),
    { icon: 'status-schedulable', labelKey: 'node.ready' },
  );
  assert.deepEqual(
    getNodeSchedulingStatus({ isReady: true, isSchedulable: false }),
    { icon: 'status-unschedulable', labelKey: 'node.schedulingDisabled' },
  );

  assert.deepEqual(
    getNodeReadinessStatus({ isReady: false, isSchedulable: true }),
    { icon: 'status-unschedulable', labelKey: 'node.notReady' },
  );
  assert.deepEqual(
    getNodeSchedulingStatus({ isReady: false, isSchedulable: true }),
    { icon: 'status-schedulable', labelKey: 'node.schedulingEnabled' },
  );
});

test('unmanaged and unavailable node states remain unknown', () => {
  assert.equal(
    getNodeReadinessStatus({ isReady: true, isExternal: true }).labelKey,
    'node.unknown',
  );
  assert.equal(getNodeReadinessStatus().labelKey, 'node.unknown');
  assert.equal(getNodeSchedulingStatus().labelKey, 'node.unknown');
});

test('node list and detail share the status contract without changing the scheduling filter', () => {
  const list = readFileSync(new URL('./admin/index.vue', import.meta.url), 'utf8');
  const detail = readFileSync(new URL('./admin/Detail.vue', import.meta.url), 'utf8');

  for (const source of [list, detail]) {
    assert.match(source, /getNodeReadinessStatus/);
    assert.match(source, /getNodeSchedulingStatus/);
  }
  assert.match(list, /filters\.isSchedulable/);
  assert.match(list, /route\.query\.isSchedulable/);
});
