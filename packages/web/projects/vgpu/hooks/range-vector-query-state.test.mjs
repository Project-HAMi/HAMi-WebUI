import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  createRangeGroupState,
  createRangeVectorOutcome,
  publishInitialRangeOutcome,
  readRangeVector,
  settleRangeGroupGeneration,
  startRangeGroupGeneration,
} from './range-vector-query-state.mjs';

const ready = (value) =>
  createRangeVectorOutcome({
    data: [{ timestamp: value, value }],
    status: REQUEST_STATUS.READY,
  });

const failed = (message) => {
  const error = new Error(message);
  return createRangeVectorOutcome(
    { data: [], status: REQUEST_STATUS.ERROR },
    error,
  );
};

const startGroup = (range = '1h') =>
  startRangeGroupGeneration(
    createRangeGroupState([{ name: 'Allocation' }, { name: 'Usage' }], 'compute'),
    { range },
  );

test('range query state preserves a real zero sample', () => {
  assert.deepEqual(
    readRangeVector({
      data: [{ values: [{ timestamp: 1000, value: 0 }] }],
    }),
    {
      data: [{ timestamp: 1000, value: 0 }],
      status: REQUEST_STATUS.READY,
    },
  );
});

test('an empty range vector is missing rather than zero', () => {
  assert.deepEqual(readRangeVector({ data: [] }), {
    data: [],
    status: REQUEST_STATUS.MISSING,
  });
});

test('missing and invalid range points remain distinct', () => {
  assert.equal(
    readRangeVector({
      data: [{ values: [{ timestamp: 1000, missing: true }] }],
    }).status,
    REQUEST_STATUS.MISSING,
  );
  assert.equal(
    readRangeVector({
      data: [{ values: [{ timestamp: 1000, value: 'NaN' }] }],
    }).status,
    REQUEST_STATUS.INVALID,
  );
});

test('a refresh switches every series to the new time range atomically', () => {
  let group = startGroup('1h');
  group = settleRangeGroupGeneration(group, group.requestId, [ready(1), ready(2)]);

  const refresh = startRangeGroupGeneration(group, { range: '7d' });
  const afterFirstResult = publishInitialRangeOutcome(
    refresh,
    refresh.requestId,
    0,
    ready(7),
  );

  assert.deepEqual(
    afterFirstResult.dataSource.map((series) => series.data[0].value),
    [1, 2],
    'a completed 7d series must not replace one half of the displayed 1h group',
  );
  assert.deepEqual(afterFirstResult.range, { range: '1h' });

  const settled = settleRangeGroupGeneration(
    afterFirstResult,
    refresh.requestId,
    [ready(7), ready(8)],
  );
  assert.deepEqual(
    settled.dataSource.map((series) => series.data[0].value),
    [7, 8],
  );
  assert.deepEqual(settled.range, { range: '7d' });
});

test('a failed refresh keeps the entire previous range and exposes refreshError', () => {
  let group = startGroup('1h');
  group = settleRangeGroupGeneration(group, group.requestId, [ready(1), ready(2)]);
  group = startRangeGroupGeneration(group, { range: '7d' });
  const error = failed('usage query failed');

  const settled = settleRangeGroupGeneration(
    group,
    group.requestId,
    [ready(7), error],
  );

  assert.deepEqual(
    settled.dataSource.map((series) => series.data[0].value),
    [1, 2],
  );
  assert.deepEqual(settled.range, { range: '1h' });
  assert.equal(settled.refreshError, error.error);
  assert.equal(settled.refreshing, false);
});

test('an initial generation publishes partial success and failure without an old group', () => {
  let group = startGroup('1h');
  const generation = group.requestId;
  group = publishInitialRangeOutcome(group, generation, 0, ready(1));

  assert.equal(group.dataSource[0].status, REQUEST_STATUS.READY);
  assert.equal(group.dataSource[1].status, REQUEST_STATUS.LOADING);

  const error = failed('usage query failed');
  group = publishInitialRangeOutcome(group, generation, 1, error);
  assert.equal(group.dataSource[1].status, REQUEST_STATUS.ERROR);

  group = settleRangeGroupGeneration(group, generation, [ready(1), error]);
  assert.deepEqual(group.range, { range: '1h' });
  assert.equal(group.dataSource[0].status, REQUEST_STATUS.READY);
  assert.equal(group.dataSource[1].status, REQUEST_STATUS.ERROR);
});

test('trend groups settle independently', () => {
  let compute = startGroup('7d');
  let memory = startRangeGroupGeneration(
    createRangeGroupState([{ name: 'Allocation' }, { name: 'Usage' }], 'memory'),
    { range: '7d' },
  );

  compute = settleRangeGroupGeneration(
    compute,
    compute.requestId,
    [ready(7), ready(8)],
  );

  assert.equal(compute.hasResolved, true);
  assert.equal(memory.hasResolved, false);
  assert.equal(memory.dataSource[0].status, REQUEST_STATUS.LOADING);

  memory = settleRangeGroupGeneration(
    memory,
    memory.requestId,
    [ready(9), ready(10)],
  );
  assert.equal(memory.hasResolved, true);
});

test('an older generation cannot overwrite the latest range', () => {
  const first = startGroup('1h');
  const latest = startRangeGroupGeneration(first, { range: '7d' });

  const staleResult = settleRangeGroupGeneration(
    latest,
    first.requestId,
    [ready(1), ready(2)],
  );
  assert.equal(staleResult, latest);
  assert.equal(staleResult.hasResolved, false);

  const settled = settleRangeGroupGeneration(
    latest,
    latest.requestId,
    [ready(7), ready(8)],
  );
  assert.deepEqual(settled.range, { range: '7d' });
  assert.deepEqual(
    settled.dataSource.map((series) => series.data[0].value),
    [7, 8],
  );
});
