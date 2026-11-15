import { buildNodeDetailLocation } from '../../node/detail-location.mjs';

export const isCount = value => Number.isSafeInteger(value) && value >= 0;

export function summarizeNodeIssues(issues) {
  const byNode = new Map();
  const deviceIds = new Map();
  for (const issue of issues) {
    if (!issue?.node || !['deviceUnhealthy', 'nodeNotReady', 'nodeCordoned'].includes(issue.kind)) continue;
    let row = byNode.get(issue.node);
    if (!row) {
      row = { node: issue.node, to: null, devices: [], affectedDevices: 0, notReady: null, cordoned: null };
      byNode.set(issue.node, row);
      deviceIds.set(issue.node, new Set());
    }
    row.to ||= buildNodeDetailLocation({ uid: issue.nodeUid, nodeName: issue.node });
    if (issue.kind === 'deviceUnhealthy') {
      if (issue.deviceId) {
        const id = JSON.stringify([issue.provider, issue.deviceId]);
        const seen = deviceIds.get(issue.node);
        if (seen.has(id)) continue;
        seen.add(id);
      }
      row.devices.push(issue);
      row.affectedDevices += !issue.deviceId && isCount(issue.deviceCount) ? issue.deviceCount : 1;
    } else if (issue.kind === 'nodeNotReady') row.notReady = issue;
    else row.cordoned = issue;
  }

  const incidents = [];
  const cordonedNodes = [];
  let count = 0;
  for (const row of byNode.values()) {
    if (row.devices.length || row.notReady) {
      incidents.push(row);
      count += row.affectedDevices + Number(Boolean(row.notReady));
    } else {
      cordonedNodes.push(row);
    }
  }
  incidents.sort((a, b) => b.affectedDevices - a.affectedDevices || Number(Boolean(b.notReady)) - Number(Boolean(a.notReady)) || a.node.localeCompare(b.node));
  cordonedNodes.sort((a, b) => a.node.localeCompare(b.node));
  return { incidents, cordonedNodes, count };
}

export function visibleDeviceGroups(devices, limit) {
  const groups = new Map();
  for (const device of devices.slice(0, limit)) {
    const model = device.model || '';
    if (!groups.has(model)) groups.set(model, { model, devices: [], reportedWithoutId: 0 });
    const group = groups.get(model);
    if (device.deviceId) group.devices.push(device);
    else if (isCount(device.deviceCount)) group.reportedWithoutId += device.deviceCount;
  }
  return [...groups.values()];
}
