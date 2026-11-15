import { matchDeviceRegistration, registrationDetail } from '../../../api/device-detail-link.mjs';

export function attachLegacyDeviceLinks(insights, legacyRows) {
  const match = matchDeviceRegistration(legacyRows);
  return {
    devices: insights.devices.map(d => ({ ...d, detailTo: registrationDetail(match(d, d.id)) })),
    issues: insights.issues.map(i => i.kind === 'deviceUnhealthy'
      ? { ...i, detailTo: registrationDetail(match(i, i.deviceId)) } : i),
  };
}
