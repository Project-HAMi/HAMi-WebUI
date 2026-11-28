const first = (value) => (Array.isArray(value) ? value[0] : value);

// Missing or empty query values become undefined.
export const readQuery = (query, keys) => Object.fromEntries(keys.map((key) => {
  const value = first(query?.[key]);
  return [key, typeof value === 'string' && value !== '' ? value : undefined];
}));

// Empty values are removed; keys that are not listed stay as they are.
export const writeQuery = (query, values) => {
  const next = { ...query };
  Object.entries(values).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') delete next[key];
    else next[key] = String(value);
  });
  return next;
};

export const sameQuery = (left = {}, right = {}) => [...new Set([...Object.keys(left), ...Object.keys(right)])]
  .every((key) => String(first(left[key]) ?? '') === String(first(right[key]) ?? ''));
