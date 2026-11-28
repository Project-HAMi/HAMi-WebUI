import { NavigationFailureType } from 'vue-router';

export const CHUNK_RELOAD_STORAGE_KEY = 'hami-webui:chunk-reload';

const CHUNK_ERROR_PATTERNS = [
  /Failed to fetch dynamically imported module/i,
  /Importing a module script failed/i,
  /error loading dynamically imported module/i,
  /Unable to preload CSS/i,
  /Failed to load module script/i,
];

/**
 * Return true when an error indicates a failed lazy route or asset chunk load.
 */
export function isChunkLoadError(error) {
  if (!error) return false;
  if (error.name === 'ChunkLoadError') return true;
  const message =
    typeof error === 'string' ? error : String(error.message || '');
  return CHUNK_ERROR_PATTERNS.some((pattern) => pattern.test(message));
}

function readStorageAttempt(win) {
  try {
    const storage = win.sessionStorage;
    if (!storage) return { ok: false, value: '' };
    return { ok: true, value: storage.getItem(CHUNK_RELOAD_STORAGE_KEY) || '' };
  } catch {
    return { ok: false, value: '' };
  }
}

function writeStorageAttempt(win, href) {
  try {
    const storage = win.sessionStorage;
    if (!storage) return false;
    storage.setItem(CHUNK_RELOAD_STORAGE_KEY, href);
    return true;
  } catch {
    return false;
  }
}

function clearStorageAttempt(win) {
  try {
    win.sessionStorage?.removeItem(CHUNK_RELOAD_STORAGE_KEY);
  } catch {}
}

/**
 * Recover once from stale hashed chunk 404s during rolling upgrades without
 * entering an infinite reload loop or bypassing blocked-storage sandboxes.
 */
export function installChunkRecovery(router, options = {}) {
  const win = options.win ?? globalThis.window;
  const navigate = options.navigate ?? ((href) => win.location.assign(href));
  if (!router || !win?.location) return () => {};

  let pendingTargetHref = '';
  let reloading = false;

  const resolveTargetHref = (to) => {
    const fallback = pendingTargetHref || win.location.href;
    if (!to?.fullPath || typeof router.resolve !== 'function') {
      return fallback;
    }
    try {
      const resolved = router.resolve(to);
      if (!resolved?.href) return fallback;
      const parsed = new URL(resolved.href, win.location.href);
      if (parsed.origin !== win.location.origin) return win.location.href;
      return parsed.href;
    } catch {
      return fallback;
    }
  };

  const attemptReload = (targetHref) => {
    if (reloading) return false;
    const { ok, value } = readStorageAttempt(win);
    // Require persistent storage confirmation so blocked iframes cannot loop.
    if (!ok || value === targetHref) return false;
    if (!writeStorageAttempt(win, targetHref)) return false;
    reloading = true;
    navigate(targetHref);
    return true;
  };

  const stopBeforeEach = router.beforeEach((to) => {
    pendingTargetHref = resolveTargetHref(to);
  });

  const stopAfterEach = router.afterEach((to, _from, failure) => {
    if (reloading) return;
    if (!failure) {
      pendingTargetHref = '';
      clearStorageAttempt(win);
      return;
    }
    if (
      failure.type &&
      ((failure.type & NavigationFailureType.duplicated) !== 0 ||
        ((failure.type & NavigationFailureType.aborted) !== 0 &&
          resolveTargetHref(to) === pendingTargetHref))
    ) {
      pendingTargetHref = '';
    }
  });

  const stopOnError = router.onError((error, to) => {
    if (!isChunkLoadError(error)) return;
    const targetHref = resolveTargetHref(to);
    if (!pendingTargetHref || targetHref !== pendingTargetHref) return;
    attemptReload(targetHref);
  });

  return () => {
    stopBeforeEach?.();
    stopAfterEach?.();
    stopOnError?.();
  };
}
