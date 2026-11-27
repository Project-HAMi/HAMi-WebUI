import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  buildComputeAllocationQueries,
  buildMemoryAllocationQueries,
  buildMemoryUsageQueries,
  buildUnknownComputeShareQuery,
} from './query-contract.mjs';
import {
  promQLStringLiteral,
  renderPromQLTemplate,
} from './promql-template.mjs';

test('PromQL templates render complete escaped string literals', () => {
  assert.equal(promQLStringLiteral('gpu.node-1'), '"gpu.node-1"');
  assert.equal(promQLStringLiteral('GPU-"lab"'), '"GPU-\\"lab\\""');
  assert.equal(promQLStringLiteral('rack\\slot'), '"rack\\\\slot"');
  assert.equal(promQLStringLiteral('train\nv1'), '"train\\nv1"');

  assert.equal(
    renderPromQLTemplate(
      'metric{node=$node,device_uuid=$device_uuid,pod=$pod}',
      {
        node: 'gpu.node-1',
        device_uuid: 'GPU-"lab"\\slot',
        pod: 'train\nv1',
      },
    ),
    'metric{node="gpu.node-1",device_uuid="GPU-\\"lab\\"\\\\slot",pod="train\\nv1"}',
  );
  assert.equal(
    renderPromQLTemplate('metric{device_uuid=$device_uuid,pod=$pod}', {
      device_uuid: 'GPU-$pod',
      pod: 'train-v1',
    }),
    'metric{device_uuid="GPU-$pod",pod="train-v1"}',
  );
  assert.throws(
    () => renderPromQLTemplate('metric{node=$node}', {}),
    /Missing PromQL template variable: node/,
  );
});

test('detail queries do not regex-match exact resource identities', () => {
  const exactIdentityRegex =
    /\b(?:node|pod|pod_name|container|container_name|namespace|namespace_name|device_uuid)=~/;
  const detailFiles = [
    '../views/nodes/Detail.vue',
    '../views/accelerators/Detail.vue',
    '../views/workloads/Detail.vue',
  ];

  for (const file of detailFiles) {
    const source = readFileSync(new URL(file, import.meta.url), 'utf8');
    assert.doesNotMatch(source, exactIdentityRegex, file);
  }
});

test('compute allocation stays idle at real zero and keeps what it can measure', () => {
  const queries = buildComputeAllocationQueries();
  const scoped = buildComputeAllocationQueries({
    selector: 'node=$node',
    groupLabel: 'node',
  });

  assert.match(queries.query, /hami_core_size\)\) \* 0/);
  // A lower bound, never a scope-wide hole: the exporter omits the allocations
  // it cannot state, and buildUnknownComputeShareQuery counts them.
  for (const query of [
    queries.query,
    queries.percentQuery,
    queries.totalQuery,
    scoped.query,
    scoped.percentQuery,
  ]) {
    assert.doesNotMatch(query, /hami_container_vcore_allocation_known/);
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
    selector: 'node=$node',
  });
  const card = buildMemoryAllocationQueries({
    selector: 'device_uuid=$device_uuid',
  });

  assert.match(node.totalQuery, /hami_vmemory_size\{node=\$node\}/);
  assert.match(
    card.totalQuery,
    /hami_vmemory_size\{device_uuid=\$device_uuid\}/,
  );
  assert.match(node.percentQuery, / or /);
  assert.match(card.percentQuery, / or /);
});

test('memory usage only includes devices that report both used and capacity', () => {
  const queries = buildMemoryUsageQueries();

  assert.match(
    queries.query,
    /hami_memory_used and on \(instance, node, provider, device_uuid\) hami_memory_size/,
  );
  assert.match(
    queries.totalQuery,
    /hami_memory_size and on \(instance, node, provider, device_uuid\) hami_memory_used/,
  );
  assert.doesNotMatch(queries.query, / or /);
  assert.doesNotMatch(queries.totalQuery, / or /);
  assert.doesNotMatch(queries.percentQuery, /\* 0/);
});

test('uncounted compute shares are counted once per allocation', () => {
  // max by the allocation's identity first: every exporter replica repeats it.
  assert.equal(
    buildUnknownComputeShareQuery(),
    'count(max by (container_pod_uuid, device_uuid) (hami_container_vcore_allocation_known == 0))',
  );
  assert.equal(
    buildUnknownComputeShareQuery({ groupLabel: 'device_uuid' }),
    'count by (device_uuid) (max by (device_uuid, container_pod_uuid) (hami_container_vcore_allocation_known == 0))',
  );
  assert.equal(
    buildUnknownComputeShareQuery({ selector: 'node="node-a"' }),
    'count(max by (container_pod_uuid, device_uuid) (hami_container_vcore_allocation_known{node="node-a"} == 0))',
  );
  assert.equal(
    buildUnknownComputeShareQuery({ groupLabel: 'node' }),
    'count by (node) (max by (node, container_pod_uuid, device_uuid) (hami_container_vcore_allocation_known == 0))',
  );
  assert.throws(() => buildUnknownComputeShareQuery({ groupLabel: '1bad' }), /Invalid Prometheus group label/);
});
