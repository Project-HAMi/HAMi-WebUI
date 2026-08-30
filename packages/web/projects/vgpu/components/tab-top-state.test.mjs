import assert from 'node:assert/strict';
import test from 'node:test';

import { REQUEST_STATUS } from '../../../src/hooks/request-state.mjs';
import {
  formatRankingValue,
  readRankingRows,
} from './tab-top-state.mjs';

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
