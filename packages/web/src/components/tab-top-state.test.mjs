import assert from 'node:assert/strict';
import test from 'node:test';

import { formatRankingValue } from './tab-top-state.mjs';

test('ranking values preserve meaningful fractional data', () => {
  assert.equal(formatRankingValue(0.4, '%'), '0.4 %');
  assert.equal(formatRankingValue(1.25, 'GiB'), '1.25 GiB');
  assert.equal(formatRankingValue(4, ' '), '4');
  assert.equal(formatRankingValue(0, '%'), '0 %');
  assert.equal(formatRankingValue(0.004, '%'), '<0.01 %');
});
