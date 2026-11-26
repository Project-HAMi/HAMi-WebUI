const STORAGE_PREFIX = 'hami-webui:';

const resolveLocalStorage = (storageOverride) => {
  if (storageOverride !== undefined) {
    return storageOverride;
  }
  try {
    return typeof window !== 'undefined' ? window.localStorage : undefined;
  } catch {
    return undefined;
  }
};

export const createSafeStorage = (storageOverride) => {
  const memoryStore = new Map();
  const dirtyKeys = new Set();
  const prefixKey = (key) => `${STORAGE_PREFIX}${key}`;

  const getItem = (key) => {
    const namespaced = prefixKey(key);
    if (dirtyKeys.has(namespaced)) {
      return memoryStore.has(namespaced) ? memoryStore.get(namespaced) : null;
    }
    const target = resolveLocalStorage(storageOverride);
    if (target && typeof target.getItem === 'function') {
      try {
        return target.getItem(namespaced);
      } catch {
        // Cross-origin iframes and private contexts can throw SecurityError.
      }
    }
    return memoryStore.has(namespaced) ? memoryStore.get(namespaced) : null;
  };

  const setItem = (key, value) => {
    const namespaced = prefixKey(key);
    const serialized = String(value);
    memoryStore.set(namespaced, serialized);
    const target = resolveLocalStorage(storageOverride);
    if (target && typeof target.setItem === 'function') {
      try {
        target.setItem(namespaced, serialized);
        dirtyKeys.delete(namespaced);
        return true;
      } catch {
        dirtyKeys.add(namespaced);
        return false;
      }
    }
    dirtyKeys.add(namespaced);
    return false;
  };

  const removeItem = (key) => {
    const namespaced = prefixKey(key);
    memoryStore.delete(namespaced);
    const target = resolveLocalStorage(storageOverride);
    if (target && typeof target.removeItem === 'function') {
      try {
        target.removeItem(namespaced);
        dirtyKeys.delete(namespaced);
        return;
      } catch {
        // Keep in-memory state consistent even if host storage fails.
      }
    }
    dirtyKeys.add(namespaced);
  };

  const getJson = (key, fallback = null) => {
    const raw = getItem(key);
    if (raw === null || raw === '') {
      return fallback;
    }
    try {
      return JSON.parse(raw);
    } catch {
      return fallback;
    }
  };

  const setJson = (key, value) => {
    try {
      return setItem(key, JSON.stringify(value));
    } catch {
      return false;
    }
  };

  return {
    getItem,
    setItem,
    removeItem,
    getJson,
    setJson,
  };
};

export const safeStorage = createSafeStorage();
