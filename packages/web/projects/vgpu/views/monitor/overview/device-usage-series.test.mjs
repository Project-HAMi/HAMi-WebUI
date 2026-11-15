import assert from 'node:assert/strict';
import test from 'node:test';
import { makeUsageSeries } from './device-usage-series.mjs';

const from = (rates) => makeUsageSeries(rates.map(compute => ({ compute })), 'compute', 8);

test('equal readings stay exact; zero and missing use separate counts', () => {
  const zero = from(Array(8).fill(0));
  assert.deepEqual(zero.dots.map(dot => [dot.value, dot.cards.length]), [[0, 8]]);
  const mixed = from([0, 0, 0, 94, null]);
  assert.deepEqual(mixed.dots.map(dot => [dot.value, dot.cards.length]), [[0, 3], [94, 1]]);
  assert.equal(mixed.missing.length, 1);
});

test('adjacent readings share an interval while retaining every original value', () => {
  const values = [0, 1, 2, 3, 4, 5, 6, 7];
  assert.deepEqual(from(values).bins[0].map(card => card.compute), values);
  const sparse = from([4, 5, 6, null]);
  assert.equal(sparse.bins[0].length, 3);
  assert.equal(sparse.missing.length, 1);
  const edges = from([0, 9.999, 10, 89.99, 90, 100]);
  assert.deepEqual(edges.bins.map(bucket => bucket.length), [2, 1, 0, 0, 0, 0, 0, 0, 1, 2]);
});

test('the number of usable readings determines the chart for each metric', () => {
  const spaced = [0, 14, 28, 42, 56, 70, 84, 100];
  assert.equal(from([...spaced, null, null]).dots.length, 8);
  assert.equal(from([...spaced, 90]).bins.flat().length, 9);
  assert.equal(from(Array(20).fill(100)).dots[0].cards.length, 20);
  const absent = from([null, undefined, NaN, Infinity, 101, -1]);
  assert.equal(absent.missing.length, 6);
  assert.equal(absent.bins, null);
  assert.deepEqual(absent.dots, []);
});
