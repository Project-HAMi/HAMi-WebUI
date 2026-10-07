import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildNodeDeviceAllocations,
  createNodeDeviceAllocationLoader,
  readAllocationNumber,
} from './node-device-allocation.mjs';

const node = { uid: 'node-uid', name: 'node-a' };
const device = (overrides = {}) => ({
  uuid: 'gpu-a', nodeUid: node.uid, nodeName: node.name, mode: 'hami-core',
  memoryTotal: 81920, coreTotal: 100, memoryUsed: 4096, coreUsed: 20,
  coreUsedKnown: true, vgpuUsed: 1, vgpuTotal: 10,
  ...overrides,
});
const allocation = (overrides = {}) => ({
  id: 'gpu-a', allocatedMem: 4096, allocatedCores: 20, allocatedCoresKnown: true,
  allocationShape: 'soft', ...overrides,
});
const container = (overrides = {}) => ({
  podUid: 'pod-a', name: 'worker', namespace: 'research', nodeUid: node.uid,
  devices: [allocation()], ...overrides,
});
const build = (devices = [device()], containers = [container()], rest = {}) =>
  buildNodeDeviceAllocations({ node, devices, containers, ...rest });
const deferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
  return { promise, resolve, reject };
};
const flush = () => new Promise((resolve) => setImmediate(resolve));

test('allocation values retain real zero and reject missing or invalid measurements', () => {
  for (const value of [undefined, null, '', ' ', false, true, NaN, Infinity, -1, '-1', {}, []]) {
    assert.equal(readAllocationNumber(value), undefined);
  }
  for (const value of [0, '0']) assert.equal(readAllocationNumber(value), 0);
  assert.equal(readAllocationNumber('12.5'), 12.5);
});

test('node inventory is stably ordered and excludes other node lifecycles', () => {
  const devices = [device({ uuid: 'gpu-z', vgpuUsed: 0, memoryUsed: 0, coreUsed: 0 }), device(), device({ uuid: 'old-gpu', nodeUid: 'old-node-uid' })];
  const originalOrder = devices.map((item) => item.uuid);
  const entries = build(devices, [container(), container({ nodeUid: 'old-node-uid', podUid: 'old-pod' })]);
  assert.deepEqual(entries.map((entry) => entry.device.uuid), ['gpu-a', 'gpu-z']);
  assert.deepEqual(devices.map((item) => item.uuid), originalOrder);
  assert.equal(entries[0].allocationStatus, 'complete');
  assert.equal(entries[0].containerCount, 1);
  assert.equal(entries[1].containerCount, 0);
});

test('more than 100 holders are counted and segmented without paging truncation', () => {
  const containers = Array.from({ length: 125 }, (_, i) => container({
    podUid: `pod-${i}`, devices: [allocation({ allocatedMem: 100, allocatedCores: 1 })],
  }));
  const [entry] = build([device({ vgpuUsed: 125, memoryUsed: 12500, coreUsed: 125 })], containers);
  assert.equal(entry.allocationStatus, 'complete');
  assert.equal(entry.containerCount, 125);
  assert.equal(entry.split.holders.length, 125);
  assert.equal(entry.split.compute.parts.length, 125);
  assert.equal(entry.split.compute.over, 25);
});

test('one container holding two MIG instances stays one container with two allocation rows', () => {
  const [entry] = build([device({
    mode: 'mig', vgpuUsed: 2, memoryUsed: 20480, coreUsed: 28,
    migProfiles: [{ name: '1g.10gb', placements: [{ start: 0, size: 1 }, { start: 1, size: 1 }] }],
  })], [container({ devices: [0, 1].map((start) => allocation({
    allocationShape: 'mig', allocatedMem: 10240, allocatedCores: 14,
    template: '1g.10gb', migStart: start, migSize: 1,
  })) })]);
  assert.equal(entry.allocationStatus, 'complete');
  assert.equal(entry.containerCount, 1);
  assert.equal(entry.split.kind, 'mig');
  assert.equal(entry.split.holders.length, 2);
  assert.notEqual(entry.split.holders[0].key, entry.split.holders[1].key);
});

