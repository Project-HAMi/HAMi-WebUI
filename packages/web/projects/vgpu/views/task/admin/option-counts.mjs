// Counts arrive as JSON objects; older backends omit them.
export const readOptionCounts = (value) => (
  value && typeof value === 'object' && !Array.isArray(value) && Object.values(value).every(Number.isInteger) ? value : null
);

// Only own keys count, so a node named "constructor" does not read Object's prototype.
export const optionCountOf = (counts, key) => (counts && Object.hasOwn(counts, key) ? counts[key] : 0);
