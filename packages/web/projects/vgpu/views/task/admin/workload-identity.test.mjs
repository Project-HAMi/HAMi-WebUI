import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  buildWorkloadIdentityIndex,
  createWorkloadDetailLocation,
  createWorkloadRowKey,
  formatWorkloadName,
  parseWorkloadMetricIdentity,
  resolveWorkloadRankingIdentity,
} from './workload-identity.mjs';

test('containers in the same Pod have distinct workload row keys', () => {
  const worker = createWorkloadRowKey({ podUid: 'pod-uid', name: 'worker' });
  const sidecar = createWorkloadRowKey({ podUid: 'pod-uid', name: 'sidecar' });

  assert.equal(worker, 'pod-uid/worker');
  assert.equal(sidecar, 'pod-uid/sidecar');
  assert.notEqual(worker, sidecar);
});

test('workload name exposes both Pod and container identity', () => {
  assert.equal(
    formatWorkloadName({ appName: 'training-pod', name: 'worker' }),
    'training-pod / worker',
  );
  assert.equal(
    formatWorkloadName({ appName: 'worker', name: 'worker' }),
    'worker',
  );
  assert.equal(formatWorkloadName(), '--');
});

test('rankings resolve the exact Pod and container without changing their metric identity', () => {
  const workloads = [
    { podUid: 'pod-a', name: 'main', appName: 'training', namespace: 'research' },
    { podUid: 'pod-b', name: 'main', appName: 'training', namespace: 'production' },
    { podUid: 'pod-a', name: 'sidecar', appName: 'training', namespace: 'research' },
  ];
  const index = buildWorkloadIdentityIndex(workloads);
  for (const workload of workloads) {
    const metricName = `${workload.name}:${workload.podUid}`;
    const result = resolveWorkloadRankingIdentity(metricName, index);
    assert.deepEqual(result, {
      metricName,
      identity: { name: workload.name, podUid: workload.podUid },
      appName: workload.appName,
      namespace: workload.namespace,
    });
  }
});

test('ranking lookup retains names outside the filtered workload table', () => {
  const inventory = [
    { podUid: 'pod-a', name: 'main', appName: 'training', namespace: 'research' },
    { podUid: 'pod-b', name: 'main', appName: 'serving', namespace: 'production' },
  ];
  const index = buildWorkloadIdentityIndex(inventory);
  const filteredTableRows = inventory.filter((row) => row.appName === 'serving');
  assert.equal(filteredTableRows.length, 1);
  assert.equal(resolveWorkloadRankingIdentity('main:pod-a', index).appName, 'training');
  assert.equal(inventory.length, 2);
});

test('missing inventory preserves routing identity without guessing a Pod name', () => {
  const index = buildWorkloadIdentityIndex([
    { podUid: 'pod-new', name: 'main', appName: 'recreated-pod', namespace: 'research' },
    { name: 'main', appName: 'missing-uid' },
  ]);
  assert.deepEqual(resolveWorkloadRankingIdentity('main:pod-old', index), {
    metricName: 'main:pod-old',
    identity: { name: 'main', podUid: 'pod-old' },
    appName: '',
    namespace: '',
  });
  assert.deepEqual(resolveWorkloadRankingIdentity('main:pod-old'),
    resolveWorkloadRankingIdentity('main:pod-old', new Map()));
  assert.deepEqual(createWorkloadDetailLocation('main:pod-old'), {
    name: 'workload-detail',
    params: { podUid: 'pod-old', container: 'main' },
  });
});

test('display names do not become workload detail identifiers', () => {
  const metricName = 'worker:pod-a';
  const index = buildWorkloadIdentityIndex([
    { name: 'worker', podUid: 'pod-a', appName: 'training-pod', namespaceName: 'research' },
  ]);
  const workload = resolveWorkloadRankingIdentity(metricName, index);
  const displayName = formatWorkloadName({ ...workload.identity, appName: workload.appName });
  assert.equal(displayName, 'training-pod / worker');
  assert.equal(workload.namespace, 'research');
  assert.equal(createWorkloadDetailLocation(displayName), null);
  assert.deepEqual(createWorkloadDetailLocation(metricName).params, { podUid: 'pod-a', container: 'worker' });
});

test('unresolved metric labels do not produce an invalid details link', () => {
  for (const metricName of ['', '-', undefined, 'main:', ':pod-a', 'main:pod-a:extra']) {
    assert.equal(parseWorkloadMetricIdentity(metricName), null);
    assert.equal(createWorkloadDetailLocation(metricName), null);
    assert.equal(resolveWorkloadRankingIdentity(metricName).identity, null);
  }
});

test('workload rows use one link for the middle-truncated Pod and full container', () => {
  const source = readFileSync(new URL('./index.vue', import.meta.url), 'utf8');

  assert.match(source, /<RouterLink[\s\S]*?class="workload-identity-primary workload-identity-link"/);
  assert.match(source, /aria-label=\{workloadName\}/);
  assert.match(source, /class="workload-pod-name"/);
  assert.match(source, /class="workload-container-name"/);
  assert.match(source, /class="workload-identity-label"/);
  assert.match(source, /class="workload-namespace-line"/);
  assert.match(source, /\{t\('task\.namespace'\)\}:/);
  assert.match(source, /class="workload-pod-name"[\s\S]*?<EllipsisText[^>]*mode="middle"/);
  assert.match(source, /class="workload-container-name"[\s\S]*?<EllipsisText[^>]*tooltip="overflow"/);
  assert.match(source, /\.workload-identity-label[\s\S]*?display:\s*inline-flex;/);
  assert.match(source, /\.workload-identity-label[\s\S]*?align-items:\s*baseline;/);
  assert.match(source, /\.workload-pod-name[\s\S]*?flex:\s*0 1 auto;/);
  assert.match(source, /\.workload-pod-name[\s\S]*?max-width:\s*240px;/);
  assert.match(source, /\.workload-container-name[\s\S]*?flex:\s*0 0 auto;/);
  assert.match(source, /\.workload-container-name[\s\S]*?max-width:\s*240px;/);
  assert.match(source, /\.workload-identity-label::after[\s\S]*?height:\s*1px;/);
  assert.match(source, /\.workload-namespace-line[\s\S]*?align-items:\s*baseline;/);
  assert.doesNotMatch(source, /\.workload-identity-link\s*\{[^}]*width:\s*fit-content;/s);
  assert.doesNotMatch(source, /\.workload-pod-name::after/);
  assert.doesNotMatch(source, /\.workload-container-name::after/);
  assert.doesNotMatch(source, /<TextPlus/);
});
