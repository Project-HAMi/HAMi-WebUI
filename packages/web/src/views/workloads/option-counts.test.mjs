import assert from 'node:assert/strict';
import test from 'node:test';
import { optionCountOf, readOptionCounts } from './option-counts.mjs';

test('option counts accept only objects of integers', () => {
  assert.deepEqual(readOptionCounts({ 'node-a': 2 }), { 'node-a': 2 });
  assert.deepEqual(readOptionCounts({}), {});
  for (const value of [undefined, null, [], { 'node-a': '2' }, { 'node-a': 1.5 }]) assert.equal(readOptionCounts(value), null);
});

test('missing and inherited keys count as zero', () => {
  assert.equal(optionCountOf({}, 'constructor'), 0);
  assert.equal(optionCountOf({}, '__proto__'), 0);
  assert.equal(optionCountOf({ constructor: 3 }, 'constructor'), 3);
  assert.equal(optionCountOf({ 'node-a': 2 }, 'node-b'), 0);
  assert.equal(optionCountOf(JSON.parse('{"__proto__": 4}'), '__proto__'), 4);
});
