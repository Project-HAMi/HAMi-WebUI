import assert from 'node:assert/strict';
import test from 'node:test';

import { createWorkloadDistributionOptions } from './workload-distribution.mjs';

const translate = (key) => key;

test('workload distribution never clips buckets containing more than five nodes', () => {
  const options = createWorkloadDistributionOptions({
    rows: Array.from({ length: 7 }, () => ({ value: 4 })),
    translate,
  });

  assert.deepEqual(options.xAxis.data, ['0-9']);
  assert.deepEqual(options.series[0].data, [7]);
  assert.equal(Object.hasOwn(options.yAxis, 'max'), false);
});

test('workload distribution keeps boundary values in distinct buckets', () => {
  const options = createWorkloadDistributionOptions({
    rows: [{ value: 0 }, { value: 9 }, { value: 10 }, { value: 20 }],
    translate,
  });

  assert.deepEqual(options.xAxis.data, ['0-9', '10-19', '20-29']);
  assert.deepEqual(options.series[0].data, [2, 1, 1]);
});
