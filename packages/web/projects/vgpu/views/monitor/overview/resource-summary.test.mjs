import assert from 'node:assert/strict';
import test from 'node:test';
import { readContainerSummary, readResourceInventory } from './resource-summary.mjs';

const coverage = (entity, observed, complete) => ({ entity, observed, complete, ...(complete ? { total: observed } : {}) });
const inventory = (nodes, devices, nodeComplete = true, deviceComplete = true) => ({
  nodes, devices, nodeCoverage: coverage('node', nodes.length, nodeComplete), deviceCoverage: coverage('device', devices.length, deviceComplete),
});
const node = { name: 'node-1', uid: 'current', ready: true, schedulable: true };
const device = { node: node.name, provider: 'NVIDIA', deviceId: 'gpu', model: 'A100', healthy: true };

test('only ready uncordoned nodes qualify; links can use the UID from the same response', () => {
  const listed = [node, { name: 'node-2', uid: 'n2', ready: false, schedulable: true },
    { name: 'node-3', uid: 'n3', ready: true, schedulable: false }, { name: 'node-4', uid: 'n4' }];
  const { nodes, summary } = readResourceInventory(inventory(listed, [], false));
  assert.deepEqual(summary.nodes, { count: 4, observed: 4, partial: true, schedulable: 1, unavailable: 3 });
  assert.equal(nodes.find(n => n.name === node.name).uid, 'current');
  const replacement = readResourceInventory(inventory([{ ...node, uid: 'replacement' }], []));
  assert.equal(replacement.nodes[0].uid, 'replacement');
});

test('health totals and model shares use every observed physical identity, including unknowns', () => {
  const named = 'A100 (80GB) ';
  const other = { ...node, name: 'node-2', uid: 'n2' };
  const devices = [{ ...device, model: named }, { ...device, model: named, node: other.name, healthy: false },
    { ...device, provider: 'DCU', model: '  ', healthy: null }, { ...device, deviceId: 'gpu-1', model: undefined }];
  const input = inventory([node, other], devices, true, false);
  input.deviceCoverage.total = 8;
  const { summary, models } = readResourceInventory(input);
  assert.deepEqual(summary.devices, { count: 4, observed: 4, partial: true, counts: { healthy: 2, unhealthy: 1, unknown: 1 } });
  assert.deepEqual(models.rows, [{ type: named, vendor: 'NVIDIA', count: 2, share: 0.5 }, { type: null, count: 2, share: 0.5 }]);
  assert.equal(models.inventory.count, summary.devices.count);
});

test('an empty observation is zero only for the resource whose coverage is complete', () => {
  const { summary, models } = readResourceInventory(inventory([], [], true, false));
  assert.deepEqual(summary.nodes, { count: 0, observed: 0, partial: false, schedulable: 0, unavailable: 0 });
  assert.deepEqual(summary.devices, { count: undefined, observed: 0, partial: true, counts: {} });
  assert.deepEqual(models.rows, []);
  const reversed = readResourceInventory(inventory([], [], false, true));
  assert.deepEqual(reversed.summary.nodes, { count: undefined, observed: 0, partial: true, schedulable: undefined, unavailable: undefined });
  assert.deepEqual(reversed.summary.devices, { count: 0, observed: 0, partial: false, counts: { healthy: 0, unhealthy: 0, unknown: 0 } });
});

test('missing or inconsistent inventory coverage does not produce counts', () => {
  const input = inventory([node], [device]);
  for (const invalid of [{ ...input, deviceCoverage: undefined }, { ...input, nodeCoverage: { ...input.nodeCoverage, observed: 2 } },
    inventory([node], [device, device]), inventory([node], [{ ...device, healthy: 'false' }])]) {
    assert.throws(() => readResourceInventory(invalid), TypeError);
  }
});

test('container counts partition the list total before pagination and preserve unclassified statuses', () => {
  const reply = { items: [{ status: 'success' }], total: 12, statusCounts: { all: 12, success: 5, abnormal: 2, pending: 2, waiting: 1 } };
  assert.deepEqual(readContainerSummary(reply), { total: 12, status: 'ready', counts: { success: 5, abnormal: 2, other: 5 } });
  assert.deepEqual(readContainerSummary({ items: [], total: 0, statusCounts: {} }), {
    total: 0, status: 'ready', counts: { success: 0, abnormal: 0, other: 0 },
  });
});

