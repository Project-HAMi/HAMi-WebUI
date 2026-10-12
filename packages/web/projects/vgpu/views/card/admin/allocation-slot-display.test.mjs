import assert from 'node:assert/strict';
import test from 'node:test';

import { getAllocationSlotDisplay } from './allocation-slot-display.mjs';

const shared = { vendor: 'NVIDIA', mode: 'hami-core', vgpuUsed: 2, vgpuTotal: 10 };

test('registered NVIDIA and Ascend sharing quotas retain their allocated count', () => {
  for (const vendor of ['NVIDIA', 'Ascend']) {
    assert.deepEqual(getAllocationSlotDisplay({ ...shared, vendor }), { used: 2, limit: 10, percent: 20 });
  }
  assert.deepEqual(getAllocationSlotDisplay({ ...shared, vgpuUsed: 0 }), { used: 0, limit: 10, percent: 0 });
  assert.deepEqual(getAllocationSlotDisplay({ ...shared, vgpuUsed: '2', vgpuTotal: '10' }), { used: 2, limit: 10, percent: 20 });
});

test('partition instance counts never become a capacity percentage', () => {
  assert.equal(getAllocationSlotDisplay({ ...shared, mode: 'mig', vgpuTotal: 7 }), undefined);
  assert.equal(getAllocationSlotDisplay({ ...shared, vendor: 'Ascend', mode: 'template', vgpuUsed: 1, vgpuTotal: 8 }), undefined);
});

test('a positive count does not establish support for an unknown mode or provider', () => {
  for (const mode of [undefined, '', 'whole', 'unknown', 'future-mode']) {
    assert.equal(getAllocationSlotDisplay({ ...shared, mode, vgpuTotal: 1 }), undefined);
  }
  for (const vendor of [undefined, '', 'MLU', 'DCU', 'HCU', 'Metax', 'future-vendor']) {
    assert.equal(getAllocationSlotDisplay({ ...shared, vendor }), undefined);
  }
  assert.equal(getAllocationSlotDisplay({ ...shared, unconfigured: true }), undefined);
  assert.equal(getAllocationSlotDisplay(), undefined);
  assert.equal(getAllocationSlotDisplay(null), undefined);
});

test('unavailable counts do not become zero or a fabricated percentage', () => {
  for (const value of [undefined, null, '', ' ', false, 'invalid', -1, 1.5, NaN, Infinity, Number.MAX_SAFE_INTEGER + 1]) {
    assert.deepEqual(getAllocationSlotDisplay({ ...shared, vgpuUsed: value }), { used: undefined, limit: 10, percent: undefined });
    assert.deepEqual(getAllocationSlotDisplay({ ...shared, vgpuTotal: value }), { used: 2, limit: undefined, percent: undefined });
  }
  assert.deepEqual(getAllocationSlotDisplay({ ...shared, vgpuTotal: 0 }), { used: 2, limit: 0, percent: undefined });
});
