import assert from 'node:assert/strict';
import test from 'node:test';

import en from '../../../../../src/locales/en.js';
import zh from '../../../../../src/locales/zh.js';
import {
  CORES_UNKNOWN_REASONS,
  getAllocationShapeCopy,
  getCoresUnknownReasonKey,
  isUnreservedSoftSplit,
} from './allocation-display.mjs';

const lookup = (messages, key) => key.split('.').reduce((value, part) => value?.[part], messages);

test('allocation shapes name the applied template', () => {
  assert.deepEqual(getAllocationShapeCopy({ allocationShape: 'template', template: 'vir05_1c_16g' }), {
    key: 'task.allocation.shape.templateNamed',
    params: { template: 'vir05_1c_16g' },
  });
  assert.equal(getAllocationShapeCopy({ allocationShape: 'template' }).key, 'task.allocation.shape.template');
  assert.equal(getAllocationShapeCopy({ allocationShape: 'whole' }).key, 'task.allocation.shape.whole');
  assert.equal(getAllocationShapeCopy({ allocationShape: 'soft' }).key, 'task.allocation.shape.soft');
  assert.equal(getAllocationShapeCopy({ allocationShape: 'unknown' }).key, 'task.allocation.shape.unknown');
  for (const row of [{}, { allocationShape: '' }, { allocationShape: 'mig' }, undefined]) {
    assert.equal(getAllocationShapeCopy(row), undefined);
  }
});

test('unknown compute shares explain why only when the server gives a reason', () => {
  assert.equal(
    getCoresUnknownReasonKey({ allocatedCoresKnown: false, allocatedCoresReason: 'mode_ambiguous' }),
    'task.allocation.reason.mode_ambiguous',
  );
  assert.equal(getCoresUnknownReasonKey({ allocatedCoresKnown: false }), '');
  assert.equal(getCoresUnknownReasonKey({ allocatedCoresKnown: false, allocatedCoresReason: 'future_reason' }), '');
  assert.equal(getCoresUnknownReasonKey({ allocatedCoresKnown: true, allocatedCoresReason: 'mode_ambiguous' }), '');
  assert.equal(getCoresUnknownReasonKey({ allocatedCoresReason: 'mode_ambiguous' }), '');
});

test('only a soft split without a share reads as unreserved', () => {
  assert.equal(isUnreservedSoftSplit({ allocationShape: 'soft', allocatedCores: 0, allocatedCoresKnown: true }), true);
  assert.equal(isUnreservedSoftSplit({ allocationShape: 'soft', allocatedCores: 25, allocatedCoresKnown: true }), false);
  assert.equal(isUnreservedSoftSplit({ allocationShape: 'template', allocatedCores: 0, allocatedCoresKnown: true }), false);
  assert.equal(isUnreservedSoftSplit({ allocationShape: 'soft', allocatedCores: 0, allocatedCoresKnown: false }), false);
  assert.equal(isUnreservedSoftSplit({}), false);
});

test('every shape and reason has copy in both languages', () => {
  const keys = [
    ...['whole', 'template', 'templateNamed', 'soft', 'unknown'].map((shape) => `task.allocation.shape.${shape}`),
    ...CORES_UNKNOWN_REASONS.map((reason) => `task.allocation.reason.${reason}`),
    'task.allocation.label',
    'task.allocation.reasonLabel',
  ];
  for (const key of keys) {
    assert.equal(typeof lookup(zh, key), 'string', `zh ${key}`);
    assert.equal(typeof lookup(en, key), 'string', `en ${key}`);
  }
  assert.match(lookup(zh, 'task.allocation.shape.templateNamed'), /\{template\}/);
  assert.match(lookup(en, 'task.allocation.shape.templateNamed'), /\{template\}/);
});