test('missing or contradictory status counts preserve the total without presenting zeroes', () => {
  const input = { items: [{ status: 'success' }], total: 3 };
  assert.deepEqual(readContainerSummary(input), { total: 3, status: 'missing', counts: null });
  for (const statusCounts of [null, [], {}, { all: 2, success: 2 }, { all: 3, success: '2' }, { all: 3, abnormal: -1 },
    { all: 3, abnormal: 2, pending: 1, waiting: 1 }, { all: 3, abnormal: 2, success: 2 }, { all: 3, future: 4 }]) {
    assert.deepEqual(readContainerSummary({ ...input, statusCounts }), { total: 3, status: 'invalid', counts: null });
  }
  for (const malformed of [{ total: 0 }, { ...input, total: 0 }, { ...input, total: Number.MAX_SAFE_INTEGER + 1 }]) {
    assert.throws(() => readContainerSummary(malformed), TypeError);
  }
});

test('zero, missing readings and positive incidents survive incomplete coverage', () => {
  const reply = inventory([{ ...node, ready: false, schedulable: false }], [
    { ...device, healthy: false, utilizationRatio: 0, memoryUsageRatio: 0.75 },
    { ...device, deviceId: 'other', healthy: null, utilizationRatio: null, memoryUsageRatio: 0 },
  ], true, false);
  const result = readResourceInventory(reply);
  assert.equal(result.issueStatus, 'ready');
  assert.equal(result.usageStatus, 'ready');
  assert.deepEqual(result.issues.map(issue => issue.kind), ['nodeNotReady', 'deviceUnhealthy']);
  assert.deepEqual(result.devices.map(d => [d.nodeUid, d.compute, d.memory]), [[node.uid, 0, 75], [node.uid, null, 0]]);
  assert.equal(readResourceInventory(inventory([], [], true, false)).usageStatus, 'error');
});

test('only reported issues affect the entry; missing health and readings are not faults', () => {
  const healthy = readResourceInventory(inventory([node], [device]));
  assert.equal(healthy.issueStatus, 'ready');
  assert.deepEqual(healthy.issues, []);
  assert.deepEqual(healthy.devices.map(({ compute, memory }) => [compute, memory]), [[null, null]]);

  const unknown = readResourceInventory(inventory([node], [{ ...device, healthy: null, utilizationRatio: 0 }]));
  assert.equal(unknown.issueStatus, 'ready');
  assert.equal(unknown.devices[0].compute, 0);
  assert.equal(readResourceInventory(inventory([], [])).issueStatus, 'empty');
  assert.equal(readResourceInventory(inventory([], [], true, false)).issueStatus, 'empty');
  assert.equal(readResourceInventory(inventory([node], [])).issueStatus, 'ready');
});

test('model marks follow the reported vendor, even for shared or ambiguous model names', () => {
  const devices = ['HCU', 'DCU', 'Metax', 'MLU'].map((provider, i) => ({
    ...device, provider, deviceId: String(i), model: i < 2 ? 'K100' : 'Shared',
  }));
  const { models } = readResourceInventory(inventory([node], devices));
  assert.deepEqual(models.rows.map(({ type, vendor, count }) => [type, vendor, count]), [
    ['K100', 'Hygon', 2], ['Shared', undefined, 2],
  ]);
});


test('inventory gaps do not create incidents or change resource coverage', () => {
  const reply = inventory([node], [device], true, false);
  reply.deviceCoverage.sources = [
    { reason: 'unmapped_resource', params: { node: node.name, detail: 'nvidia.com/gpu' } },
    { reason: 'unread_registration', params: { node: node.name, detail: 'hami.io/node-nvidia-register' } },
  ];
  const result = readResourceInventory(reply);
  assert.equal(result.issueStatus, 'ready');
  assert.deepEqual(result.issues, []);
  assert.equal(result.summary.devices.partial, true);
  assert.equal(result.summary.devices.observed, 1);
});
