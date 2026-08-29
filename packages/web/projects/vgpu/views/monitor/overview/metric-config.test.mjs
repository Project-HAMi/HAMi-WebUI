import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createComputeUsageGaugeConfig,
  createNodeAllocationTopQueries,
} from './metric-config.mjs';

test('compute utilization is treated as an already-normalized percentage', () => {
  const config = createComputeUsageGaugeConfig();

  assert.equal(config.total, 100);
  assert.equal(config.totalQuery, undefined);
  assert.match(config.query, /hami_core_util/);
  assert.equal((41 / config.total) * 100, 41);
});

test('node allocation rankings include every device and fully idle nodes', () => {
  const queries = createNodeAllocationTopQueries();

  assert.equal(
    queries.compute,
    'topk(5, (avg by (node) (sum by (node, instance) (hami_container_vcore_allocated)) / avg by (node) (sum by (node, instance) (hami_core_size)) * 100) or on (node) (avg by (node) (sum by (node, instance) (hami_core_size)) * 0))',
  );
  assert.equal(
    queries.memory,
    'topk(5, (avg by (node) (sum by (node, instance) (hami_container_vmemory_allocated)) / avg by (node) (sum by (node, instance) (hami_memory_size)) * 100) or on (node) (avg by (node) (sum by (node, instance) (hami_memory_size)) * 0))',
  );
});
