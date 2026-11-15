import assert from 'node:assert/strict';
import test from 'node:test';

import { formatAmountPair } from './amount-format.mjs';

test('memory pairs switch to TiB once the total reaches 1024 GiB', () => {
  assert.deepEqual(formatAmountPair(1259.4, 12384.2, 'GiB'), { used: '1.2', total: '12.1', unit: 'TiB' });
  assert.deepEqual(formatAmountPair(48, 96, 'GiB'), { used: '48', total: '96', unit: 'GiB' });
});

test('amounts keep at most one decimal, drop a trailing zero and group thousands', () => {
  assert.deepEqual(formatAmountPair(21, 187, ''), { used: '21', total: '187', unit: '' });
  assert.deepEqual(formatAmountPair(21.25, 187, ''), { used: '21.3', total: '187', unit: '' });
  assert.deepEqual(formatAmountPair(512, 1000, 'GiB'), { used: '512', total: '1,000', unit: 'GiB' });
});

test('a small nonzero amount does not read as zero', () => {
  assert.deepEqual(formatAmountPair(40, 12576.2, 'GiB'), { used: '<0.1', total: '12.3', unit: 'TiB' });
  assert.deepEqual(formatAmountPair(0, 12576.2, 'GiB'), { used: '0', total: '12.3', unit: 'TiB' });
});
