import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

import { parseDeviceSplitPage } from '../src/components/device-split.mjs';
import { readOverviewInventory } from '../src/api/overview-inventory.mjs';
import { classifyDetailPayload } from '../src/hooks/detail-resource-state.mjs';
import { REQUEST_STATUS } from '../src/hooks/request-state.mjs';

const goldensDir = resolve(new URL('../../../test/contracts/http-goldens', import.meta.url).pathname);

const loadGolden = (name) => JSON.parse(readFileSync(resolve(goldensDir, name), 'utf8'));

test('frontend contract parsers match serialized Kratos HTTP JSON goldens', () => {
  const overview = loadGolden('overview-inventory.json');
  const parsedOverview = readOverviewInventory(overview);
  assert.equal(parsedOverview.nodes.length, 1);
  assert.equal(parsedOverview.devices.length, 1);
  assert.equal(parsedOverview.devices[0].utilizationRatio, 0.42);
  assert.equal(parsedOverview.devices[0].memoryUsageRatio, 0.25);

  const emptyOverview = loadGolden('overview-inventory-empty.json');
  const parsedEmptyOverview = readOverviewInventory(emptyOverview);
  assert.deepEqual(parsedEmptyOverview.nodes, []);
  assert.deepEqual(parsedEmptyOverview.devices, []);
  assert.equal(parsedEmptyOverview.nodeCoverage.complete, true);

  const gpuDetail = loadGolden('gpu-detail-ok.json');
  assert.equal(
    classifyDetailPayload(gpuDetail, {
      identityKeys: ['uuid'],
      expectedIdentity: { uuid: 'GPU-1' },
    }),
    REQUEST_STATUS.READY,
  );

  const gpuNotFound = loadGolden('gpu-detail-not-found.json');
  assert.equal(gpuNotFound.code, 404);
  assert.equal(gpuNotFound.reason, 'RESOURCE_GPU_NOT_FOUND');
  assert.equal(
    classifyDetailPayload({}, { identityKeys: ['uuid'], expectedIdentity: { uuid: 'GPU-MISSING' } }),
    REQUEST_STATUS.MISSING,
  );

  const workloadsPage = loadGolden('workloads-page.json');
  const page = parseDeviceSplitPage(workloadsPage, 100);
  assert.equal(page.items.length, 1);
  assert.equal(page.totalContainers, 1);
  assert.equal(page.truncated, false);

  const workloadsEmpty = loadGolden('workloads-empty.json');
  const emptyPage = parseDeviceSplitPage(workloadsEmpty, 100);
  assert.deepEqual(emptyPage, { items: [], totalContainers: 0, truncated: false });
});
