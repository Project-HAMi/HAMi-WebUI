import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  aggregateStatuses,
  getPartialRangeStates,
  selectRangeAxisData,
  stateTextKey,
  summarizeRangeSeries,
} from './metric-state.mjs';

test('a panel renders partial real data instead of hiding it behind an error', () => {
  assert.equal(
    aggregateStatuses([
      { status: REQUEST_STATUS.READY },
      { status: REQUEST_STATUS.ERROR },
    ]),
    REQUEST_STATUS.READY,
  );
});

test('loading, error, invalid, no-capacity and missing remain distinct', () => {
  assert.equal(
    aggregateStatuses([{ status: REQUEST_STATUS.LOADING }]),
    REQUEST_STATUS.LOADING,
  );
  assert.equal(
    aggregateStatuses([{ status: REQUEST_STATUS.ERROR }]),
    REQUEST_STATUS.ERROR,
  );
  assert.equal(
    aggregateStatuses([{ status: REQUEST_STATUS.INVALID }]),
    REQUEST_STATUS.INVALID,
  );
  assert.equal(
    aggregateStatuses([{ status: 'no-capacity' }]),
    'no-capacity',
  );
  assert.equal(aggregateStatuses([]), REQUEST_STATUS.MISSING);
});

test('a ready partial range exposes the state of each unavailable series', () => {
  const missing = { name: 'Usage', status: REQUEST_STATUS.MISSING };
  assert.deepEqual(
    getPartialRangeStates([
      { name: 'Allocation', status: REQUEST_STATUS.READY },
      missing,
    ]),
    [missing],
  );
  assert.deepEqual(
    getPartialRangeStates([{ status: REQUEST_STATUS.LOADING }]),
    [],
  );
  assert.equal(
    stateTextKey(REQUEST_STATUS.LOADING),
    'common.loading',
  );
});

test('inventory errors do not use metric-missing copy', () => {
  assert.equal(
    stateTextKey(REQUEST_STATUS.ERROR, { metric: false }),
    'common.requestError',
  );
  assert.equal(
    stateTextKey(REQUEST_STATUS.INVALID, { metric: false }),
    'common.requestError',
  );
  assert.equal(
    stateTextKey(REQUEST_STATUS.MISSING),
    'dashboard.metricNoData',
  );
});

test('a partial range panel takes its axis from an available series', () => {
  const readyData = [{ timestamp: 1, value: 0 }];
  assert.equal(
    selectRangeAxisData([
      { status: REQUEST_STATUS.MISSING, data: [] },
      { status: REQUEST_STATUS.READY, data: readyData },
    ]),
    readyData,
  );
});

test('a chart names each series it cannot draw and keeps its refresh state', () => {
  const t = (key, params) => (params ? `${key}(${params.name}:${params.state})` : key);
  const failure = new Error('timeout');
  const summary = summarizeRangeSeries([
    { name: 'Allocation', status: REQUEST_STATUS.READY, refreshError: failure },
    { name: 'Usage', status: REQUEST_STATUS.MISSING, refreshError: failure },
  ], t);

  assert.equal(summary.status, REQUEST_STATUS.READY);
  assert.equal(summary.note, 'dashboard.partialState(Usage:dashboard.metricNoData)');
  assert.equal(summary.refreshing, false);
  assert.equal(summary.refreshError, failure);
  // A line still arriving is a refresh, not a note that appears and disappears.
  assert.deepEqual(
    summarizeRangeSeries([
      { name: 'Allocation', status: REQUEST_STATUS.READY },
      { name: 'Usage', status: REQUEST_STATUS.LOADING },
    ], t),
    { status: REQUEST_STATUS.READY, note: '', refreshing: true, refreshError: null },
  );
});
