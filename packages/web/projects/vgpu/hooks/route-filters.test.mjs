import assert from 'node:assert/strict';
import test from 'node:test';
import { readQuery, sameQuery, writeQuery } from './route-filters.mjs';

test('readQuery keeps only listed non-empty string values', () => {
  assert.deepEqual(readQuery({ q: 'a100', type: '', nodeName: ['n1', 'n2'], other: 'x' }, ['q', 'type', 'nodeName']), {
    q: 'a100',
    type: undefined,
    nodeName: 'n1',
  });
  assert.deepEqual(readQuery(undefined, ['q']), { q: undefined });
});

test('writeQuery drops empty values and keeps unrelated keys', () => {
  assert.deepEqual(writeQuery({ q: 'old', from: 'overview' }, { q: '', type: 'A100' }), { from: 'overview', type: 'A100' });
});

test('sameQuery compares the first value of every key', () => {
  assert.equal(sameQuery({ q: 'a' }, { q: ['a'] }), true);
  assert.equal(sameQuery({ q: 'a' }, { q: 'a', type: '' }), true);
  assert.equal(sameQuery({ q: 'a' }, { q: 'b' }), false);
});
