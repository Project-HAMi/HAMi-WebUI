// Queries use absolute UTC instants; local formatting belongs to the UI only.
// Keep second precision so serialization does not shift the existing sample grid.
export const formatQueryTimestamp = (value) =>
  new Date(value).toISOString().replace(/\.\d{3}Z$/, 'Z');
