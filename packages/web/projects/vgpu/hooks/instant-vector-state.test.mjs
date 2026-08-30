import assert from 'node:assert/strict';
import test from 'node:test';

import {
  calculateMetricPercent,
  METRIC_STATUS,
  readInstantValue,
  readReadyMetricField,
  requiresMetricCapacity,
  resolveMetricStatus,
  setMetricErrorState,
} from './instant-vector-state.mjs';

test('an empty instant vector is distinct from a real zero sample', () => {
  assert.deepEqual(readInstantValue({ data: [] }), {
    status: METRIC_STATUS.MISSING,
    value: 0,
  });
  assert.deepEqual(readInstantValue({ data: [{ value: 0 }] }), {
    status: METRIC_STATUS.READY,
    value: 0,
  });
});

test('protobuf JSON non-finite samples are marked invalid', () => {
  for (const value of ['NaN', 'Infinity', '-Infinity', Number.NaN]) {
    assert.equal(
      readInstantValue({ data: [{ value }] }).status,
      METRIC_STATUS.INVALID,
    );
  }
});

test('detail values only expose ready samples while preserving a real zero', () => {
  assert.equal(
    readReadyMetricField(
      { status: METRIC_STATUS.READY, percent: 0 },
      'percent',
    ),
    0,
  );
  assert.equal(
    readReadyMetricField(
      { status: METRIC_STATUS.MISSING, percent: 0 },
      'percent',
    ),
    undefined,
  );
  assert.equal(
    readReadyMetricField(
      { status: METRIC_STATUS.ERROR, used: 0 },
      'used',
    ),
    undefined,
  );
});

test('percentages require a finite positive capacity', () => {
  assert.equal(calculateMetricPercent(0, 800), 0);
  assert.equal(calculateMetricPercent(1200, 800), 150);
  assert.equal(calculateMetricPercent(0, 0), undefined);
  assert.equal(calculateMetricPercent(10, undefined), undefined);
});

test('capacity semantics come from the original metric config', () => {
  assert.equal(requiresMetricCapacity({ query: 'sum(metric)' }), false);
  assert.equal(requiresMetricCapacity({ percent: 0 }), true);
  assert.equal(
    requiresMetricCapacity(Object.create({ percent: 0 })),
    false,
  );
});

test('query errors remain unknown instead of becoming missing data', () => {
  const plainMetric = {
    hasData: false,
    count: 42,
    used: 42,
    percent: 99,
  };
  setMetricErrorState(plainMetric);
  assert.equal(plainMetric.status, METRIC_STATUS.ERROR);
  assert.equal(plainMetric.hasData, undefined);
  assert.equal(Object.hasOwn(plainMetric, 'percent'), false);

  const capacityMetric = { percent: 50, total: 100 };
  setMetricErrorState(capacityMetric, {
    hasTotal: true,
    requiresCapacity: true,
  });
  assert.equal(capacityMetric.hasData, undefined);
  assert.equal(capacityMetric.totalHasData, undefined);
  assert.equal(capacityMetric.percent, 0);
  assert.equal(capacityMetric.total, 0);
});

test('metric status keeps missing, invalid and zero-capacity states distinct', () => {
  assert.equal(
    resolveMetricStatus({
      usedStatus: METRIC_STATUS.MISSING,
      totalStatus: METRIC_STATUS.READY,
      total: 800,
      requiresCapacity: true,
    }),
    METRIC_STATUS.MISSING,
  );
  assert.equal(
    resolveMetricStatus({
      usedStatus: METRIC_STATUS.READY,
      totalStatus: METRIC_STATUS.INVALID,
      total: 0,
      requiresCapacity: true,
    }),
    METRIC_STATUS.INVALID,
  );
  assert.equal(
    resolveMetricStatus({
      usedStatus: METRIC_STATUS.READY,
      totalStatus: METRIC_STATUS.READY,
      total: 0,
      requiresCapacity: true,
    }),
    METRIC_STATUS.NO_CAPACITY,
  );
  assert.equal(
    resolveMetricStatus({
      usedStatus: METRIC_STATUS.READY,
      totalStatus: METRIC_STATUS.READY,
      total: 800,
      requiresCapacity: true,
    }),
    METRIC_STATUS.READY,
  );
});
