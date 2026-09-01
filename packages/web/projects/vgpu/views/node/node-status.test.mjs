import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  getNodeSchedulingEligibilityStatus,
  getNodeReadinessStatus,
  getNodeSchedulingStatus,
  isNodeSchedulingEligible,
  matchesNodeSchedulingEligibility,
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

test('node scheduling eligibility presents one result and only explains abnormal states', () => {
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus({ isReady: true, isSchedulable: true }),
    {
      icon: 'status-schedulable',
      labelKey: 'node.schedulable',
      showHelp: false,
    },
  );
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus({ isReady: true, isSchedulable: false }),
    {
      icon: 'status-unschedulable',
      labelKey: 'node.temporarilyUnschedulable',
      descriptionKey: 'node.cordonedDescription',
      showHelp: true,
    },
  );
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus({ isReady: false, isSchedulable: true }),
    {
      icon: 'status-unschedulable',
      labelKey: 'node.temporarilyUnschedulable',
      descriptionKey: 'node.notInReadyStateDescription',
      showHelp: true,
    },
  );
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus({ isReady: false, isSchedulable: false }),
    {
      icon: 'status-unschedulable',
      labelKey: 'node.temporarilyUnschedulable',
      descriptionKey: 'node.notInReadyStateAndCordonedDescription',
      showHelp: true,
    },
  );
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus({ isReady: true, isSchedulable: true, isExternal: true }),
    {
      icon: 'status-unschedulable',
      labelKey: 'node.temporarilyUnschedulable',
      descriptionKey: 'node.schedulingUnknownDescription',
      showHelp: true,
    },
  );
  assert.deepEqual(
    getNodeSchedulingEligibilityStatus(),
    {
      icon: 'status-unschedulable',
      labelKey: 'node.temporarilyUnschedulable',
      descriptionKey: 'node.schedulingUnknownDescription',
      showHelp: true,
    },
  );
});

test('eligibility filtering uses the same combined Ready and cordon boundary', () => {
  const ready = { isReady: true, isSchedulable: true };
  const notReady = { isReady: false, isSchedulable: true };
  const cordoned = { isReady: true, isSchedulable: false };
  const external = { isReady: true, isSchedulable: true, isExternal: true };

  assert.equal(isNodeSchedulingEligible(ready), true);
  assert.equal(isNodeSchedulingEligible(notReady), false);
  assert.equal(matchesNodeSchedulingEligibility(ready, 'schedulable'), true);
  assert.equal(matchesNodeSchedulingEligibility(notReady, 'schedulable'), false);
  assert.equal(matchesNodeSchedulingEligibility(cordoned, 'temporarilyUnschedulable'), true);
  assert.equal(matchesNodeSchedulingEligibility(external, 'temporarilyUnschedulable'), true);
});

test('node list and detail share one combined scheduling eligibility status', () => {
  const list = readFileSync(new URL('./admin/index.vue', import.meta.url), 'utf8');
  const detail = readFileSync(new URL('./admin/Detail.vue', import.meta.url), 'utf8');

  assert.match(list, /getNodeSchedulingEligibilityStatus/);
  assert.match(detail, /getNodeSchedulingEligibilityStatus/);
  assert.doesNotMatch(list, /getNodeReadinessStatus/);
  assert.doesNotMatch(list, /getNodeSchedulingStatus/);
  assert.doesNotMatch(detail, /getNodeReadinessStatus/);
  assert.doesNotMatch(detail, /getNodeSchedulingStatus/);
  assert.doesNotMatch(detail, /label: t\('node\.schedulingStatus'\)/);
  assert.match(list, /filters\.schedulingEligibility/);
  assert.match(list, /route\.query\.schedulingEligibility/);
});
