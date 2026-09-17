import assert from 'node:assert/strict';
import test from 'node:test';

import { buildDeviceSplit, collectHolders } from './device-split.mjs';

// An A100 as HAMi's plugin registers it: 8 slices, 1g only in 0-6, 7g spans all 8.
const a100 = {
  uuid: 'GPU-A',
  mode: 'mig',
  memoryTotal: 40960,
  migProfiles: [
    { name: '1g.5gb', placements: [0, 1, 2, 3, 4, 5, 6].map((start) => ({ start, size: 1 })) },
    { name: '2g.10gb', placements: [0, 2, 4].map((start) => ({ start, size: 2 })) },
    { name: '3g.20gb', placements: [{ start: 0, size: 4 }, { start: 4, size: 4 }] },
    { name: '7g.40gb', placements: [{ start: 0, size: 8 }] },
  ],
};

const container = (podUid, name, devices) => ({ podUid, name, appName: `app-${podUid}`, namespace: 'ml', devices });
const mig = (id, template, migStart, migSize, extra = {}) => ({
  id, allocationShape: 'mig', template, migStart, migSize, allocatedMem: 5120 * migSize, allocatedCores: 14 * migSize, ...extra,
});

test('MIG instances sit in their registered slices and the rest says what still fits', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [
      container('p1', 'train', [mig('GPU-A', '3g.20gb', 0, 4)]),
      container('p2', 'serve', [mig('GPU-A', '1g.5gb', 4, 1)]),
    ],
    highlight: { podUid: 'p2', container: 'serve' },
  });
  assert.equal(split.kind, 'mig');
  assert.equal(split.slots, 8);
  assert.equal(split.lanes, 1);
  assert.deepEqual(split.blocks.map(({ placement, current }) => [placement.start, placement.size, current]), [[0, 4, false], [4, 1, true]]);
  // Slices 5 and 6 still take a 1g; slice 7 takes nothing on its own.
  assert.deepEqual(split.cells.map(({ state }) => state), ['used', 'used', 'used', 'used', 'used', 'open', 'open', 'stranded']);
  assert.deepEqual(split.fits, [{ name: '1g.5gb', count: 2 }]);
});

test('a full A100 has stranded slices and nothing left to place', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [0, 1, 2, 3, 4, 5, 6].map((start) => container(`p${start}`, 'c', [mig('GPU-A', '1g.5gb', start, 1)])),
  });
  assert.equal(split.cells[7].state, 'stranded');
  assert.deepEqual(split.fits, []);
});

test('two instances of one card in one request are two parts, both current', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [container('p1', 'train', [mig('GPU-A', '1g.5gb', 0, 1), mig('GPU-A', '1g.5gb', 1, 1)])],
    highlight: { podUid: 'p1', container: 'train' },
  });
  assert.equal(split.blocks.length, 2);
  assert.ok(split.blocks.every((block) => block.current));
  assert.equal(new Set(split.holders.map((holder) => holder.key)).size, 2);
});

test('overlapping or unregistered placements are drawn where they are, not repacked', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [
      container('p1', 'a', [mig('GPU-A', '3g.20gb', 0, 4)]),
      container('p2', 'b', [mig('GPU-A', '1g.5gb', 2, 1)]),
      container('p3', 'c', [mig('GPU-A', '1g.5gb', 9, 1)]),
    ],
  });
  assert.equal(split.overlapping, true);
  assert.equal(split.lanes, 2);
  assert.deepEqual(split.blocks.map(({ placement, lane }) => [placement.start, lane]), [[0, 0], [2, 1], [9, 0]]);
  assert.equal(split.beyondRegistered, true);
  assert.equal(split.slots, 10);
});

test('a MIG card without registered profiles cannot tell open slices from stranded ones', () => {
  const split = buildDeviceSplit({
    device: { uuid: 'GPU-A', mode: 'mig' },
    containers: [container('p1', 'a', [mig('GPU-A', '1g.5gb', 1, 1)])],
  });
  assert.equal(split.slots, 2);
  assert.equal(split.fits, undefined);
  assert.equal(split.cells[0].state, 'unknown');
});

