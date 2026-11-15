import assert from 'node:assert/strict';
import test from 'node:test';

import {
  METRIC_STATUS,
  readReadyMetricField,
  resetMetricQueryState,
  requiresMetricCapacity,
  resolveMetricStatus,
} from '../../../hooks/instant-vector-state.mjs';
import { nodeResourceQueries, nodeUsageGaugeConfigs } from './metric-config.mjs';
import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';

const [config, memoryGauge] = nodeUsageGaugeConfigs;

test('node usage cards and trends use the same physical devices as the node ranking', () => {
  const ranking = buildPhysicalResourceQueries({ byNode: true });
  for (const metric of ['computeUsage', 'memoryUsage']) {
    for (const [field, query] of Object.entries(ranking[metric])) {
      assert.equal(nodeResourceQueries[metric][field], query.replaceAll('device_kind="physical"', 'device_kind="physical",node=$node,node_uid=$uid'));
    }
  }
  assert.equal(config.query, nodeResourceQueries.computeUsage.percentQuery);
  assert.equal(config.total, 100);
  assert.equal(memoryGauge.query, nodeResourceQueries.memoryUsage.query);
  assert.equal(memoryGauge.totalQuery, nodeResourceQueries.memoryUsage.totalQuery);
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
