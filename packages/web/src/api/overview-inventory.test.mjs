import assert from 'node:assert/strict';
import test from 'node:test';
import { readCoverageCount, readOverviewInventory } from './overview-inventory.mjs';
import { readModelDistribution } from './model-distribution.mjs';

const coverage = (entity, observed, complete = true) => ({
  entity, observed, complete, ...(complete ? { total: observed } : {}),
});
const device = { node: 'n1', provider: 'NVIDIA', deviceId: 'gpu', model: 'A100', healthy: false, utilizationRatio: 0 };
const node = { name: 'n1', uid: 'current-node', schedulable: false, physicalDevices: 1 };
const inventory = (devices = [device], nodes = [node]) => ({
  devices, nodes, deviceCoverage: coverage('device', devices.length), nodeCoverage: coverage('node', nodes.length),
});

test('zero devices, unavailable inventory, and partial membership remain distinct', () => {
  for (const complete of [true, false]) {
    const input = { ...inventory([]), deviceCoverage: coverage('device', 0, complete) };
    const result = readOverviewInventory(input);
    assert.equal(readCoverageCount(result.deviceCoverage).count, complete ? 0 : undefined);
    assert.equal(result.nodes[0].schedulable, false);
    assert.deepEqual(result.devices, []);
  }
  const partial = { ...inventory(), nodeCoverage: coverage('node', 1, false) };
  const observed = readOverviewInventory(partial).devices[0];
  assert.equal(observed.utilizationRatio, 0);
  assert.equal(observed.healthy, false);
});

test('reject inconsistent coverage, repeated identities, and malformed readings', () => {
  const input = inventory();
  for (const changed of [
    { ...input, deviceCoverage: coverage('node', 1) }, { ...input, deviceCoverage: coverage('device', 0) },
    inventory([device, device]), inventory([], [node, node]), inventory([{ ...device, provider: '' }]),
    inventory([{ ...device, healthy: 'false' }]), inventory([{ ...device, memoryUsageRatio: -1 }]),
  ]) assert.throws(() => readOverviewInventory(changed));
});

test('a device ID can recur on a different node or under a different provider', () => {
  const devices = [device, { ...device, node: 'n2' }, { ...device, provider: 'DCU', model: '', healthy: undefined }];
  const nodes = [{ ...node, physicalDevices: 2 }, { ...node, name: 'n2', uid: 'other-node' }];
  const result = readOverviewInventory(inventory(devices, nodes));
  assert.deepEqual(result.devices, devices);
});

test('the model chart and headline count account for each physical card even when its model is unknown', () => {
  const reply = { coverage: coverage('device', 3, false),
    items: [{ model: 'A100', physicalDevices: 2 }, { model: '', physicalDevices: 1 }] };
  const { inventory, rows } = readModelDistribution(reply);
  assert.equal(inventory.count, 3);
  assert.deepEqual(rows.map(row => row.type), ['A100', null]);
  assert.throws(() => readModelDistribution({ ...reply, items: [reply.items[0]] }));
  assert.throws(() => readModelDistribution({ ...reply, items: [reply.items[1], reply.items[1], reply.items[1]] }));
});
