import assert from 'node:assert/strict';
import test from 'node:test';

import {
  calculateMetricPercent,
  METRIC_STATUS,
  readReadyMetricField,
  resetMetricQueryState,
  requiresMetricCapacity,
  resolveMetricStatus,
} from '../../../hooks/instant-vector-state.mjs';
import { createNodeComputeUsageGaugeConfig } from './metric-config.mjs';

const config = createNodeComputeUsageGaugeConfig({
  selector: 'node=~"$node"',
});

test('node compute utilization stays normalized for one-card and eight-card nodes', () => {
  assert.equal(
    config.query,
    'avg(avg(hami_core_util{node=~"$node"}) by (node, device_uuid))',
  );
  assert.equal(config.total, 100);
  assert.equal(config.totalQuery, undefined);

  for (const deviceCount of [1, 8]) {
    const deviceSamples = Array(deviceCount).fill(41);
    const nodePercentage =
      deviceSamples.reduce((total, sample) => total + sample, 0) /
      deviceSamples.length;

    const metric = {
      ...config,
      status: METRIC_STATUS.READY,
      percent: calculateMetricPercent(nodePercentage, config.total),
    };

    assert.equal(readReadyMetricField(metric, 'percent'), 41);
  }
});

test('node compute utilization preserves loading and missing states', () => {
  const loadingMetric = {
    ...config,
    status: METRIC_STATUS.READY,
    used: 41,
    percent: 41,
  };

  resetMetricQueryState(loadingMetric, config, {
    requestId: 2,
    status: METRIC_STATUS.LOADING,
  });

  assert.equal(loadingMetric.status, METRIC_STATUS.LOADING);
  assert.equal(loadingMetric.total, 100);
  assert.equal(readReadyMetricField(loadingMetric, 'percent'), undefined);

  const missingStatus = resolveMetricStatus({
    usedStatus: METRIC_STATUS.MISSING,
    total: config.total,
    requiresCapacity: requiresMetricCapacity(config),
  });
  const missingMetric = { ...config, status: missingStatus };

  assert.equal(missingMetric.status, METRIC_STATUS.MISSING);
  assert.equal(readReadyMetricField(missingMetric, 'percent'), undefined);
});
