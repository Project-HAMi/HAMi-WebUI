import apiClient from '@/api/client.mjs';
import { physicalDeviceKey, readOverviewInventory } from './overview-inventory.mjs';
import { physicalModelDistribution } from './model-distribution.mjs';
import { matchDeviceRegistration, registrationDetail } from './device-detail-link.mjs';

export async function getPhysicalCardList(signal) {
  const [inventory, registrations] = await Promise.all([
    apiClient.getOverviewInventory({ signal }).then(readOverviewInventory),
    apiClient.listDevices({}, { signal }).catch(() => null),
  ]);
  if (!inventory.deviceCoverage.complete && !inventory.devices.length) {
    throw new Error('Physical inventory unavailable');
  }
  const byNode = new Map(inventory.nodes.map((node) => [node.name, node.uid]));
  const match = matchDeviceRegistration(Array.isArray(registrations?.list) ? registrations.list : []);
  const list = inventory.devices.map((device) => {
    const row = match({ ...device, nodeUid: byNode.get(device.node) }, device.deviceId);
    return {
      ...row,
      physicalKey: physicalDeviceKey(device),
      uuid: device.deviceId,
      nodeName: device.node,
      vendor: device.provider,
      type: device.model,
      health: device.healthy,
      utilizationRatio: device.utilizationRatio,
      memoryUsageRatio: device.memoryUsageRatio,
      detailTo: registrationDetail(row),
    };
  });
  return { list, models: physicalModelDistribution(inventory.devices, inventory.deviceCoverage) };
}

// The registration detail returns a boolean even when its health field was absent.
export async function getDetailHealth(card, signal) {
  const inventory = readOverviewInventory(await apiClient.getOverviewInventory({ signal }));
  const node = inventory.nodes.find((row) => row.name === card.nodeName);
  if (!node?.uid || node.uid !== card.nodeUid) return {};
  const match = matchDeviceRegistration([card]);
  const device = inventory.devices.find((row) =>
    row.node === node.name && match({ ...row, nodeUid: node.uid }, row.deviceId));
  if (device) return { health: device.healthy };
  // Ascend910C registers modules, so its node detail can show the module's own
  // registration. An unmatched physical ID provides no health evidence.
  const moduleRegistration = card.vendor === 'Ascend' && card.type === 'Ascend910C';
  return moduleRegistration ? card : {};
}
