import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { buildPhysicalAllocationQueries, buildPhysicalResourceQueries } from '../projects/vgpu/metrics/physical-resource-queries.mjs';

import { nodeVendorDistributionQuery } from '../projects/vgpu/metrics/query-contract.mjs';
import { nodeResourceQueries } from '../projects/vgpu/views/node/admin/metric-config.mjs';
import { renderPromQLTemplate } from '../projects/vgpu/metrics/promql-template.mjs';

const promtool = process.env.PROMTOOL || 'promtool';
const queries = buildPhysicalResourceQueries();
const nodeQueries = buildPhysicalResourceQueries({ byNode: true });
const scopedNode = buildPhysicalAllocationQueries({ selector: 'node="node-a"' });
const nodeDetail = buildPhysicalResourceQueries({ byNode: true, selector: 'node="node-a"' });
const metricNames = {
  info: 'hami_webui_device_info',
  capacity: 'hami_webui_device_compute_capacity_ratio',
  allocated: 'hami_webui_device_compute_allocated_ratio',
  usage: 'hami_webui_device_utilization_ratio',
  unknown: 'hami_webui_device_compute_unknown_occupancies',
  memoryCapacity: 'hami_webui_device_memory_capacity_bytes',
  memoryAllocated: 'hami_webui_device_memory_allocated_bytes',
  memoryUsed: 'hami_webui_device_memory_used_bytes',
  memoryPhysical: 'hami_webui_device_memory_physical_bytes',
};

const labelsText = (labels) => `{${Object.entries(labels)
  .sort(([left], [right]) => left.localeCompare(right))
  .map(([key, value]) => `${key}=${JSON.stringify(value)}`).join(',')}}`;

const series = (metric, value, labels = {}) => ({
  series: `${metricNames[metric]}${labelsText({
    device_id: 'gpu-1', node: 'node-a', provider: 'hami-nvidia',
    device_kind: 'physical', model: 'model-a', instance: 'replica-a',
    ...(metric === 'info' ? { configured: 'true' } : {}),
    ...(['usage', 'memoryUsed', 'memoryPhysical'].includes(metric) ? { source: 'dcgm' } : {}),
    ...labels,
  })}`,
  values: String(value),
});

const sample = (value, labels = {}) => ({ labels: labelsText(labels), value });
const check = (expr, expected, evalTime = '0m') => ({
  expr, eval_time: evalTime,
  exp_samples: expected === undefined ? [] : Array.isArray(expected) ? expected : [sample(expected)],
});
const fixture = (name, inputSeries, expressionTests) => ({
  name, interval: '1m', input_series: inputSeries, promql_expr_test: expressionTests,
});

const currentNode = query => renderPromQLTemplate(query, { node: 'node-a', uid: 'new-uid' });

