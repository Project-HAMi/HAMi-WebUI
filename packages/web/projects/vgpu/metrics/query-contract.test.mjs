import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildClusterTrendQueries,
  buildComputeAllocationQueries,
  buildGroupedResourceTopQueries,
  buildMemoryAllocationQueries,
  buildMemoryUsageQueries,
  buildTaskAllocationTopQueries,
  buildTaskComputeAllocationQuery,
  buildTaskCountQueries,
  buildTaskResourceOverviewQueries,
} from './query-contract.mjs';

test('compute allocation stays idle at real zero and hides unknown allocations', () => {
  const queries = buildComputeAllocationQueries();

  assert.match(queries.query, /hami_core_size\)\) \* 0/);
  assert.match(
    queries.query,
    /unless on \(\) max\(hami_container_vcore_allocation_known == 0\)/,
  );
  assert.match(
    queries.percentQuery,
    /unless on \(\) max\(hami_container_vcore_allocation_known == 0\)/,
  );
  assert.doesNotMatch(
    queries.totalQuery,
    /hami_container_vcore_allocation_known/,
  );
});

test('scoped compute allocation excludes the whole affected group', () => {
  const queries = buildComputeAllocationQueries({
    selector: 'node=~"$node"',
    groupLabel: 'node',
  });

  assert.match(
    queries.percentQuery,
    /unless on \(node\) max by \(node\) \(hami_container_vcore_allocation_known\{node=~"\$node"\} == 0\)/,
  );
});

