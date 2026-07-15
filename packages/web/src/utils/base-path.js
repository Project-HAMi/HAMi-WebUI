/**
 * Runtime URL sub-path (base path) helpers for the SPA.
 *
 * The BFF injects `window.__BASE_PATH__` into index.html at request time
 * (e.g. "/" for root serving, "/gpu-ui/" behind a reverse-proxy prefix).
 * Everything that builds an absolute-ish URL — the vue-router history base,
 * the axios baseURL, the socket.io path — derives it from here so a single
 * frontend build works under any prefix without a rebuild.
 *
 * In the Vite dev server (where the BFF does not serve index.html) the global
 * is undefined and we fall back to root, matching the dev proxy setup.
 */

/** Canonical base path with leading + trailing slash, e.g. "/" or "/gpu-ui/". */
export function getBasePath() {
  const runtime =
    typeof window !== 'undefined' ? window.__BASE_PATH__ : undefined;
  if (!runtime) {
    return '/';
  }
  let p = String(runtime).trim();
  if (!p.startsWith('/')) p = '/' + p;
  if (!p.endsWith('/')) p = p + '/';
  return p;
}
