import assert from 'node:assert/strict';
import test from 'node:test';

import { effectScope, nextTick, ref, watch } from 'vue';

import useTrendTimeRange from './useTrendTimeRange.js';

const NOW = new Date('2026-10-06T12:00:00Z').getTime();
const HOUR = 60 * 60 * 1000;
const range = (hours = 1, end = NOW) => [new Date(end - hours * HOUR), new Date(end)];
const values = (dates) => dates.map((date) => date.getTime());

function mount(t, initial = range()) {
  const scope = effectScope();
  t.after(() => scope.stop());
  const model = ref(initial);
  const updates = [];
  const queries = [];
  let clock = NOW;
  let state;
  scope.run(() => {
    watch(model, (value) => queries.push(value), { immediate: true });
    state = useTrendTimeRange(model, (value) => {
      updates.push(value);
      model.value = value;
    }, () => clock);
  });
  return { state, model, updates, queries, advance: (milliseconds) => { clock += milliseconds; } };
}

test('a parent-provided non-default preset is preserved without issuing a second query', async (t) => {
  const { state, model, updates, queries } = mount(t, range(6));
  await nextTick();
  assert.equal(state.currentDateRange.value, '6h');
  assert.deepEqual(values(model.value), values(range(6)));
  assert.equal(updates.length, 0);
  assert.equal(queries.length, 1);
});

test('an absolute parent range opens as custom without being replaced', async (t) => {
  const initial = range(2.5, NOW - HOUR);
  const { state, model, updates } = mount(t, initial);
  await nextTick();
  assert.equal(state.currentDateRange.value, 'custom');
  assert.deepEqual(values(state.customDateRange.value), values(initial));
  assert.deepEqual(values(model.value), values(initial));
  assert.equal(updates.length, 0);
});

test('missing or invalid initial values produce exactly one default range', async (t) => {
  for (const initial of [[], null, ['', ''], ['invalid', 'invalid'], range(1, NOW + HOUR)]) {
    const { state, model, updates } = mount(t, initial);
    await nextTick();
    assert.equal(state.currentDateRange.value, '1h');
    assert.deepEqual(values(model.value), values(range()));
    assert.equal(updates.length, 1);
  }
});

test('reselecting the current preset refreshes to now exactly once per activation', async (t) => {
  const { state, model, updates, queries, advance } = mount(t);
  advance(61_000);
  state.selectRange('1h');
  await nextTick();
  assert.deepEqual(values(model.value), values(range(1, NOW + 61_000)));
  assert.equal(updates.length, 1);
  assert.equal(queries.length, 2);

  advance(61_000);
  state.selectRange('1h');
  await nextTick();
  assert.deepEqual(values(model.value), values(range(1, NOW + 122_000)));
  assert.equal(updates.length, 2);
  assert.equal(queries.length, 3);
});

test('switching presets and entering custom always shows the applied range', async (t) => {
  const { state, updates, advance } = mount(t);
  state.selectRange('custom');
  assert.deepEqual(values(state.customDateRange.value), values(range()));
  assert.equal(updates.length, 0);

  const selected = range(2, NOW - HOUR);
  state.applyCustomRange(selected);
  await nextTick();
  assert.deepEqual(values(state.customDateRange.value), values(selected));
  assert.equal(state.currentDateRange.value, 'custom');
  assert.equal(updates.length, 1);

  advance(10_000);
  state.selectRange('3h');
  await nextTick();
  state.selectRange('custom');
  assert.deepEqual(values(state.customDateRange.value), values(range(3, NOW + 10_000)));
  assert.equal(updates.length, 2);
});

test('canceling a custom draft restores the applied range without emitting', async (t) => {
  const { state, updates, model } = mount(t);
  state.selectRange('custom');
  state.customDateRange.value = range(24, NOW - HOUR);
  state.resetCustomRange();
  await nextTick();
  assert.deepEqual(values(state.customDateRange.value), values(range()));
  assert.deepEqual(values(model.value), values(range()));
  assert.equal(updates.length, 0);
});

test('an echoed custom update and an unchanged confirmation do not loop or refetch', async (t) => {
  const { state, model, updates, queries } = mount(t);
  state.selectRange('custom');
  const selected = range(2.5, NOW - HOUR);
  state.applyCustomRange(selected);
  await nextTick();
  await nextTick();
  assert.equal(updates.length, 1);
  assert.equal(queries.length, 2);

  model.value = selected.map((date) => new Date(date));
  await nextTick();
  state.applyCustomRange(selected);
  await nextTick();
  assert.equal(updates.length, 1);
  assert.equal(state.currentDateRange.value, 'custom');
});

test('an external parent update synchronizes the picker without emitting', async (t) => {
  const { state, model, updates } = mount(t);
  model.value = range(24);
  await nextTick();
  assert.equal(state.currentDateRange.value, '24h');
  state.selectRange('custom');
  model.value = range(3, NOW - HOUR);
  await nextTick();
  assert.equal(state.currentDateRange.value, 'custom');
  assert.deepEqual(values(state.customDateRange.value), values(model.value));
  assert.equal(updates.length, 0);
});

test('confirming the displayed seconds does not refetch a range supplied with milliseconds', async (t) => {
  const initial = range(1, NOW - 123);
  const { state, model, updates, queries } = mount(t, initial);
  state.selectRange('custom');
  state.applyCustomRange(initial.map((date) => new Date(Math.floor(date.getTime() / 1000) * 1000)));
  await nextTick();
  assert.deepEqual(values(model.value), values(initial));
  assert.equal(updates.length, 0);
  assert.equal(queries.length, 1);
});

test('empty, incomplete, malformed, reversed, equal, and future custom ranges cannot change charts', async (t) => {
  const { state, model, updates, queries } = mount(t);
  state.selectRange('custom');
  const invalidRanges = [
    [[], 'invalid'],
    [[new Date(NOW - HOUR)], 'invalid'],
    [['', ''], 'invalid'],
    [[null, NOW], 'invalid'],
    [['not-a-date', new Date(NOW)], 'invalid'],
    [[new Date(NOW), new Date(NOW - HOUR)], 'order'],
    [[new Date(NOW), new Date(NOW)], 'order'],
    [range(1, NOW + 1), 'future'],
  ];
  for (const [selected, error] of invalidRanges) {
    state.applyCustomRange(selected);
    await nextTick();
    assert.equal(state.validationError.value, error);
    assert.deepEqual(values(model.value), values(range()));
    assert.deepEqual(values(state.customDateRange.value), values(range()));
  }
  assert.equal(updates.length, 0);
  assert.equal(queries.length, 1);
  state.applyCustomRange(range(6));
  await nextTick();
  assert.equal(state.validationError.value, '');
  assert.equal(updates.length, 1);
});
