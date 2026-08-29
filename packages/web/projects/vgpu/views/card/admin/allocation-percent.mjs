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
