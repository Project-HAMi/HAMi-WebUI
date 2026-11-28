export const getAllocationPercent = (used, total) => {
  const usedValue = Number(used ?? 0);
  const totalValue = Number(total);

  if (
    !Number.isFinite(usedValue) ||
    !Number.isFinite(totalValue) ||
    totalValue <= 0
  ) {
    return undefined;
  }

  const raw = Math.max(0, (usedValue / totalValue) * 100);
  return {
    raw,
    progress: Math.min(100, raw),
  };
};

export const computeAllocationPercent = card =>
  card.isExternal || card.unconfigured || card.coreUsedKnown === false
    ? undefined : getAllocationPercent(card.coreUsed, card.coreTotal);

export const memoryAllocationPercent = card =>
  card.isExternal || card.unconfigured
    ? undefined : getAllocationPercent(card.memoryUsed, card.memoryTotal);
