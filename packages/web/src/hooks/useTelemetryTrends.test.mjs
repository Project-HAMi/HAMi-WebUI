import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';
import { effectScope, ref } from 'vue';
import { REQUEST_STATUS } from './request-state.mjs';
import { projectTrendSeries, useTelemetryTrends } from './useTelemetryTrends.mjs';

const goldensDir = resolve(import.meta.dirname, '../../../../test/contracts/http-goldens');
const clusterGolden = JSON.parse(readFileSync(resolve(goldensDir, 'telemetry-trends-cluster.json'), 'utf8'));
const circuitOpenGolden = JSON.parse(readFileSync(resolve(goldensDir, 'telemetry-trends-circuit-open.json'), 'utf8'));

const OVERVIEW_METRICS = [
  { key: 'compute-allocation', metric: 'compute_allocation_percent', sectionIndex: 'compute' },
  { key: 'compute-usage', metric: 'compute_utilization_percent', sectionIndex: 'compute' },
  { key: 'memory-allocation', metric: 'memory_allocation_percent', sectionIndex: 'memory' },
  { key: 'memory-usage', metric: 'memory_utilization_percent', sectionIndex: 'memory' },
];

test('projectTrendSeries maps HTTP golden known series and preserves gaps without coercing to zero', () => {
  const projected = projectTrendSeries(clusterGolden, OVERVIEW_METRICS);
  assert.equal(projected.length, 4);
  for (const series of projected) {
    assert.equal(series.status, REQUEST_STATUS.READY);
    assert.deepEqual(
      series.data.map((point) => point.value),
      [null, 42.5, 50],
    );
    assert.equal(series.data[0].timestamp, Date.parse('2023-11-14T22:11:00Z'));
  }
});

test('projectTrendSeries maps circuit_open golden, no_data, and conflict states accurately', () => {
  const openProjected = projectTrendSeries(circuitOpenGolden, OVERVIEW_METRICS);
  for (const series of openProjected) {
    assert.equal(series.status, REQUEST_STATUS.ERROR);
    assert.deepEqual(series.data, []);
  }

  const mixed = projectTrendSeries({
    series: [
      { metric: 'compute_allocation_percent', state: { state: 'unknown', reason: 'no_data' }, points: [{ at: '2026-10-06T12:00:00Z' }] },
      { metric: 'compute_utilization_percent', state: { state: 'conflict', reason: 'ambiguous_series' }, points: [] },
      { metric: 'memory_allocation_percent', state: { state: 'known' }, points: [{ at: '2026-10-06T12:00:00Z' }] },
    ],
  }, OVERVIEW_METRICS);

  assert.equal(mixed[0].status, REQUEST_STATUS.MISSING);
  assert.equal(mixed[1].status, REQUEST_STATUS.INVALID);
  assert.equal(mixed[2].status, REQUEST_STATUS.MISSING);
  assert.equal(mixed[3].status, REQUEST_STATUS.ERROR);
});

test('useTelemetryTrends aborts stale requests and preserves resolved data on refresh failure', async () => {
  let call = 0;
  let abortedFirst = false;
  let resolveFirst;
  const times = ref([new Date('2026-10-06T11:00:00Z'), new Date('2026-10-06T12:00:00Z')]);
  const client = {
    getTelemetryTrends(query, { signal }) {
      call += 1;
      assert.equal(query.scope, 'cluster');
      assert.equal(query.step, undefined);
      if (call === 1) {
        return new Promise((_resolve, reject) => {
          resolveFirst = reject;
          signal.addEventListener('abort', () => {
            abortedFirst = true;
            reject(Object.assign(new Error('Aborted'), { name: 'AbortError' }));
          });
        });
      }
      if (call === 2) {
        return Promise.resolve(clusterGolden);
      }
      return Promise.reject(new Error('Upstream failed'));
    },
  };

  const scope = effectScope();
  const state = scope.run(() => useTelemetryTrends({
    scope: 'cluster',
    metrics: OVERVIEW_METRICS,
    times,
    client,
  }));

  assert.equal(call, 1);
  assert.equal(abortedFirst, false);

  times.value = [new Date('2026-10-06T11:05:00Z'), new Date('2026-10-06T12:05:00Z')];
  await Promise.resolve();
  assert.equal(abortedFirst, true);
  assert.equal(call, 2);

  await new Promise((r) => setTimeout(r, 10));
  for (const item of state.data.value) {
    assert.equal(item.status, REQUEST_STATUS.READY);
    assert.equal(item.refreshing, false);
    assert.equal(item.refreshError, null);
  }

  // Refresh failure preserves the resolved points and reports refreshError.
  await state.refresh();
  for (const item of state.data.value) {
    assert.equal(item.status, REQUEST_STATUS.READY);
    assert.equal(item.data.length, 3);
    assert.equal(item.refreshing, false);
    assert.ok(item.refreshError instanceof Error);
  }

  resolveFirst?.();
  scope.stop();
});
