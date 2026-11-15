import assert from 'node:assert/strict';
import test from 'node:test';

import { createNodeWorkloadDistributionQuery, createOverviewGaugeConfigs } from './metric-config.mjs';
import { getRangeConfigInit } from './config.js';
import { buildPhysicalResourceQueries } from '../../../metrics/physical-resource-queries.mjs';

test('the four header gauges and their trend lines use the same metric definitions', () => {
  const gauges = createOverviewGaugeConfigs();
  const trends = getRangeConfigInit(key => key).flatMap(section => section.dataSource);
  const metrics = buildPhysicalResourceQueries();
  assert.equal(gauges.length, 4);
  assert.equal(trends.length, 4);

  for (const [id, seriesKey, resource] of [
    ['compute-allocation', 'compute-allocation', 'computeAllocation'],
    ['compute-utilization', 'compute-usage', 'computeUsage'],
    ['memory-allocation', 'memory-allocation', 'memoryAllocation'],
    ['memory-utilization', 'memory-usage', 'memoryUsage'],
  ]) {
    const gauge = gauges.find(item => item.id === id);
    assert.equal(gauge.query, metrics[resource].query);
    assert.equal(gauge.totalQuery, metrics[resource].totalQuery);
    assert.equal(trends.find(item => item.key === seriesKey).query, metrics[resource].percentQuery);
  }
});

test('node workload distribution includes idle accelerator inventory nodes', () => {
  const query = createNodeWorkloadDistributionQuery();

  assert.equal(
    query,
    'count(count by (node, container_pod_uuid) (hami_container_vgpu_allocated)) by (node) or on (node) (count by (node) (hami_vgpu_count) * 0)',
  );
  assert.doesNotMatch(query, /vector\(0\)/);
});