test('each device gets only its own allocations from multi-device containers', () => {
  const [a, b] = build([device(), device({ uuid: 'gpu-b', memoryUsed: 8192, coreUsed: 40 })], [container({
    devices: [allocation(), allocation({ id: 'gpu-b', allocatedMem: 8192, allocatedCores: 40 })],
  })]);
  for (const entry of [a, b]) {
    assert.equal(entry.allocationStatus, 'complete');
    assert.equal(entry.containerCount, 1);
    assert.equal(entry.split.holders.length, 1);
  }
  assert.equal(a.split.memory.used, 4096);
  assert.equal(b.split.memory.used, 8192);
});

test('missing holders and mismatched totals never yield a free layout', () => {
  for (const [devices, containers] of [
    [[device()], []],
    [[device({ vgpuUsed: 2 })], [container()]],
    [[device({ memoryUsed: 8192 })], [container()]],
    [[device({ coreUsed: 30 })], [container()]],
    [[device({ vgpuUsed: undefined })], [container()]],
    [[device({ memoryTotal: undefined })], [container()]],
    [[device({ coreTotal: 0 })], [container()]],
  ]) {
    const [entry] = build(devices, containers);
    assert.equal(entry.allocationStatus, 'partial');
    assert.equal(entry.split, undefined);
  }
});

test('missing values and unknown shapes retain their records without inventing zeroes', () => {
  for (const overrides of [
    { allocatedMem: undefined }, { allocatedMem: null }, { allocatedCores: undefined },
    { allocatedCoresKnown: undefined }, { allocatedCoresKnown: false },
    { allocationShape: 'unknown' }, { allocationShape: 'template' },
  ]) {
    const row = container({ devices: [allocation(overrides)] });
    const [entry] = build([device()], [row]);
    assert.equal(entry.allocationStatus, 'partial');
    assert.equal(entry.split, undefined);
    assert.equal(entry.containers[0], row);
    assert.equal(entry.containerCount, 1);
  }
  assert.equal(build([device({ coreUsedKnown: false })])[0].allocationStatus, 'partial');
});

test('known zero compute remains unlimited, while confirmed empty allocations remain empty', () => {
  const [unlimited] = build([device({ coreUsed: 0 })], [container({ devices: [allocation({ allocatedCores: 0 })] })]);
  assert.equal(unlimited.allocationStatus, 'complete');
  assert.equal(unlimited.split.unlimited, 1);
  assert.equal(unlimited.containerCount, 1);

  const [empty] = build([device({ vgpuUsed: 0, memoryUsed: 0, coreUsed: 0 })], []);
  assert.equal(empty.allocationStatus, 'complete');
  assert.equal(empty.containerCount, 0);
  assert.equal(empty.split.holders.length, 0);
});

test('unknown modes, unconfigured devices and unavailable holders remain visible', () => {
  assert.equal(build([device({ mode: '' })])[0].allocationStatus, 'partial');
  assert.equal(build([device({ unconfigured: true })])[0].allocationStatus, 'unconfigured');
  const [entry] = build([device()], [], { containersAvailable: false });
  assert.equal(entry.allocationStatus, 'unavailable');
  assert.equal(entry.containerCount, undefined);
  assert.equal(entry.split, undefined);
});

test('MIG with missing placement cannot fall back to a made-up memory layout', () => {
  const [entry] = build([device({ mode: 'mig' })], [container({ devices: [allocation({ allocationShape: 'mig' })] })]);
  assert.equal(entry.allocationStatus, 'partial');
  assert.equal(entry.split, undefined);
});

test('MIG without registered geometry retains allocations without inferring device extent', () => {
  const holders = [container({ devices: [allocation({
    allocationShape: 'mig', template: '1g.10gb', migStart: 0, migSize: 1,
  })] })];
  for (const migProfiles of [
    undefined, [], [{ name: '1g.10gb' }],
    [{ name: '1g.10gb', placements: [{ start: 0, size: 0 }] }],
    [{ name: '1g.10gb', placements: [{ start: 0, size: 1 }] }, { name: 'incomplete-profile' }],
  ]) {
    const [entry] = build([device({ mode: 'mig', migProfiles })], holders);
    assert.equal(entry.allocationStatus, 'partial');
    assert.equal(entry.split, undefined);
    assert.equal(entry.containers[0].devices[0].template, '1g.10gb');
    assert.equal(entry.containerCount, 1);
  }
});

