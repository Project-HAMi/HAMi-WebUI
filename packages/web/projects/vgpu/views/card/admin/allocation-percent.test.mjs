import assert from 'node:assert/strict';
import test from 'node:test';

import { getAllocationPercent } from './allocation-percent.mjs';

test('allocation text preserves overcommit while progress remains bounded', () => {
  assert.deepEqual(getAllocationPercent(150, 100), {
    raw: 150,
    progress: 100,
  });
});

test('allocation percentage rejects an unavailable capacity', () => {
  assert.equal(getAllocationPercent(10, 0), undefined);
  assert.equal(getAllocationPercent(10, undefined), undefined);
});
