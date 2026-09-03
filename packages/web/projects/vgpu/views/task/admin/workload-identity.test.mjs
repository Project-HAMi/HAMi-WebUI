import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  createWorkloadRowKey,
  formatWorkloadName,
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
