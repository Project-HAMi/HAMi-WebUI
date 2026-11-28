const NUMBER = new Intl.NumberFormat('en-US', { maximumFractionDigits: 1 });

const formatAmount = (value) => {
  if (value > 0 && value < 0.1) return '<0.1';
  return NUMBER.format(value);
};

// Both sides share the unit the total picks, so the pair reads as one quantity.
export const formatAmountPair = (used, total, unit = '') => {
  const scale = unit === 'GiB' && Math.abs(total) >= 1024 ? 1024 : 1;
  return {
    used: formatAmount(used / scale),
    total: formatAmount(total / scale),
    unit: scale === 1024 ? 'TiB' : unit,
  };
};