test('a corrupt placement is laid out by memory, never as billions of slices', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [container('p1', 'a', [mig('GPU-A', '1g.5gb', 2 ** 31 - 2, 1)])],
  });
  assert.equal(split.kind, 'memory');
  assert.equal(split.holders[0].placement, undefined);
});

test('a MIG allocation without a placement is laid out by memory', () => {
  const split = buildDeviceSplit({
    device: a100,
    containers: [container('p1', 'a', [{ id: 'GPU-A', allocationShape: 'mig', allocatedMem: 5120 }])],
  });
  assert.equal(split.kind, 'memory');
  assert.equal(split.blocks[0].share, 5120 / 40960);
});

test('HAMi-core devices report memory and compute quotas, and zero compute is unlimited', () => {
  const split = buildDeviceSplit({
    device: { uuid: 'GPU-C', mode: 'hami-core', memoryTotal: 24000, coreTotal: 100, vgpuTotal: 10 },
    containers: [
      container('p1', 'a', [{ id: 'GPU-C', allocationShape: 'soft', allocatedMem: 8000, allocatedCores: 100 }]),
      container('p2', 'b', [{ id: 'GPU-C', allocationShape: 'soft', allocatedMem: 2000, allocatedCores: 0 }]),
    ],
    highlight: { podUid: 'p2', container: 'b' },
  });
  assert.equal(split.kind, 'shared');
  assert.deepEqual([split.memory.used, split.memory.total, split.memory.over], [10000, 24000, 0]);
  assert.deepEqual(split.compute.parts.map(({ value, share }) => [value, share]), [[100, 1]]);
  assert.equal(split.unlimited, 1);
  assert.deepEqual([split.holderCount, split.limit], [2, 10]);
});

test('overcommitted or unknown memory is reported, not hidden in the scale', () => {
  const over = buildDeviceSplit({
    device: { uuid: 'GPU-C', mode: 'hami-core', memoryTotal: 20000 },
    containers: [container('p1', 'a', [{ id: 'GPU-C', allocationShape: 'soft', allocatedMem: 16000 }]),
      container('p2', 'b', [{ id: 'GPU-C', allocationShape: 'soft', allocatedMem: 16000 }])],
  });
  assert.equal(over.memory.over, 12000);
  assert.equal(over.memory.parts[0].share, 0.5);
  const unknown = buildDeviceSplit({
    device: { uuid: 'B3-0', mode: 'template' },
    containers: [container('p1', 'a', [{ id: 'B3-0', allocationShape: 'template', template: 'vir05', allocatedMem: 16384 }])],
  });
  assert.deepEqual([unknown.kind, unknown.total, unknown.free, unknown.over], ['memory', 0, 0, 0]);
});

test('template splits are parts of the device memory with the unallocated rest', () => {
  const split = buildDeviceSplit({
    device: { uuid: 'B3-0', mode: 'template', memoryTotal: 65536 },
    containers: [
      container('p1', 'a', [{ id: 'B3-0', allocationShape: 'template', template: 'vir05_1c_16g', allocatedMem: 16384 }]),
      container('p2', 'b', [{ id: 'B3-0', allocationShape: 'unknown', allocatedMem: 32768, allocatedCoresKnown: false }]),
    ],
  });
  assert.equal(split.kind, 'memory');
  assert.deepEqual([split.used, split.free], [49152, 16384]);
  assert.deepEqual(split.holders.map(({ shape, name, coresKnown }) => [shape, name, coresKnown]), [
    ['template', 'vir05_1c_16g', true], ['unknown', '', false],
  ]);
});

test('an idle device keeps its mode and a workload without identity is never current', () => {
  assert.equal(buildDeviceSplit({ device: a100 }).kind, 'mig');
  assert.equal(buildDeviceSplit({ device: { uuid: 'G', mode: 'hami-core' } }).kind, 'shared');
  assert.equal(buildDeviceSplit({ device: { uuid: 'B', mode: 'template' } }).kind, 'memory');
  const [holder] = collectHolders({ uuid: 'G' }, [{ devices: [{ id: 'G' }] }], {});
  assert.equal(holder.current, false);
  assert.deepEqual(collectHolders({ uuid: 'G' }, [{ devices: [{ id: 'other' }, null] }]), []);
});