test('overlapping or out-of-registration MIG instances do not show remaining capacity', () => {
  const migProfiles = [{ name: '1g.10gb', placements: [{ start: 0, size: 1 }, { start: 1, size: 1 }] }];
  for (const starts of [[0, 0], [0, 2]]) {
    const holders = starts.map((start, index) => container({
      podUid: `pod-${index}`,
      devices: [allocation({ allocationShape: 'mig', template: '1g.10gb', migStart: start, migSize: 1 })],
    }));
    const [entry] = build([device({
      mode: 'mig', vgpuUsed: 2, memoryUsed: 8192, coreUsed: 40, migProfiles,
    })], holders);
    assert.equal(entry.allocationStatus, 'partial');
    assert.equal(entry.split, undefined);
    assert.equal(entry.containerCount, 2);
  }
});

test('node reload cancels previous requests and ignores responses from old routes', async () => {
  const requests = [];
  const states = [];
  const loader = createNodeDeviceAllocationLoader({
    getDevices: (name, signal) => { const request = deferred(); requests.push({ name, signal, request }); return request.promise; },
    getContainers: async () => ({ items: [] }),
    onChange: (state) => states.push(state),
  });
  const oldLoad = loader.load(node);
  await flush();
  const next = { uid: 'node-b-uid', name: 'node-b' };
  const newLoad = loader.load(next);
  await flush();
  assert.equal(requests[0].signal.aborted, true);
  requests[1].request.resolve({ list: [device({ uuid: 'gpu-b', nodeUid: next.uid })] });
  await newLoad;
  requests[0].request.resolve({ list: [device()] });
  await oldLoad;
  assert.deepEqual(states.at(-1).entries.map((entry) => entry.device.uuid), ['gpu-b']);
  loader.dispose();
});

test('container failure retains devices, inventory failure is retryable, disposal cancels pending work', async () => {
  let failDevices = false;
  const states = [];
  const loader = createNodeDeviceAllocationLoader({
    getDevices: async () => { if (failDevices) throw new Error('inventory unavailable'); return { list: [device()] }; },
    getContainers: async () => { throw new Error('holders unavailable'); },
    onChange: (state) => states.push(state),
  });
  await loader.load(node);
  assert.equal(states.at(-1).status, 'ready');
  assert.equal(states.at(-1).entries[0].allocationStatus, 'unavailable');
  failDevices = true;
  await loader.load(node);
  assert.deepEqual(states.at(-1), { entries: [], status: 'error' });
  failDevices = false;
  await loader.load(node);
  assert.equal(states.at(-1).status, 'ready');
  loader.dispose();

  const pending = deferred();
  let signal;
  const disposedStates = [];
  const another = createNodeDeviceAllocationLoader({
    getDevices: (_, requestSignal) => { signal = requestSignal; return pending.promise; },
    getContainers: async () => ({ items: [] }),
    onChange: (state) => disposedStates.push(state),
  });
  const loading = another.load(node);
  await flush();
  another.dispose();
  assert.equal(signal.aborted, true);
  pending.resolve({ list: [device()] });
  await loading;
  assert.deepEqual(disposedStates.map((state) => state.status), ['loading']);
});

test('invalid sources issue no requests and malformed responses are not interpreted as empty data', async () => {
  let requests = 0;
  const states = [];
  const loader = createNodeDeviceAllocationLoader({
    getDevices: async () => { requests += 1; return {}; },
    getContainers: async () => { requests += 1; return {}; },
    onChange: (state) => states.push(state),
  });
  await loader.load(null);
  assert.equal(requests, 0);
  assert.equal(states.at(-1).status, 'idle');
  await loader.load(node);
  assert.equal(states.at(-1).status, 'error');
  loader.dispose();
});

test('wrong-node, duplicate or invalid inventory cannot masquerade as an empty node', async () => {
  for (const list of [
    [device({ nodeUid: 'previous-node-lifecycle' })],
    [device(), device({ uuid: 'gpu-old', nodeUid: 'old-node' })],
    [device(), device()],
    [device({ uuid: '' })],
    [null],
  ]) {
    let state;
    const loader = createNodeDeviceAllocationLoader({
      getDevices: async () => ({ list }),
      getContainers: async () => ({ items: [] }),
      onChange: (value) => { state = value; },
    });
    await loader.load(node);
    assert.deepEqual(state, { entries: [], status: 'error' });
    loader.dispose();
  }
});
