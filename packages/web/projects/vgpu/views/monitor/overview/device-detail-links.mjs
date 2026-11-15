// The legacy detail endpoint accepts only the device ID, even when two nodes
// use the same one. A node-scoped match alone cannot disambiguate that route.
function legacyDeviceDetail(legacyRows) {
  const byId = new Map();
  for (const row of legacyRows) {
    if (row?.uuid) byId.set(row.uuid, byId.has(row.uuid) ? null : row);
  }

  return function detailFor(device, registrationId) {
    if (!registrationId || !device.node || !device.provider) return null;
    let uuid = registrationId;
    if (device.provider === 'HCU') {
      // hcu.FetchDevices removes exactly one HCU- prefix from the serial.
      if (!registrationId.startsWith('HCU-')) return null;
      uuid = registrationId.slice(4);
    }
    const target = byId.get(uuid);
    if (!target || target.nodeName !== device.node || target.vendor !== device.provider) return null;
    if (!device.nodeUid || target.nodeUid !== device.nodeUid) return null;
    return { name: 'accelerator-detail', params: { uuid } };
  };
}

export function attachLegacyDeviceLinks(insights, legacyRows) {
  const detailFor = legacyDeviceDetail(legacyRows);
  return {
    devices: insights.devices.map(d => ({ ...d, detailTo: detailFor(d, d.id) })),
    issues: insights.issues.map(i => i.kind === 'deviceUnhealthy'
      ? { ...i, detailTo: detailFor(i, i.deviceId) } : i),
  };
}
