import { buildDeviceSplit } from '../../../components/device-split.mjs';

export const readAllocationNumber = (value) => {
  if (typeof value !== 'number' && typeof value !== 'string') return undefined;
  if (typeof value === 'string' && !value.trim()) return undefined;
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : undefined;
};

const identity = (value) => typeof value === 'string' && value.trim() !== '';
const countOf = (value) => {
  const count = readAllocationNumber(value);
  return Number.isSafeInteger(count) ? count : undefined;
};
const supportedModes = new Set(['hami-core', 'mig', 'template']);
const modeShapes = {
  'hami-core': new Set(['soft', 'whole']),
  mig: new Set(['mig']),
  template: new Set(['template', 'whole']),
};
const validInventory = (devices, node) => {
  if (!Array.isArray(devices)) return false;
  const seen = new Set();
  return devices.every((device) => {
    if (!device || !identity(device.uuid) || device.nodeUid !== node.uid || seen.has(device.uuid)) return false;
    seen.add(device.uuid);
    return true;
  });
};
const validPlacement = (allocation) => {
  const start = countOf(allocation.migStart);
  const size = countOf(allocation.migSize);
  return start !== undefined && size > 0 && start + size <= 32;
};
const registeredMigLayout = (device) => Array.isArray(device.migProfiles)
  && device.migProfiles.length > 0
  && device.migProfiles.every((profile) => identity(profile?.name)
    && Array.isArray(profile.placements) && profile.placements.length > 0
    && profile.placements.every((placement) => validPlacement({
      migStart: placement?.start, migSize: placement?.size,
    })));

const completeAllocation = (device, records) => {
  if (!supportedModes.has(device.mode)
    || countOf(device.vgpuUsed) !== records.length
    || !(readAllocationNumber(device.memoryTotal) > 0)
    || !(readAllocationNumber(device.coreTotal) > 0)) return false;
  if (device.mode === 'mig' && !registeredMigLayout(device)) return false;

  let memory = 0;
  let cores = 0;
  for (const record of records) {
    const allocatedMemory = readAllocationNumber(record.allocatedMem);
    const allocatedCores = readAllocationNumber(record.allocatedCores);
    if (allocatedMemory === undefined || allocatedCores === undefined
      || record.allocatedCoresKnown !== true
      || !modeShapes[device.mode].has(record.allocationShape)) return false;
    if (record.allocationShape === 'mig' && !validPlacement(record)) return false;
    memory += allocatedMemory;
    cores += allocatedCores;
  }
  return device.coreUsedKnown === true
    && readAllocationNumber(device.memoryUsed) === memory
    && readAllocationNumber(device.coreUsed) === cores;
};

export const buildNodeDeviceAllocations = ({ node, devices = [], containers = [], containersAvailable = true }) => {
  const byDevice = new Map();
  let incompleteIdentity = false;
  for (const container of containers) {
    if (!container || container.nodeUid !== node.uid) continue;
    if (!identity(container.podUid) || !identity(container.name) || !Array.isArray(container.devices)) {
      incompleteIdentity = true;
      continue;
    }
    if (container.devices.some((device) => !identity(device?.id))) incompleteIdentity = true;
    const ids = new Set(container.devices.map((device) => device?.id).filter(identity));
    for (const id of ids) {
      if (!byDevice.has(id)) byDevice.set(id, []);
      byDevice.get(id).push(container);
    }
  }

  return devices
    .filter((device) => device && identity(device.uuid) && device.nodeUid === node.uid)
    .sort((left, right) => left.uuid.localeCompare(right.uuid))
    .map((device) => {
      const deviceContainers = byDevice.get(device.uuid) || [];
      const records = deviceContainers.flatMap((container) => container.devices.filter((record) => record?.id === device.uuid));
      const containerCount = containersAvailable
        ? new Set(deviceContainers.map((container) => `${container.podUid}/${container.name}`)).size
        : undefined;
      let allocationStatus = 'partial';
      if (device.unconfigured) allocationStatus = 'unconfigured';
      else if (!containersAvailable) allocationStatus = 'unavailable';
      else if (!incompleteIdentity && completeAllocation(device, records)) allocationStatus = 'complete';

      let split = allocationStatus === 'complete'
        ? buildDeviceSplit({ device, containers: deviceContainers })
        : undefined;
      if (split?.overlapping || split?.beyondRegistered) {
        allocationStatus = 'partial';
        split = undefined;
      }
      return { device, containers: deviceContainers, allocationStatus, containerCount, split };
    });
};

// Inventory and holders are separate snapshots. A missing reservation must not
// turn into a free slice when the node-filtered holder list lags the inventory.
export const createNodeDeviceAllocationLoader = ({ getDevices, getContainers, onChange }) => {
  let controller;
  let generation = 0;
  let disposed = false;

  const cancel = () => {
    generation += 1;
    controller?.abort();
  };
  const load = async (source) => {
    cancel();
    if (disposed) return;
    const current = generation;
    if (!source || !identity(source.uid) || !identity(source.name)) {
      onChange({ entries: [], status: 'idle' });
      return;
    }
    const node = { uid: source.uid, name: source.name };
    const requestController = new AbortController();
    controller = requestController;
    onChange({ entries: [], status: 'loading' });
    const [devicesResult, containersResult] = await Promise.allSettled([
      Promise.resolve().then(() => getDevices(node.name, requestController.signal)),
      Promise.resolve().then(() => getContainers(node.uid, requestController.signal)),
    ]);
    if (disposed || current !== generation) return;
    if (devicesResult.status !== 'fulfilled' || !validInventory(devicesResult.value?.list, node)) {
      onChange({ entries: [], status: 'error' });
      return;
    }
    const containersAvailable = containersResult.status === 'fulfilled' && Array.isArray(containersResult.value?.items);
    onChange({
      entries: buildNodeDeviceAllocations({
        node,
        devices: devicesResult.value.list,
        containers: containersAvailable ? containersResult.value.items : [],
        containersAvailable,
      }),
      status: 'ready',
    });
  };
  return {
    load,
    dispose: () => { disposed = true; cancel(); },
  };
};
