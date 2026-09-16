import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../../../src/hooks/request-state.mjs';
import { applyUncountedShares } from './overview-state.mjs';
import { getRangeConfigInit } from './config.js';
import { buildClusterTrendQueries } from '../../../metrics/query-contract.mjs';

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
