const readSlotCount = (value) => {
  if (typeof value !== 'number' && typeof value !== 'string') return undefined;
  if (typeof value === 'string' && value.trim() === '') return undefined;
  const count = Number(value);
  return Number.isSafeInteger(count) && count >= 0 ? count : undefined;
};

export const getAllocationSlotDisplay = (device = {}) => {
  // Partition profiles are not equal-sized shares; only show a verified sharing quota.
  if (!device || !['NVIDIA', 'Ascend'].includes(device.vendor)
    || device.mode !== 'hami-core'
    || device.unconfigured) return undefined;

  const used = readSlotCount(device.vgpuUsed);
  const limit = readSlotCount(device.vgpuTotal);
  return {
    used,
    limit,
    percent: used !== undefined && limit > 0 ? (used / limit) * 100 : undefined,
  };
};
