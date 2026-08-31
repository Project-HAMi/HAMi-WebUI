import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createComputeUsageGaugeConfig,
  createNodeTopQueries,
  createOverviewGaugeConfigs,
} from './metric-config.mjs';

test('compute utilization is treated as an already-normalized percentage', () => {
  const config = createComputeUsageGaugeConfig();

  assert.equal(config.total, 100);
  assert.equal(config.totalQuery, undefined);
  assert.match(config.query, /hami_core_util/);
  assert.equal((41 / config.total) * 100, 41);
});

test('overview gauges declare display semantics independently of translated titles', () => {
  const configs = createOverviewGaugeConfigs();

  assert.deepEqual(
    configs.map(({ titleKey, detailMode, displayDivisor = 1 }) => ({
      titleKey,
      detailMode,
      displayDivisor,
    })),
    [
      {
        titleKey: 'dashboard.vgpuAllocRate',
        detailMode: 'ratio',
        displayDivisor: 1,
      },
      {
        titleKey: 'dashboard.computeAllocRate',
        detailMode: 'ratio',
        displayDivisor: 100,
      },
      {
        titleKey: 'dashboard.memAllocRate',
        detailMode: 'ratio',
        displayDivisor: 1,
      },
      {
        titleKey: 'dashboard.computeUsageRate',
        detailMode: 'value',
        displayDivisor: 1,
      },
      {
        titleKey: 'dashboard.memUsageRate',
        detailMode: 'ratio',
        displayDivisor: 1,
      },
    ],
  );
});

test('allocation gauges use their defined capacity contracts', () => {
  const [vgpu, compute, memory] = createOverviewGaugeConfigs();

  assert.match(vgpu.totalQuery, /hami_vgpu_count/);
  assert.match(compute.totalQuery, /hami_core_size/);
  assert.doesNotMatch(compute.totalQuery, /hami_vcore_size/);
  assert.match(memory.totalQuery, /hami_vmemory_size/);
  assert.doesNotMatch(memory.totalQuery, /hami_memory_size/);
});

test('an idle cluster reports zero allocation when capacity is present', () => {
  const [vgpu, compute, memory] = createOverviewGaugeConfigs();

  assert.match(vgpu.query, /or \(avg\(sum\(hami_vgpu_count\)/);
  assert.match(
    compute.query,
    /or \(avg\(sum by \(instance\) \(hami_core_size\)\)/,
  );
  assert.match(
    memory.query,
    /or \(avg\(sum by \(instance\) \(hami_vmemory_size\)\)/,
  );
});

test('memory utilization capacity only includes reporting devices', () => {
  const memoryUtilization = createOverviewGaugeConfigs().at(-1);

  assert.match(
    memoryUtilization.totalQuery,
    /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
  );
});

test('node allocation rankings keep idle nodes and exclude unknown compute scopes', () => {
  const queries = createNodeTopQueries();

  assert.equal(
    queries.computeAllocation,
    'topk(5, ((avg by (node) (sum by (node, instance) (hami_container_vcore_allocated)) / avg by (node) (sum by (node, instance) (hami_core_size)) * 100) or on (node) (avg by (node) (sum by (node, instance) (hami_core_size)) * 0)) unless on (node) max by (node) (hami_container_vcore_allocation_known == 0))',
  );
  assert.equal(
    queries.memoryAllocation,
    'topk(5, (avg by (node) (sum by (node, instance) (hami_container_vmemory_allocated)) / avg by (node) (sum by (node, instance) (hami_vmemory_size)) * 100) or on (node) (avg by (node) (sum by (node, instance) (hami_vmemory_size)) * 0))',
  );
  assert.match(
    queries.memoryUsage,
    /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
  );
  assert.doesNotMatch(queries.memoryUsage, / or /);
});
