const indexRow = (map, key, row) => {
  if (!key) return;
  map.set(key, map.has(key) ? null : row);
};

// Devices are scoped by nodeUid so repeated device IDs across distinct nodes stay linkable,
// while legacy DCU registrations also index under registrationId when rewritten to a physical serial.
export function matchDeviceRegistration(legacyRows) {
  const byNodeAndId = new Map();
  for (const row of legacyRows) {
    if (!row?.nodeUid || !row?.uuid) continue;
    indexRow(byNodeAndId, `${row.nodeUid}\0${row.uuid}`, row);
    if (row.registrationId && row.registrationId !== row.uuid) {
      indexRow(byNodeAndId, `${row.nodeUid}\0${row.registrationId}`, row);
    }
  }

  return function match(device, registrationId) {
    if (!registrationId || !device.nodeUid || !device.node || !device.provider) return null;
    let uuid = registrationId;
    if (device.provider === 'HCU') {
      // hcu.FetchDevices removes exactly one HCU- prefix from the serial.
      if (!registrationId.startsWith('HCU-')) return null;
      uuid = registrationId.slice(4);
    }
    const target = byNodeAndId.get(`${device.nodeUid}\0${uuid}`);
    if (!target || target.nodeName !== device.node || target.vendor !== device.provider) return null;
    if (target.nodeUid !== device.nodeUid) return null;
    return target;
  };
}

export function registrationDetail(row) {
  if (!row) return null;
  return {
    name: 'accelerator-detail',
    params: { uuid: row.uuid },
    ...(row.nodeUid ? { query: { nodeUid: row.nodeUid } } : {}),
  };
}
