import assert from 'node:assert/strict';
import test from 'node:test';

import { readWorkloadTotal } from './overview-state.mjs';

test('the overview workload count is the list total, never the page length', () => {
  assert.equal(readWorkloadTotal({ items: [{ name: 'worker' }], total: 68 }), 68);
  assert.equal(readWorkloadTotal({ items: [], total: 0 }), 0);
});

test('a malformed workload reply has no count rather than zero', () => {
  for (const reply of [
    undefined,
    null,
    {},
    { total: 5 },
    { items: [{ name: 'worker' }] },
    { items: [], total: -1 },
    { items: [], total: 1.5 },
    { items: [], total: '68' },
  ]) {
    assert.equal(readWorkloadTotal(reply), undefined, JSON.stringify(reply));
  }
});
