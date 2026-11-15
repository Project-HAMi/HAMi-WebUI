import assert from 'node:assert/strict';
import test from 'node:test';
import { attachLegacyDeviceLinks } from './device-detail-links.mjs';

const source = { node: 'worker', nodeUid: 'node-a', id: 'GPU-1', provider: 'NVIDIA' };
const target = { nodeName: 'worker', nodeUid: 'node-a', uuid: 'GPU-1', vendor: 'NVIDIA' };
const route = uuid => ({ name: 'accelerator-detail', params: { uuid } });

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

test('opens the globally unique legacy ID for this node and provider', () => {
  assert.deepEqual(links(source, [target]), route('GPU-1'));
  for (const extra of [{ ...target, nodeName: 'other' }, { ...target, vendor: 'DCU' }, target]) {
    assert.equal(links(source, [extra, target]), null);
  }
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
