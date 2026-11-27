import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import test from 'node:test';

import { ApiClientError, buildApiUrl, createApiClient } from './client.mjs';

const goldensDir = resolve(new URL('../../../../test/contracts/http-goldens', import.meta.url).pathname);

const loadGoldenRaw = (name) => readFileSync(resolve(goldensDir, name), 'utf8');

function mockResponse(body, { status = 200, statusText = 'OK' } = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText,
    async text() {
      return body;
    },
  };
}

test('buildApiUrl normalizes basePath, omits empty params, and formats query strings', () => {
  assert.equal(buildApiUrl('nodes', undefined, '/'), '/api/v1/nodes');
  assert.equal(buildApiUrl('/nodes', { is_schedulable: false, empty: '' }, '/gpu-ui'), '/gpu-ui/api/v1/nodes?is_schedulable=false');
  assert.equal(
    buildApiUrl('containers', { page: 1, page_size: 20, status: undefined }, '/gpu-ui/'),
    '/gpu-ui/api/v1/containers?page=1&page_size=20',
  );
});

test('createApiClient encodes path segments, validates required params, and forwards AbortSignal', async () => {
  const seen = [];
  const controller = new AbortController();
  const client = createApiClient({
    getBasePathImpl: () => '/gpu-ui/',
    fetchImpl: async (url, init) => {
      seen.push({ url, init });
      return mockResponse('{"list":[]}');
    },
  });

  await client.getDevice('GPU/with space', { signal: controller.signal });
  assert.equal(seen[0].url, '/gpu-ui/api/v1/devices/GPU%2Fwith%20space');
  assert.equal(seen[0].init.method, 'GET');
  assert.equal(seen[0].init.signal, controller.signal);

  await assert.rejects(() => client.getNode(' '), TypeError);
  await assert.rejects(() => client.listNodeContainers('uid-1', ''), TypeError);
  await assert.rejects(() => client.getDevice(''), TypeError);
  await assert.rejects(() => client.getContainer('', 'ctr'), TypeError);
  await assert.rejects(() => client.getSchedulingPod('ns', 'pod', ' '), TypeError);
  await assert.rejects(() => client.getTelemetryTrends({}), TypeError);
});

test('createApiClient maps Kratos error envelopes and invalid JSON to ApiClientError', async () => {
  const badJsonClient = createApiClient({
    fetchImpl: async () => mockResponse('{bad', { status: 200 }),
  });
  await assert.rejects(
    () => badJsonClient.getAbout(),
    (err) => err instanceof ApiClientError && err.reason === 'INVALID_JSON',
  );

  const notFoundClient = createApiClient({
    fetchImpl: async () => mockResponse(loadGoldenRaw('api-v1-container-not-found.json'), { status: 404 }),
  });
  await assert.rejects(
    () => notFoundClient.getContainer('pod-uid-1', 'missing'),
    (err) =>
      err instanceof ApiClientError &&
      err.status === 404 &&
      err.code === 404 &&
      err.reason === 'RESOURCE_CONTAINER_NOT_FOUND',
  );
});

test('createApiClient parses every frozen /api/v1 HTTP golden payload cleanly', async () => {
  const routes = new Map([
    ['/api/v1/overview/inventory', { status: 200, file: 'overview-inventory.json' }],
    ['/api/v1/telemetry/trends?scope=cluster&start=2023-11-14T22%3A11%3A00Z&end=2023-11-14T22%3A13%3A00Z&step=60s', { status: 200, file: 'telemetry-trends-cluster.json' }],
    ['/api/v1/nodes?ip=10.0.0.1&is_schedulable=true', { status: 200, file: 'api-v1-nodes.json' }],
    ['/api/v1/nodes/node-uid-1', { status: 200, file: 'api-v1-node-detail.json' }],
    ['/api/v1/nodes/node-uid-1/containers?node_name=node-1', { status: 200, file: 'api-v1-node-containers.json' }],
    ['/api/v1/devices?node_name=node-1&node_uid=node-uid-1&provider=NVIDIA', { status: 200, file: 'api-v1-devices.json' }],
    ['/api/v1/devices/GPU-1', { status: 200, file: 'api-v1-device-detail.json' }],
    ['/api/v1/containers?page=1&page_size=20', { status: 200, file: 'api-v1-containers.json' }],
    ['/api/v1/containers/pod-uid-1/trainer', { status: 200, file: 'api-v1-container-detail.json' }],
    ['/api/v1/scheduling/pods/default/train-job-0?uid=pod-uid-1', { status: 200, file: 'api-v1-scheduling-pod.json' }],
    ['/api/v1/device-config', { status: 200, file: 'api-v1-device-config.json' }],
    ['/api/v1/about', { status: 200, file: 'api-v1-about.json' }],
  ]);

  const client = createApiClient({
    getBasePathImpl: () => '/',
    fetchImpl: async (url) => {
      const hit = routes.get(url);
      assert.ok(hit, `Unexpected request URL: ${url}`);
      return mockResponse(loadGoldenRaw(hit.file), { status: hit.status });
    },
  });

  const inventory = await client.getOverviewInventory();
  assert.equal(inventory.nodes[0].name, 'node-1');

  const trends = await client.getTelemetryTrends({
    scope: 'cluster',
    start: '2023-11-14T22:11:00Z',
    end: '2023-11-14T22:13:00Z',
    step: '60s',
  });
  assert.equal(trends.series.length, 4);
  assert.equal(trends.series[0].points[0].value, undefined);

  const nodes = await client.listNodes({ ip: '10.0.0.1', isSchedulable: true });
  assert.equal(nodes.list[0].uid, 'node-uid-1');

  const node = await client.getNode('node-uid-1');
  assert.equal(node.inventoryCoverage.entity, 'device');

  const nodeContainers = await client.listNodeContainers('node-uid-1', 'node-1');
  assert.equal(nodeContainers.allocationNodeUid, 'node-uid-1');

  const devices = await client.listDevices({ nodeName: 'node-1', nodeUid: 'node-uid-1', provider: 'NVIDIA' });
  assert.equal(devices.list[0].uuid, 'GPU-1');

  const device = await client.getDevice('GPU-1');
  assert.equal(device.vendor, 'NVIDIA');

  const containers = await client.listContainers({ page: 1, pageSize: 20 });
  assert.equal(containers.total, 1);
  assert.equal(containers.items[0].devices[0].id, 'GPU-1');

  const container = await client.getContainer('pod-uid-1', 'trainer');
  assert.equal(container.podUid, 'pod-uid-1');

  const scheduling = await client.getSchedulingPod('default', 'train-job-0', 'pod-uid-1');
  assert.equal(scheduling.pod.stage, 'bound');

  const deviceConfig = await client.getDeviceConfig();
  assert.equal(deviceConfig.ascendModels[0].templates[0].computeShare, 25);

  const about = await client.getAbout();
  assert.equal(about.hami.state, 'ok');
});
