import assert from 'node:assert/strict';
import test from 'node:test';
import { summarizeNodeIssues, visibleDeviceGroups } from './node-device-issues.mjs';

test('counts unready nodes and unhealthy devices once, without counting paused scheduling as an incident', () => {
  const issues = [
    { kind: 'nodeCordoned', node: 'mixed' },
    { kind: 'nodeNotReady', node: 'mixed', nodeUid: 'uid-mixed', deviceCount: 8 },
    { kind: 'nodeNotReady', node: 'mixed' },
    { kind: 'deviceUnhealthy', node: 'mixed', deviceId: 'gpu-1' },
    { kind: 'deviceUnhealthy', node: 'mixed', deviceId: 'gpu-1' },
    { kind: 'deviceUnhealthy', node: 'second', deviceId: 'gpu-1' },
    { kind: 'nodeCordoned', node: 'maintenance', deviceCount: 6 },
  ];
  const { incidents } = summarizeNodeIssues(issues);
  assert.deepEqual(incidents.map(row => [row.node, row.affectedDevices]), [['mixed', 1], ['second', 1]]);
  assert.equal(incidents[0].to.params.uid, 'uid-mixed');
  assert.equal(incidents[0].devices.length, 1);
});

test('aggregate counts survive without generating a device ID or inferring node health', () => {
  const { incidents } = summarizeNodeIssues([
    { kind: 'deviceUnhealthy', node: 'node-1', model: 'A', deviceCount: 12 },
    { kind: 'nodeCordoned', node: 'node-2', deviceCount: 48 },
    { kind: 'unknownHealth', node: 'other' },
  ]);
  assert.equal(incidents.length, 1);
  assert.equal(incidents[0].affectedDevices, 12);
  assert.equal(incidents[0].devices[0].deviceId, undefined);
  assert.equal(incidents[0].notReady, null);
  assert.equal(incidents[0].to, null);
});

test('each visible device stays under its reported model as more issues are opened', () => {
  const devices = [
    { model: 'H100', deviceId: 'gpu-1' },
    { model: 'L40S', deviceId: 'gpu-2' },
    { model: 'H100', deviceId: 'gpu-3' },
    { deviceId: 'gpu-4' },
    { model: 'L40S', deviceCount: 2 },
  ];
  assert.deepEqual(visibleDeviceGroups(devices, 3), [
    { model: 'H100', devices: [devices[0], devices[2]], reportedWithoutId: 0 },
    { model: 'L40S', devices: [devices[1]], reportedWithoutId: 0 },
  ]);
  assert.deepEqual(visibleDeviceGroups(devices, 5), [
    { model: 'H100', devices: [devices[0], devices[2]], reportedWithoutId: 0 },
    { model: 'L40S', devices: [devices[1]], reportedWithoutId: 2 },
    { model: '', devices: [devices[3]], reportedWithoutId: 0 },
  ]);
});

test('device IDs are local to the provider and node, including in the expanded list', () => {
  const records = ['X', 'Y'].map(provider => ({ kind: 'deviceUnhealthy', node: 'n1', model: 'A',
    provider, deviceId: '0', detailTo: provider === 'X' ? null : { name: 'detail' } }));
  const { incidents } = summarizeNodeIssues([...records, records[0]]);
  assert.equal(incidents[0].affectedDevices, 2);
  assert.deepEqual(visibleDeviceGroups(incidents[0].devices, 3)[0].devices, records);
});
