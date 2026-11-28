import assert from 'node:assert/strict';
import test from 'node:test';
import { attachLegacyDeviceLinks } from './device-detail-links.mjs';

const source = { node: 'worker', nodeUid: 'node-a', id: 'GPU-1', provider: 'NVIDIA' };
const target = { nodeName: 'worker', nodeUid: 'node-a', uuid: 'GPU-1', vendor: 'NVIDIA' };
const route = (uuid, nodeUid = 'node-a') => ({
  name: 'accelerator-detail',
  params: { uuid },
  ...(nodeUid ? { query: { nodeUid } } : {}),
});

function links(device, inventory) {
  const nodeIssue = { kind: 'nodeNotReady', node: device.node };
  const { devices, issues } = attachLegacyDeviceLinks({
    devices: [device], issues: [nodeIssue, { ...device, deviceId: device.id, kind: 'deviceUnhealthy' }],
  }, inventory);
  assert.equal(issues[0], nodeIssue);
  assert.equal(devices[0].id, device.id);
  assert.deepEqual(devices[0].detailTo, issues[1].detailTo);
  return devices[0].detailTo;
}

test('opens matching device for this node and provider, even when ID repeats on another node', () => {
  assert.deepEqual(links(source, [target]), route('GPU-1'));
  // Same UUID on a distinct node must stay linkable and keep its own nodeUid.
  const otherNode = { node: 'other', nodeUid: 'node-b', id: 'GPU-1', provider: 'NVIDIA' };
  const otherTarget = { nodeName: 'other', nodeUid: 'node-b', uuid: 'GPU-1', vendor: 'NVIDIA' };
  assert.deepEqual(
    attachLegacyDeviceLinks({ devices: [source, otherNode], issues: [] }, [target, otherTarget]).devices.map((d) => d.detailTo),
    [route('GPU-1', 'node-a'), route('GPU-1', 'node-b')],
  );
  // Collisions on the same nodeUid stay unlinked.
  for (const extra of [{ ...target, nodeName: 'other' }, { ...target, vendor: 'DCU' }, target]) {
    assert.equal(links(source, [extra, target]), null);
  }
});

test('legacy DCU registration IDs join against physical serial rows on the same node', () => {
  const dcuOverview = { node: 'worker', nodeUid: 'node-a', id: 'DCU-3', provider: 'DCU' };
  const dcuLegacy = { nodeName: 'worker', nodeUid: 'node-a', uuid: 'legacy-serial-03', registrationId: 'DCU-3', vendor: 'DCU' };
  assert.deepEqual(links(dcuOverview, [dcuLegacy]), route('legacy-serial-03'));
  assert.equal(links({ ...dcuOverview, nodeUid: 'node-b' }, [dcuLegacy]), null);
});

test('missing, replaced, and mismatched targets stay readable without a link', () => {
  assert.equal(links({ ...source, nodeUid: '' }, [target]), null);
  for (const inventory of [[], [{ ...target, nodeName: 'other' }], [{ ...target, nodeUid: 'replacement' }],
    [{ ...target, nodeUid: '' }], [{ ...target, vendor: 'DCU' }], [{ ...target, uuid: '' }]]) {
    assert.equal(links(source, inventory), null);
  }
});

test('HCU links to its verified serial without changing the registration ID on screen', () => {
  const hcu = { ...source, provider: 'HCU', id: 'HCU-HCU-serial-with-hyphens' };
  const legacy = { ...target, vendor: 'HCU', uuid: 'HCU-serial-with-hyphens' };
  assert.deepEqual(links(hcu, [legacy]), route(legacy.uuid));
  assert.equal(links(hcu, [legacy, { ...target, uuid: legacy.uuid }]), null);
  assert.equal(links({ ...hcu, id: 'serial-without-prefix' }, [legacy]), null);
  assert.equal(links({ ...hcu, provider: 'DCU' }, [legacy]), null);
});
