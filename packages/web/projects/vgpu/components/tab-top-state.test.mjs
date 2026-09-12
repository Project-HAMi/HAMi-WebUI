import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  buildRankingItems,
  formatRankingValue,
  readRankingRows,
} from './tab-top-state.mjs';

test('count rankings keep counts and scale bars against the largest count', () => {
  const rows = [{ name: 'small', value: 4 }, { name: 'large', value: 8 }];
  const items = buildRankingItems(rows, '');
  assert.deepEqual(items, [
    { name: 'large', value: 8, index: 1, percentage: 100, valueDisplay: '8' },
    { name: 'small', value: 4, index: 2, percentage: 50, valueDisplay: '4' },
  ]);
  assert.deepEqual(rows.map((item) => item.name), ['small', 'large']);
  assert.equal(buildRankingItems([{ name: 'idle', value: 0 }], '')[0].percentage, 0);
});

test('percent rankings retain an absolute 100 percent scale and raw values', () => {
  const rows = [{ name: 'normal', value: 20 }, { name: 'overallocated', value: 120 }];
  for (const unit of [undefined, null, '%', ' % ']) {
    const items = buildRankingItems(rows, unit);
    assert.equal(items[0].percentage, 100);
    assert.equal(items[0].valueDisplay, '120 %');
    assert.equal(items[1].percentage, 20);
    assert.equal(items[1].valueDisplay, '20 %');
  }
});

test('other ranking units retain relative bars and meaningful fractions', () => {
  const items = buildRankingItems([
    { name: 'small', value: 0.25 },
    { name: 'large', value: 0.5 },
  ], 'GiB');
  assert.equal(items[0].percentage, 100);
  assert.equal(items[0].valueDisplay, '0.5 GiB');
  assert.equal(items[1].percentage, 50);
  assert.equal(items[1].valueDisplay, '0.25 GiB');
  assert.deepEqual(buildRankingItems([], ''), []);
});

test('ranking values preserve meaningful fractional data', () => {
  assert.equal(formatRankingValue(0.4, '%'), '0.4 %');
  assert.equal(formatRankingValue(1.25, 'GiB'), '1.25 GiB');
  assert.equal(formatRankingValue(4, ' '), '4');
  assert.equal(formatRankingValue(0, '%'), '0 %');
  assert.equal(formatRankingValue(0.004, '%'), '<0.01 %');
});

test('ranking rows preserve real zero and reject invalid-only results', () => {
  assert.deepEqual(
    readRankingRows(
      { data: [{ metric: { node: 'worker-1' }, value: 0 }] },
      'node',
    ),
    {
      data: [{ name: 'worker-1', value: 0 }],
      status: REQUEST_STATUS.READY,
    },
  );
  assert.equal(
    readRankingRows({ data: [{ value: 'NaN' }] }, 'node').status,
    REQUEST_STATUS.INVALID,
  );
});

test('ranking rows render valid partial data without manufacturing zeroes', () => {
  assert.deepEqual(
    readRankingRows(
      {
        data: [
          { metric: { node: 'bad' }, value: undefined },
          { metric: { node: 'idle' }, value: '0' },
        ],
      },
      'node',
    ),
    {
      data: [{ name: 'idle', value: 0 }],
      status: REQUEST_STATUS.READY,
    },
  );
});
