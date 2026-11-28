import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isLowerBound,
  isNothingCounted,
  lowerBoundMessage,
} from './uncounted.mjs';

test('a bound of zero over unknown allocations is not shown as a number', () => {
  assert.equal(isNothingCounted(0, 1), true);
  assert.equal(isNothingCounted('0', 1), true);
  assert.equal(isNothingCounted(0, null), false);
  assert.equal(isNothingCounted(5, 1), false);
  assert.equal(isNothingCounted(0, 0), false);
});

test('an unread count keeps the rate a lower bound without naming a number', () => {
  assert.equal(isLowerBound(0), false);
  assert.equal(isLowerBound(undefined), false);
  assert.equal(isLowerBound(3), true);
  assert.equal(isLowerBound(null), true);
  const t = (key, params) => `${key}${params ? `:${params.count}` : ''}`;
  assert.equal(lowerBoundMessage(t, 3), 'dashboard.metricLowerBound:3');
  assert.equal(lowerBoundMessage(t, null), 'dashboard.metricLowerBoundUnknown');
});
