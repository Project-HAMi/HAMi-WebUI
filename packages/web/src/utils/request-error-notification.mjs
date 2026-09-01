const DEFAULT_WINDOW_MS = 5000;
const DEFAULT_MAX_ENTRIES = 64;

const normalize = (value) => (value === undefined || value === null ? '' : String(value));

const requestPath = (config) => {
  const url = normalize(config?.url);
  if (!url) return '';

  try {
    return new URL(url, 'http://request.local').pathname;
  } catch {
    return url.split('?')[0];
  }
};

export const requestErrorFingerprint = ({
  kind,
  config,
  status,
  code,
  reason,
  message,
}) =>
  JSON.stringify(
    [
      kind,
      normalize(config?.method).toUpperCase(),
      requestPath(config),
      status,
      code,
      reason,
      message,
    ].map(normalize),
  );

export const createRequestErrorNotificationGate = ({
  now = Date.now,
  windowMs = DEFAULT_WINDOW_MS,
  maxEntries = DEFAULT_MAX_ENTRIES,
} = {}) => {
  const firstSeen = new Map();
  const entryLimit = Math.max(1, maxEntries);

  return (details) => {
    const currentTime = now();
    const fingerprint = requestErrorFingerprint(details);
    const previousTime = firstSeen.get(fingerprint);

    if (previousTime !== undefined && currentTime - previousTime < windowMs) {
      // Keep a fixed window from the first visible notification. Updating this
      // timestamp would let a continuous failure suppress itself forever.
      return false;
    }

    for (const [key, timestamp] of firstSeen) {
      if (currentTime - timestamp >= windowMs) firstSeen.delete(key);
    }
    while (firstSeen.size >= entryLimit) {
      firstSeen.delete(firstSeen.keys().next().value);
    }

    firstSeen.set(fingerprint, currentTime);
    return true;
  };
};
