import { Request } from 'express'

/**
 * URL sub-path (base path) support for the BFF.
 *
 * The WebUI can be served either at the site root (`/`, the default and
 * historical behaviour) or under an arbitrary reverse-proxy prefix such as
 * `/gpu-ui/`. The prefix is resolved at request time from two sources, in
 * order of precedence:
 *
 *   1. the `X-Forwarded-Prefix` request header — set by a path-stripping
 *      reverse proxy, so a deployment behind such a proxy "just works";
 *   2. the `HAMI_WEBUI_BASE_PATH` environment variable — the deploy-time
 *      default, used when the proxy passes the full prefixed path through.
 *
 * Modelled on Grafana's `serve_from_sub_path` and ArgoCD's `server.rootpath`.
 */

/**
 * Normalize a raw base path into a canonical `/segment/.../` form with a
 * leading and trailing slash. Returns `/` for empty / root inputs.
 *
 * The value ends up in an HTML `<base href>` attribute and a `window.__BASE_PATH__`
 * assignment, and is also compared against request URLs, so we strip anything
 * outside a conservative URL-path character set to avoid HTML/JS injection.
 */
export function normalizeBasePath(raw?: string | string[]): string {
  if (!raw) {
    return '/'
  }
  let p = Array.isArray(raw) ? raw[0] : raw
  p = String(p).trim()
  // Keep only safe URL-path characters (drops quotes, angle brackets, spaces…).
  p = p.replace(/[^A-Za-z0-9\-._~/%]/g, '')
  // Collapse any run of slashes into a single one.
  p = p.replace(/\/{2,}/g, '/')
  if (p === '' || p === '/') {
    return '/'
  }
  if (!p.startsWith('/')) {
    p = '/' + p
  }
  if (!p.endsWith('/')) {
    p = p + '/'
  }
  return p
}

/**
 * The deploy-time default base path, read once from the environment.
 * `/` (root) is the default and preserves the historical behaviour exactly.
 */
export const ENV_BASE_PATH = normalizeBasePath(process.env.HAMI_WEBUI_BASE_PATH)

/**
 * Resolve the effective base path for a single request. The
 * `X-Forwarded-Prefix` header (set by a path-stripping proxy) takes precedence
 * over the environment default when present.
 */
export function resolveBasePath(req: Request): string {
  const header = req.headers['x-forwarded-prefix']
  if (header) {
    return normalizeBasePath(header)
  }
  return ENV_BASE_PATH
}

/**
 * Inject the resolved base path into an index.html string at request time:
 *  - a `<base href="{basePath}">` tag so all relative asset/API URLs (the build
 *    uses a relative `./` base) resolve under the prefix;
 *  - a `window.__BASE_PATH__` global the SPA reads for its router base, axios
 *    baseURL and socket.io path.
 * Any pre-existing `<base>` tag is removed first so nothing hard-codes root
 * serving. `basePath` is expected to already be normalized (and thus safe to
 * interpolate — see normalizeBasePath).
 */
export function injectBasePath(html: string, basePath: string): string {
  const inject =
    `<base href="${basePath}">` +
    `<script>window.__BASE_PATH__=${JSON.stringify(basePath)}</script>`
  return html
    .replace(/<base\b[^>]*>/i, '')
    .replace(/<head\b[^>]*>/i, (head) => `${head}${inject}`)
}
