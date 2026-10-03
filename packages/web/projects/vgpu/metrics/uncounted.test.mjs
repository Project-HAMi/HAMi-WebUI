import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isLowerBound,
  isNothingCounted,
  lowerBoundMessage,
  readTrendUncounted,
  readUncounted,
  readUncountedMetric,
  readUncountedRange,
} from './uncounted.mjs';

test('only a read count confirms how many allocations a rate leaves out', () => {
  assert.equal(readUncounted('ready', '2'), 2);
  assert.equal(readUncounted('ready', '0'), 0);
  // count() over nothing returns no series.
  assert.equal(readUncounted('missing'), 0);
  assert.equal(readUncounted('loading'), undefined);
  assert.equal(readUncounted('error'), null);
  assert.equal(readUncounted('invalid'), null);
});

test('a range is a lower bound when any sample left allocations out', () => {
  const points = (...values) => values.map((value, i) => ({ timestamp: i, value }));
  assert.equal(readUncountedRange({ status: 'ready', data: points(0, 1, null, 0) }), 1);
  assert.equal(readUncountedRange({ status: 'ready', data: points(0, 0) }), 0);
  assert.equal(readUncountedRange({ status: 'missing', data: [] }), 0);
  assert.equal(readUncountedRange({ status: 'error', data: [] }), null);
});

test('a failed refresh cannot keep the count it failed to replace', () => {
  assert.equal(readUncountedMetric({ status: 'missing', count: 0 }), 0);
  assert.equal(readUncountedMetric({ status: 'missing', count: 0, refreshError: new Error('timeout') }), null);
  assert.equal(readUncountedMetric({ status: 'loading' }), undefined);
});

test('a trend stays a lower bound until its own range confirms zero', () => {
  assert.equal(readTrendUncounted({ status: 'missing', data: [] }), 0);
  assert.equal(readTrendUncounted({ status: 'loading', data: [] }), null);
  assert.equal(readTrendUncounted({ status: 'error', data: [] }), null);
  assert.equal(readTrendUncounted({ status: 'ready', data: [{ value: 2 }] }), 2);
});

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

test('a trend and its count settle together, so a line never rests on another range\'s count', async () => {
  const {
    createRangeGroupState,
    createRangeVectorOutcome,
    publishInitialRangeOutcome,
    settleRangeGroupGeneration,
    startRangeGroupGeneration,
  } = await import('../hooks/range-vector-query-state.mjs');
  const line = (value) => createRangeVectorOutcome({ status: 'ready', data: [{ timestamp: 1, value }] });
  const count = (value) => createRangeVectorOutcome(value === undefined
    ? { status: 'missing', data: [] }
    : { status: 'ready', data: [{ timestamp: 1, value }] });
  const failed = createRangeVectorOutcome({ status: 'error', data: [] }, new Error('injected'));
  const range = (start) => ({ start, end: start + 3600, step: '1m' });
  const settle = (group, outcomes, start) => {
    const started = startRangeGroupGeneration(group, range(start));
    return settleRangeGroupGeneration(started, started.requestId, outcomes);
  };

  // Initial load: the line can arrive first; until the count does, the line stays a bound.
  let group = startRangeGroupGeneration(createRangeGroupState([{ query: 'line' }, { query: 'count', optional: true }]), range(0));
  group = publishInitialRangeOutcome(group, group.requestId, 0, line(20));
  assert.equal(readTrendUncounted(group.dataSource[1]), null);
  group = settleRangeGroupGeneration(group, group.requestId, [line(20), count()]);
  assert.equal(readTrendUncounted(group.dataSource[1]), 0);

  // Count fails for the next range: the new line shows, and stays a bound.
  const unconfirmed = settle(group, [line(40), failed], 100);
  assert.equal(unconfirmed.dataSource[0].data[0].value, 40);
  assert.equal(readTrendUncounted(unconfirmed.dataSource[1]), null);
  assert.equal(unconfirmed.range.start, 100);

  // Line fails while the count says the next range left one out: the old pair stays.
  const keptAgain = settle(group, [failed, count(1)], 100);
  assert.equal(keptAgain.dataSource[0].data[0].value, 20);
  assert.equal(readTrendUncounted(keptAgain.dataSource[1]), 0);

  // Both succeed: the new line comes with its own count.
  const next = settle(group, [line(40), count(1)], 100);
  assert.equal(next.dataSource[0].data[0].value, 40);
  assert.equal(readTrendUncounted(next.dataSource[1]), 1);
  assert.equal(next.range.start, 100);
});
