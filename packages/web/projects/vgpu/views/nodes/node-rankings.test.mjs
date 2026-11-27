import assert from 'node:assert/strict';
import test from 'node:test';
import { buildNodeRankings, buildNodeVendorDistribution } from './node-rankings.mjs';

test('buildNodeVendorDistribution counts unique providers per node', () => {
  const nodes = [
    { name: 'n1', uid: 'u1', providers: ['NVIDIA', 'NVIDIA'] },
    { name: 'n2', uid: 'u2', providers: ['Hygon', 'NVIDIA'] },
    { name: 'n3', uid: 'u3', providers: [] },
  ];
  assert.deepEqual(buildNodeVendorDistribution(nodes), [
    { name: 'NVIDIA', vendor: 'NVIDIA', value: 2 },
    { name: 'Hygon', vendor: 'Hygon', value: 1 },
  ]);
});

test('buildNodeRankings sorts descending, omits missing values, and breaks ties by name then uid', () => {
  const nodes = [
    { name: 'node-b', uid: 'uid-2', coreAllocationPercent: 50, computeUtilizationPercent: 40 },
    { name: 'node-a', uid: 'uid-2', coreAllocationPercent: 50, computeUtilizationPercent: 80, memoryAllocationPercent: 29.6875, memoryUsagePercent: 12.5 },
    { name: 'node-a', uid: 'uid-1', coreAllocationPercent: 50, computeUtilizationPercent: null, memoryUsagePercent: -1 },
    { name: 'node-c', uid: 'uid-3', coreAllocationPercent: 90 },
    { name: 'node-d', uid: 'uid-4', coreAllocationPercent: 10 },
    { name: 'node-e', uid: 'uid-5', coreAllocationPercent: 20 },
    { name: 'node-f', uid: 'uid-6', coreAllocationPercent: 30 },
  ];
  const rankings = buildNodeRankings(nodes);
  assert.deepEqual(rankings.computeAllocation.map((item) => `${item.name}:${item.uid}:${item.value}`), [
    'node-c:uid-3:90',
    'node-a:uid-1:50',
    'node-a:uid-2:50',
    'node-b:uid-2:50',
    'node-f:uid-6:30',
  ]);
  assert.deepEqual(rankings.computeUsage.map((item) => item.name), ['node-a', 'node-b']);
  assert.deepEqual(rankings.memoryAllocation.map((item) => item.value), [29.6875]);
  assert.deepEqual(rankings.memoryUsage.map((item) => item.value), [12.5]);
});
