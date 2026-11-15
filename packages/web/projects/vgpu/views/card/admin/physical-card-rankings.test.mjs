import test from 'node:test';
import assert from 'node:assert/strict';
import { physicalCardRankings } from './physical-card-rankings.mjs';

test('rank all eligible physical cards before taking five, keeping known zero and overcommit', () => {
  const cards = Array.from({ length: 9 }, (_, i) => ({
    uuid: String(i), physicalKey: String(i), detailTo: null,
    coreUsed: i * 25, coreTotal: 100, memoryUsed: 8 - i, memoryTotal: 8,
  }));
  cards[8].unconfigured = true;
  cards[7].isExternal = true;
  cards[6].coreUsedKnown = false;
  cards[8].utilizationRatio = 0;
  cards[7].memoryUsageRatio = 0.25;
  const ranks = physicalCardRankings(cards);
  assert.deepEqual(ranks.computeAllocation.map(r => r.value), [125, 100, 75, 50, 25]);
  assert.deepEqual(ranks.memoryAllocation.map(r => r.name), ['0', '1', '2', '3', '4']);
  assert.deepEqual(ranks.computeUsage.map(r => r.value), [0]);
  assert.deepEqual(ranks.memoryUsage.map(r => r.value), [25]);
  assert.deepEqual(physicalCardRankings([cards[6]]).computeAllocation, []);
  assert.equal(physicalCardRankings([cards[6]]).memoryAllocation[0].value, 25);
});

test('equal readings keep a stable order across nodes, preserving each detail target', () => {
  const cards = ['z', 'a'].map(node => ({
    uuid: 'same', physicalKey: node, detailTo: null, utilizationRatio: 0.5,
  }));
  const { computeUsage } = physicalCardRankings(cards);
  assert.deepEqual(computeUsage.map(r => r.physicalKey), ['a', 'z']);
  assert.ok(computeUsage.every(r => r.detailTo === null));
});
