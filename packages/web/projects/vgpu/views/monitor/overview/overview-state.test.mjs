import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../../../src/hooks/request-state.mjs';
import {
  aggregateStatuses,
  applyUncountedShares,
  getPartialRangeStates,
  selectRangeAxisData,
  stateTextKey,
} from './overview-state.mjs';
import { getRangeConfigInit } from './config.js';
import { buildClusterTrendQueries } from '../../../metrics/query-contract.mjs';

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

test('the compute allocation rate reports what it left out', () => {
  const metrics = [
    { id: 'vgpu-allocation', status: REQUEST_STATUS.READY },
    { id: 'compute-allocation', status: REQUEST_STATUS.READY },
    { id: 'memory-allocation', status: REQUEST_STATUS.READY },
  ];
  const applied = applyUncountedShares(metrics, 3);

  assert.equal(applied[1].uncounted, 3);
  assert.deepEqual(applied.map(({ status }) => status), metrics.map(({ status }) => status));
  assert.equal(applied[0].uncounted, undefined);
  assert.equal(applied[2].uncounted, undefined);
  // Nothing is marked while every share is known.
  assert.deepEqual(applyUncountedShares(metrics, 0), metrics);
});

test('the compute allocation trend is marked by the same key', () => {
  const [computeSection] = getRangeConfigInit((key) => key);
  const series = computeSection.dataSource.find(
    (item) => item.query === buildClusterTrendQueries().computeAllocation,
  );
  assert.equal(series.key, 'compute-allocation');

  const [allocation, usage] = applyUncountedShares(
    [{ ...series, status: REQUEST_STATUS.READY }, { key: 'compute-usage', status: REQUEST_STATUS.READY }],
    2,
  );
  assert.equal(allocation.uncounted, 2);
  assert.equal(usage.uncounted, undefined);
});