test('task compute queries reject partial unknown allocations', () => {
  const selector =
    'container_name="$container",pod_name=~"$pod",namespace_name="$namespace"';
  const detail = buildTaskComputeAllocationQuery({ selector });
  const ranked = buildTaskComputeAllocationQuery({
    groupLabel: 'container_pod_uuid',
  });

  assert.match(
    detail,
    /^\(avg\(sum by \(instance\) \(hami_container_vcore_allocated\{/,
  );
  assert.match(
    detail,
    /unless on \(\) max\(hami_container_vcore_allocation_known\{.*\} == 0\)$/,
  );
  assert.match(
    ranked,
    /^\(avg by \(container_pod_uuid\) \(sum by \(container_pod_uuid, instance\) \(hami_container_vcore_allocated\)\)\)/,
  );
  assert.match(
    ranked,
    /unless on \(container_pod_uuid\) max by \(container_pod_uuid\)/,
  );
});

test('workload allocation rankings total cards within each exporter replica', () => {
  const queries = buildTaskAllocationTopQueries();
  const replicaSafe = (metric) =>
    `avg by (container_pod_uuid) (sum by (container_pod_uuid, instance) (${metric}))`;

  assert.equal(
    queries.compute,
    `topk(5, ((${replicaSafe('hami_container_vcore_allocated')}) unless on (container_pod_uuid) max by (container_pod_uuid) (hami_container_vcore_allocation_known == 0)) / 100)`,
  );
  assert.equal(
    queries.memory,
    `topk(5, ${replicaSafe('hami_container_vmemory_allocated')} / 1024)`,
  );
  assert.equal(
    queries.vgpu,
    `topk(5, ${replicaSafe('hami_container_vgpu_allocated')})`,
  );

  for (const query of Object.values(queries)) {
    assert.doesNotMatch(
      query,
      /avg by \(container_pod_uuid\) \(hami_container_/,
    );
  }
});

test('task resource totals stay stable for one or two exporter replicas and multiple cards', () => {
  const selector =
    'container_name="$container",pod_name=~"$pod",namespace_name="$namespace"';
  const queries = buildTaskResourceOverviewQueries({ selector });
  const vgpuSeries = `hami_container_vgpu_allocated{${selector}}`;
  const coreSeries = `hami_container_vcore_allocated{${selector}}`;
  const memorySeries = `hami_container_vmemory_allocated{${selector}}`;
  const replicaSafeVgpu = `avg(sum by (instance) (${vgpuSeries}))`;
  const replicaSafeCore = `avg(sum by (instance) (${coreSeries}))`;
  const replicaSafeMemory = `avg(sum by (instance) (${memorySeries}))`;

  // sum by (instance) totals all cards inside one exporter snapshot; averaging
  // those totals keeps one and two equivalent WebUI replicas at the same value.
  assert.equal(queries.gpuCards, replicaSafeVgpu);
  assert.equal(
    queries.computeLimit,
    `(${replicaSafeCore}) unless on () max(hami_container_vcore_allocation_known{${selector}} == 0)`,
  );
  assert.equal(
    queries.singleCardMemory,
    `${replicaSafeMemory} / clamp_min(${replicaSafeVgpu}, 1) / 1024`,
  );
});

test('workload counts do not depend on compute-allocation availability', () => {
  const queries = buildTaskCountQueries();

  for (const query of Object.values(queries)) {
    assert.match(query, /hami_container_vgpu_allocated/);
    assert.doesNotMatch(query, /hami_container_vcore_allocated/);
  }
});

test('memory allocation uses schedulable capacity and fills idle allocation', () => {
  const queries = buildMemoryAllocationQueries();

  for (const query of Object.values(queries)) {
    assert.match(query, /hami_vmemory_size/);
    assert.doesNotMatch(query, /hami_memory_size/);
  }
  assert.match(queries.query, / or /);
  assert.match(queries.percentQuery, /\* 0/);
  assert.doesNotMatch(
    queries.percentQuery,
    /hami_container_vcore_allocation_known/,
  );
});

test('scoped memory allocation keeps the same contract for node and card details', () => {
  const node = buildMemoryAllocationQueries({
    selector: 'node=~"$node"',
  });
  const card = buildMemoryAllocationQueries({
    selector: 'device_uuid=~"$device_uuid"',
  });

  assert.match(node.totalQuery, /hami_vmemory_size\{node=~"\$node"\}/);
  assert.match(
    card.totalQuery,
    /hami_vmemory_size\{device_uuid=~"\$device_uuid"\}/,
  );
  assert.match(node.percentQuery, / or /);
  assert.match(card.percentQuery, / or /);
});

test('memory usage capacity only includes devices that report usage', () => {
  const queries = buildMemoryUsageQueries();

  assert.match(
    queries.totalQuery,
    /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
  );
  assert.doesNotMatch(queries.query, / or /);
  assert.doesNotMatch(queries.totalQuery, / or /);
  assert.doesNotMatch(queries.percentQuery, /\* 0/);
});

test('grouped rankings preserve idle allocation but not missing usage', () => {
  for (const groupLabel of ['node', 'device_uuid']) {
    const queries = buildGroupedResourceTopQueries(groupLabel);

    assert.match(
      queries.memoryAllocation,
      new RegExp(`or on \\(${groupLabel}\\)`),
    );
    assert.match(queries.memoryAllocation, /hami_vmemory_size/);
    assert.match(
      queries.memoryUsage,
      /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
    );
    assert.doesNotMatch(queries.memoryUsage, / or /);
    assert.match(queries.memoryUsage, /sum by \([^)]*instance\)/);
  }
});

test('cluster trends share the allocation and reporting-device contracts', () => {
  const queries = buildClusterTrendQueries();

  assert.match(queries.computeAllocation, / or /);
  assert.match(queries.memoryAllocation, /hami_vmemory_size/);
  assert.match(queries.memoryAllocation, / or /);
  assert.match(
    queries.memoryUsage,
    /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
  );
  assert.doesNotMatch(queries.memoryUsage, / or /);
});

test('grouped query labels reject malformed PromQL input', () => {
  assert.throws(
    () => buildGroupedResourceTopQueries('node) or vector(1'),
    /Invalid Prometheus group label/,
  );
  assert.throws(
    () => buildGroupedResourceTopQueries(''),
    /group label is required/,
  );
});