const fixtures = [
  fixture('a node with both HCU and DCU counts once for Hygon', [
    { series: 'hami_vgpu_count{node="dual",provider="DCU"}', values: '4' },
    { series: 'hami_vgpu_count{node="dual",provider="HCU"}', values: '4' },
    { series: 'hami_vgpu_count{node="other",provider="HCU"}', values: '8' },
    { series: 'hami_vgpu_count{node="dual",provider="NVIDIA"}', values: '1' },
  ], [check(nodeVendorDistributionQuery, [sample(2, { provider: 'Hygon' }), sample(1, { provider: 'NVIDIA' })])]),
  fixture('a replacement node only owns readings explicitly labeled with its UID', [
    ...[['old-uid', '0.8 stale _'], ['new-uid', '_ 0.1 0.1'], ['', '0.7 0.7 0.7']].flatMap(([uid, values]) => {
      const labels = uid ? { node_uid: uid } : {};
      return [series('usage', values, labels), series('allocated', values, labels),
        series('info', '1 1 1', labels), series('unknown', '0 0 0', labels), series('capacity', '1 1 1', labels)];
    }),
  ], [
    check(currentNode(nodeResourceQueries.computeUsage.percentQuery), undefined),
    check(currentNode(nodeResourceQueries.computeUsage.percentQuery), [sample(10, { node: 'node-a' })], '1m'),
    check(currentNode(nodeResourceQueries.computeAllocation.percentQuery), undefined),
    check(currentNode(nodeResourceQueries.computeAllocation.percentQuery), 10, '1m'),
  ]),
  fixture('fractional Ascend shares stay on the requested node for both allocation trends', [
    series('info', 1), series('capacity', 1), series('allocated', 0.296875), series('unknown', 0),
    series('memoryAllocated', 0), series('memoryCapacity', 1024),
    ...['info', 'capacity', 'allocated', 'memoryAllocated', 'memoryCapacity'].map((key) => series(key, 1, { node: 'node-b' })),
    series('unknown', 0, { node: 'node-b' }),
  ], [check(scopedNode.computeAllocation.percentQuery, 29.6875), check(scopedNode.memoryAllocation.percentQuery, 0)]),
  fixture('capacity shares, not physical card count: 0.75 / 1.5 = 50%', [
    series('info', 1), series('capacity', 1.5), series('allocated', 0.75), series('unknown', 0),
  ], [
    check(queries.computeAllocation.query, 0.75),
    check(queries.computeAllocation.totalQuery, 1.5),
    check(queries.computeAllocation.percentQuery, 50),
  ]),
  fixture('one vote per device: nine idle cards plus one busy card = 10%', [
    ...Array.from({ length: 9 }, (_, index) => series('usage', 0, { device_id: `idle-${index}` })),
    series('usage', 1, { device_id: 'busy', model: 'model-b', provider: 'other-provider' }),
  ], [check(queries.computeUsage.query, 10), check(queries.computeUsage.percentQuery, 10)]),
  fixture('agreeing exporter replicas do not change shares, device weight or unknown counts', [
    ...['replica-a', 'replica-b'].flatMap((instance) => [
      series('info', 1, { instance }), series('capacity', 1.5, { instance }),
      series('allocated', 0.75, { instance }), series('usage', 0.2, { instance }),
      series('unknown', 0, { instance }),
    ]),
    series('info', 1, { device_id: 'gpu-2' }), series('capacity', 1, { device_id: 'gpu-2' }),
    series('allocated', 0.25, { device_id: 'gpu-2' }), series('usage', 0.8, { device_id: 'gpu-2' }),
    series('unknown', 0, { device_id: 'gpu-2' }),
  ], [
    check(queries.computeAllocation.query, 1), check(queries.computeAllocation.totalQuery, 2.5),
    check(queries.computeAllocation.percentQuery, 40), check(queries.computeUsage.query, 50),

  ]),
  fixture('conflicting utilization replicas exclude that device, not select max or average', [
    series('usage', 0.2), series('usage', 0.8, { instance: 'replica-b' }),
    series('usage', 0.4, { device_id: 'gpu-2' }),
  ], [check(queries.computeUsage.query, 40)]),
  fixture('competing sources exclude the device even when their values agree', [
    series('usage', 0.2), series('usage', 0.2, { source: 'other-source' }),
    series('usage', 0.8, { device_id: 'gpu-2' }),
  ], [check(queries.computeUsage.query, 80)]),
  fixture('a conflicting source cannot make another competing source become authoritative', [
    series('usage', 0.2), series('usage', 0.3, { instance: 'replica-b' }),
    series('usage', 0.8, { source: 'other-source' }),
    series('usage', 0.25, { device_id: 'gpu-2' }),
  ], [check(queries.computeUsage.query, 25)]),
  fixture('logical, instance and unverified device kinds are not physical usage votes', [
    series('usage', 0.3),
    ...['logical', 'instance', 'unknown'].map((device_kind) => series('usage', 1, {
      device_id: device_kind, device_kind,
    })),
  ], [check(queries.computeUsage.query, 30)]),
  fixture('no observations remain missing, never capacity-derived idle zero', [], [
    ...['computeAllocation', 'computeUsage', 'memoryAllocation', 'memoryUsage']
      .flatMap((key) => Object.values(queries[key]).map((expr) => check(expr, undefined))),
  ]),
  fixture('an explicit complete empty allocation and measured idle usage remain known zero', [
    series('info', 1), series('capacity', 1), series('allocated', 0),
    series('usage', 0), series('unknown', 0),
  ], [
    check(queries.computeAllocation.query, 0), check(queries.computeAllocation.percentQuery, 0),
    check(queries.computeUsage.query, 0),
  ]),
  fixture('missing capacity removes the device from both sides, preserving paired devices', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5), series('unknown', 0),
    series('info', 1, { device_id: 'gpu-2', node: 'node-b' }),
    series('allocated', 0.1, { device_id: 'gpu-2', node: 'node-b' }),
    series('unknown', 0, { device_id: 'gpu-2', node: 'node-b' }),
  ], [
    check(queries.computeAllocation.query, 0.5),
    check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 50),
    check(nodeQueries.computeAllocation.totalQuery, [sample(1, { node: 'node-a' })]),
    check(nodeQueries.computeAllocation.percentQuery, [sample(50, { node: 'node-a' })]),
  ]),
  fixture('conflicting capacity excludes the same device from numerator and denominator', [
    series('info', 1), series('capacity', 1), series('capacity', 1.5, { instance: 'replica-b' }),
    series('allocated', 0.5), series('unknown', 0),
    series('info', 1, { device_id: 'gpu-2' }), series('capacity', 1, { device_id: 'gpu-2' }),
    series('allocated', 0.25, { device_id: 'gpu-2' }), series('unknown', 0, { device_id: 'gpu-2' }),
  ], [
    check(queries.computeAllocation.query, 0.25),
    check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 25),
  ]),
  fixture('unconfigured occupancy and capacity are both excluded', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5), series('unknown', 0),
    series('info', 1, { device_id: 'unconfigured', configured: 'false' }),
    series('capacity', 2, { device_id: 'unconfigured' }),
    series('allocated', 0.25, { device_id: 'unconfigured' }),
    series('unknown', 0, { device_id: 'unconfigured' }),
  ], [
    check(queries.computeAllocation.query, 0.5), check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 50),
  ]),
  fixture('missing and partial allocation knowledge never masquerade as an exact rate', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5), series('unknown', 0),
    ...['partial', 'missing-unknown', 'missing-allocated'].flatMap((device_id) => [
      series('info', 1, { device_id }), series('capacity', 2, { device_id }),
      ...(device_id === 'missing-allocated' ? [] : [series('allocated', 1, { device_id })]),
      ...(device_id === 'missing-unknown' ? [] : [series('unknown', device_id === 'partial' ? 2 : 0, { device_id })]),
    ]),
  ], [
    check(queries.computeAllocation.query, 0.5), check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 50),
  ]),
  fixture('conflicting allocated and unknown counts exclude both sides of those devices', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5), series('unknown', 0),
    ...['allocation-conflict', 'unknown-conflict'].flatMap((device_id) => [
      series('info', 1, { device_id }), series('capacity', 2, { device_id }),
      series('allocated', 1, { device_id }), series('unknown', 0, { device_id }),
      series(device_id === 'allocation-conflict' ? 'allocated' : 'unknown', 2, { device_id, instance: 'replica-b' }),
    ]),
  ], [
    check(queries.computeAllocation.query, 0.5), check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 50),
  ]),
  fixture('zero or negative capacity and negative allocation are not valid pairs', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5), series('unknown', 0),
    ...['zero-capacity', 'negative-capacity', 'negative-allocation'].flatMap((device_id) => [
      series('info', 1, { device_id }), series('unknown', 0, { device_id }),
      series('capacity', device_id === 'zero-capacity' ? 0 : device_id === 'negative-capacity' ? -1 : 2, { device_id }),
      series('allocated', device_id === 'negative-allocation' ? -1 : 1, { device_id }),
    ]),
  ], [
    check(queries.computeAllocation.query, 0.5), check(queries.computeAllocation.totalQuery, 1),
    check(queries.computeAllocation.percentQuery, 50),
  ]),
  fixture('an inventory with no paired devices stays empty, not zero', [
    series('info', 1), series('capacity', 1), series('allocated', 0.5),
  ], [
    check(queries.computeAllocation.query, undefined), check(queries.computeAllocation.totalQuery, undefined),
    check(queries.computeAllocation.percentQuery, undefined),
  ]),
  fixture('memory queries keep byte-weighted totals and deduplicate identical replicas', [
    ...['replica-a', 'replica-b'].flatMap((instance) => [
      series('memoryAllocated', 2 * 1073741824, { instance }),
      series('memoryCapacity', 8 * 1073741824, { instance }),
      series('memoryUsed', 1073741824, { instance }),
      series('memoryPhysical', 4 * 1073741824, { instance }),
    ]),
  ], [
    check(queries.memoryAllocation.query, 2), check(queries.memoryAllocation.totalQuery, 8),
    check(queries.memoryAllocation.percentQuery, 25),
    check(queries.memoryUsage.query, 1), check(queries.memoryUsage.totalQuery, 4),
    check(queries.memoryUsage.percentQuery, 25),
  ]),
  fixture('memory allocation pairs its own fields regardless of unknown compute shares', [
    series('memoryAllocated', 2 * 1073741824), series('memoryCapacity', 8 * 1073741824), series('unknown', 2),
    series('memoryAllocated', 100 * 1073741824, { device_id: 'no-capacity' }),
    series('memoryCapacity', 100 * 1073741824, { device_id: 'no-allocation' }),
    series('memoryAllocated', 100 * 1073741824, { device_id: 'zero-capacity' }),
    series('memoryCapacity', 0, { device_id: 'zero-capacity' }),
  ], [
    check(queries.memoryAllocation.query, 2), check(queries.memoryAllocation.totalQuery, 8),
    check(queries.memoryAllocation.percentQuery, 25),
  ]),
  fixture('one card cannot be counted twice by competing physical memory sources', [
    ...['first', 'second'].flatMap(source => [
      series('memoryUsed', 10, { source, instance: source }),
      series('memoryPhysical', 100, { source, instance: source }),
    ]),
    series('memoryUsed', 100, { device_id: 'gpu-2' }),
    series('memoryPhysical', 100, { device_id: 'gpu-2' }),
  ], [
    check(queries.memoryUsage.percentQuery, 100),
    check(nodeQueries.memoryUsage.percentQuery, [sample(100, { node: 'node-a' })]),
  ]),
  fixture('memory halves reported by different targets cannot form a pair', [
    series('memoryUsed', 10, { instance: 'first' }),
    series('memoryPhysical', 100, { instance: 'second' }),
    series('memoryUsed', 20, { device_id: 'same-instance', job: 'first' }),
    series('memoryPhysical', 100, { device_id: 'same-instance', job: 'second' }),
    series('memoryAllocated', 1, { instance: 'first' }),
    series('memoryCapacity', 10, { instance: 'second' }),
  ], [
    check(queries.memoryUsage.query), check(queries.memoryUsage.totalQuery), check(queries.memoryUsage.percentQuery),
    check(queries.memoryAllocation.percentQuery),
  ]),
  fixture('memory uses complete agreeing pairs, keeps zero, and excludes conflicting or ambiguous evidence', [
    series('memoryUsed', 0), series('memoryPhysical', 1073741824),
    series('memoryUsed', 0, { instance: 'partial' }),
    ...['different-used', 'different-capacity', 'second-source', 'empty-capacity'].flatMap(device_id => [
      series('memoryUsed', 5, { device_id }), series('memoryPhysical', device_id === 'empty-capacity' ? 0 : 20, { device_id }),
    ]),
    series('memoryUsed', 6, { device_id: 'different-used', instance: 'partial' }),
    series('memoryPhysical', 40, { device_id: 'different-capacity', instance: 'partial' }),
    series('memoryPhysical', 20, { device_id: 'second-source', instance: 'partial', source: 'other' }),
  ], [
    check(queries.memoryUsage.query, 0), check(queries.memoryUsage.totalQuery, 1), check(queries.memoryUsage.percentQuery, 0),
  ]),
  fixture('node detail and rank measure both vendors once and drop conflicting replicas', [
    series('usage', 0.8), series('memoryUsed', 10), series('memoryPhysical', 40),
    ...['replica-a', 'replica-b'].flatMap(instance => {
      const ascend = { device_id: 'npu-1', provider: 'hami-ascend', model: '910B', source: 'npu-exporter', instance };
      return [series('usage', 0.2, ascend), series('memoryUsed', 50, ascend), series('memoryPhysical', 60, ascend)];
    }),
    ...['replica-a', 'replica-b'].flatMap((instance, index) => {
      const conflict = { device_id: 'conflict', instance };
      return [series('usage', index, conflict), series('memoryUsed', index, conflict), series('memoryPhysical', 100, conflict)];
    }),
    series('usage', 0.9, { node: 'node-b' }),
    series('memoryUsed', 5, { node: 'node-b' }), series('memoryPhysical', 50, { node: 'node-b' }),
  ], [
    check(nodeQueries.computeUsage.percentQuery, [sample(50, { node: 'node-a' }), sample(90, { node: 'node-b' })]),
    check(nodeQueries.memoryUsage.percentQuery, [sample(60, { node: 'node-a' }), sample(10, { node: 'node-b' })]),
    check(nodeDetail.computeUsage.query, [sample(50, { node: 'node-a' })]),
    check(nodeDetail.memoryUsage.percentQuery, [sample(60, { node: 'node-a' })]),
  ]),
  fixture('impossible physical occupancy drops that whole card while schedulable allocation can exceed capacity', [
    series('memoryUsed', 90), series('memoryPhysical', 80),
    series('memoryAllocated', 90), series('memoryCapacity', 80),
    series('info', 1), series('unknown', 0), series('allocated', 1.5), series('capacity', 1),
    series('memoryUsed', 10, { device_id: 'valid' }), series('memoryPhysical', 40, { device_id: 'valid' }),
  ], [
    check(queries.memoryUsage.percentQuery, 25),
    check(queries.memoryUsage.query, 10 / 2 ** 30), check(queries.memoryUsage.totalQuery, 40 / 2 ** 30),
    check(nodeDetail.memoryUsage.percentQuery, [sample(25, { node: 'node-a' })]),
    check(queries.memoryAllocation.percentQuery, 112.5), check(queries.computeAllocation.percentQuery, 150),
  ]),
  fixture('only fleet compute usage is smoothed; allocations, memory use and node usage are current', [
    series('info', '1 1'), series('capacity', '1 1'), series('allocated', '0 1'), series('unknown', '0 0'),
    series('usage', '0 1'), series('memoryAllocated', '0 20'), series('memoryCapacity', '20 20'),
    series('memoryUsed', '0 10'), series('memoryPhysical', '10 10'),
  ], [
    check(queries.computeUsage.percentQuery, 50, '1m30s'),
    check(nodeQueries.computeUsage.percentQuery, [sample(100, { node: 'node-a' })], '1m30s'),
    check(queries.computeAllocation.percentQuery, 100, '1m30s'),
    check(queries.memoryAllocation.percentQuery, 100, '1m30s'),
    check(queries.memoryUsage.percentQuery, 100, '1m30s'),
  ]),
  fixture('compute numerator and denominator disappear and return together when capacity is stale', [
    series('info', '1 1 1'), series('capacity', '1 stale 2'),
    series('allocated', '0.5 0.5 1'), series('unknown', '0 0 0'),
  ], [
    check(queries.computeAllocation.query, 0.5, '0m'),
    check(queries.computeAllocation.totalQuery, 1, '0m'),
    check(queries.computeAllocation.percentQuery, 50, '0m'),
    check(queries.computeAllocation.query, undefined, '1m'),
    check(queries.computeAllocation.totalQuery, undefined, '1m'),
    check(queries.computeAllocation.percentQuery, undefined, '1m'),
    check(queries.computeAllocation.query, 1, '2m'),
    check(queries.computeAllocation.totalQuery, 2, '2m'),
    check(queries.computeAllocation.percentQuery, 50, '2m'),
  ]),
  fixture('fleet usage smooths a spike but disappears when the last card stops reporting', [
    series('usage', '0 1 stale'),
  ], [
    check(queries.computeUsage.query, 0, '0m'),
    check(queries.computeUsage.query, 50, '1m30s'),
    check(queries.computeUsage.query, undefined, '2m'),
  ]),
  {
    ...fixture('the five-minute window counts measured card-time, including idle zero', [
      series('usage', '1+0x10'),
      series('usage', '0+0x4 stale', { device_id: 'exited' }),
      series('usage', 'stale stale stale stale stale 0+0x5', { device_id: 'joined' }),
      series('usage', '0 stale', { device_id: 'first-tick-only' }),
    ], [
      // Ten ticks, each with a busy and an idle card. The extra idle card
      // at 0m drops out at 5m, even though it was once a valid observation.
      check(queries.computeUsage.query, 1000 / 21, '4m30s'),
      check(queries.computeUsage.query, 50, '5m'),
      check(nodeQueries.computeUsage.percentQuery, [sample(50, { node: 'node-a' })], '5m'),
    ]),
    interval: '30s',
  },
];

test('homepage metric expressions execute against Prometheus', (t) => {
  const directory = mkdtempSync(join(tmpdir(), 'hami-homepage-promql-'));
  try {
    // Promtool accepts JSON as YAML. No running Prometheus or network is needed.
    const inputPath = join(directory, 'homepage.test.json');
    writeFileSync(inputPath, JSON.stringify({
      rule_files: [], evaluation_interval: '1m', tests: fixtures,
    }, null, 2));
    const result = spawnSync(promtool, ['test', 'rules', inputPath], {
      encoding: 'utf8', timeout: 30_000, maxBuffer: 4 * 1024 * 1024,
    });
    const output = [result.stdout, result.stderr].filter(Boolean).join('\n').trim();
    t.diagnostic(`${fixtures.length} fixtures, ${fixtures.reduce((count, item) => count + item.promql_expr_test.length, 0)} expression assertions`);
    assert.ifError(result.error);
    assert.equal(result.status, 0, output || `promtool exited with signal ${result.signal}`);
    t.diagnostic(output);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
