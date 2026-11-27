import assert from 'node:assert/strict';
import { test } from 'node:test';
import { sumMigInstanceMemory } from './mig-instance-memory.mjs';

const uuid = 'GPU-parent';
const sample = (id, field, value, labels = {}) => ({
  metric: { UUID: uuid, GPU_I_ID: id, instance: 'exporter', __name__: 'DCGM_FI_DEV_FB_' + field, ...labels },
  value,
});

test('sum GPU instance memory once per GI and keep the ratio within the same instances', () => {
  const rows = [sample('0', 'USED', 10240), sample('0', 'FREE', 10240)];
  for (const id of ['2', '3', '4', '5', '6']) rows.push(sample(id, 'USED', 5120), sample(id, 'FREE', 5120));
  rows.push(sample('0', 'USED_BYTES', 10240 * 1024 ** 2), sample('0', 'USED', 10240),
    sample('', 'USED', 35840), sample('0', 'USED', 10240, { GPU_CI_ID: '1' }));
  assert.deepEqual(sumMigInstanceMemory(rows, uuid), { usedGiB: 35, totalGiB: 70, percent: 50 });
  assert.equal(sumMigInstanceMemory([...rows, sample('2', 'USED', 0)], uuid), null);
  assert.equal(sumMigInstanceMemory([...rows, sample('2', 'USED', 5120, { instance: 'other' })], uuid), null);
});

test('distinguish measured zero, missing capacity, and a missing instance usage reading', () => {
  const used = sample('0', 'USED', 0);
  assert.deepEqual(sumMigInstanceMemory([used], uuid), { usedGiB: 0, totalGiB: undefined, percent: undefined });
  assert.deepEqual(sumMigInstanceMemory([used, sample('0', 'FREE', 1024)], uuid), { usedGiB: 0, totalGiB: 1, percent: 0 });
  assert.equal(sumMigInstanceMemory([used, sample('1', 'FREE', 5120)], uuid), null);
  assert.equal(sumMigInstanceMemory([], uuid), null);
});

test('keep Pod-labelled GI usage and exclude per-process readings, including relabelled copies', () => {
  const pod = { pod: 'training', container: 'worker' };
  const process = { ...pod, pod_uid: 'workload-uid' };
  const rows = [sample('0', 'USED', 10240, pod), sample('0', 'FREE', 10240, pod),
    sample('0', 'USED', 512, process), sample('0', 'FREE', 10240, process),
    sample('0', 'USED', 512, { exported_pod_uid: 'workload-uid' })];
  assert.deepEqual(sumMigInstanceMemory(rows, uuid), { usedGiB: 10, totalGiB: 20, percent: 50 });
  assert.equal(sumMigInstanceMemory(rows.slice(2), uuid), null);
});
