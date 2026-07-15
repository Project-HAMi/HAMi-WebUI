# Serving HAMi-WebUI under a URL sub-path

By default HAMi-WebUI is served at the site root (`https://host/`). It can also
be served under an arbitrary reverse-proxy prefix (a "sub-path" / "base path"),
e.g. `https://host/gpu-ui/`, so several tools can share one hostname.

The base path is resolved **at request time**, not baked into the frontend
bundle — so a single `*-fe-oss` image works at any path and changing the path
never requires an image rebuild. The design mirrors Grafana's
`serve_from_sub_path` and ArgoCD's `server.rootpath`.

The Go API backend (`*-be-oss`) is unaffected — it is always reached through the
BFF's `/api/vgpu` proxy and is path-agnostic.

## How the base path is resolved

The frontend BFF (the NestJS `*-fe-oss` container) resolves the prefix from two
sources, in order of precedence:

1. **`X-Forwarded-Prefix` request header** — set by a path-stripping reverse
   proxy. Takes precedence when present.
2. **`HAMI_WEBUI_BASE_PATH` environment variable** — the deploy-time default.
   Defaults to `/` (root serving, the historical behaviour).

Values are normalized to a leading/trailing-slash form: `gpu-ui`, `/gpu-ui`, and
`/gpu-ui/` all become `/gpu-ui/`. `/` means root.

On every request for `index.html` the BFF injects the resolved value as:

```html
<base href="/gpu-ui/">
<script>window.__BASE_PATH__="/gpu-ui/"</script>
```

The SPA reads `window.__BASE_PATH__` for its router base, its axios `baseURL`,
and its socket.io `path`; the `<base>` tag makes the relative asset URLs
(`./static/…`, `./favicon.svg`) resolve under the prefix.

## Two supported proxy modes

### Mode A — path-stripping proxy (sets `X-Forwarded-Prefix`)

The proxy strips the prefix before forwarding to the BFF and advertises it via
the header. Nothing needs to be configured on the chart — leave `basePath` at
`/`.

nginx example:

```nginx
location /gpu-ui/ {
    proxy_pass http://hami-webui:3000/;          # trailing slash strips /gpu-ui/
    proxy_set_header X-Forwarded-Prefix /gpu-ui;
    proxy_set_header Host $host;
    # socket.io upgrade (optional, for real-time views)
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

### Mode B — proxy forwards the full prefixed path (no stripping)

The BFF receives `/gpu-ui/...` and must know its own prefix, so set the chart
value (or the env var directly):

```yaml
# values.yaml
basePath: "/gpu-ui/"
ingress:
  enabled: true
  hosts:
    - host: your-host
      paths:
        - path: /gpu-ui
          pathType: Prefix
```

The BFF strips the configured prefix from incoming URLs up-front, so static
assets, the `/api/vgpu` proxy and the SPA deep-link fallback all keep working.

## Helm value

| Value      | Default | Description                                                                 |
|------------|---------|-----------------------------------------------------------------------------|
| `basePath` | `"/"`   | Sub-path to serve under. Injected as `HAMI_WEBUI_BASE_PATH`. `"/"` = root.   |

---

## Manual test plan

Prerequisites: a running cluster (or local `node dist/main`) with the built
frontend in `public/`. Replace `HOST` accordingly.

### 1. Root serving — no regression (default)

Deploy with defaults (`basePath: "/"`, no header).

| Check | Expect |
|-------|--------|
| `GET /` | 200; HTML contains `<base href="/">` and `window.__BASE_PATH__="/"` |
| Deep-link refresh `GET /admin/vgpu/monitor/overview` | 200; same injected `<base href="/">` |
| `GET /static/<asset>.js` | 200 |
| `GET /favicon.svg` | 200 |
| `GET /api/vgpu/v1/summary` (POST from UI) | proxied to the Go backend |
| `GET /health_check` | `{"code":0,...,"data":"OK"}` |
| Browser: open `/`, navigate the app, open a real-time view | assets load, REST works, socket.io connects on `/socket.io` |

### 2. Mode B — non-stripping proxy under `/gpu-ui/`

Deploy with `basePath: "/gpu-ui/"` and an ingress path of `/gpu-ui`.

| Check | Expect |
|-------|--------|
| `GET /gpu-ui/` and `GET /gpu-ui` | 200; `<base href="/gpu-ui/">`, `window.__BASE_PATH__="/gpu-ui/"` |
| Deep-link refresh `GET /gpu-ui/admin/vgpu/monitor/overview` | 200; injected base `/gpu-ui/` |
| `GET /gpu-ui/static/<asset>.js` | 200 |
| `GET /gpu-ui/favicon.svg` | 200 |
| UI REST calls | requested at `/gpu-ui/api/vgpu/...`, proxied to the backend |
| Real-time view | socket.io connects at `/gpu-ui/socket.io` |
| `GET /health_check` (unprefixed k8s probe) | still `...,"data":"OK"` |

### 3. Mode A — path-stripping proxy via `X-Forwarded-Prefix`

Deploy with defaults (`basePath: "/"`) behind a proxy that strips `/gpu-ui` and
sets `X-Forwarded-Prefix: /gpu-ui`.

| Check | Expect |
|-------|--------|
| `curl -H 'X-Forwarded-Prefix: /gpu-ui' https://HOST/` | `<base href="/gpu-ui/">`, `window.__BASE_PATH__="/gpu-ui/"` |
| Deep-link (proxy has already stripped path) `curl -H 'X-Forwarded-Prefix: /gpu-ui' https://HOST/admin/...` | injected base `/gpu-ui/` |
| Through the browser at `https://HOST/gpu-ui/` | page loads, assets/REST/socket.io all resolve under `/gpu-ui/` |

### 4. Security

| Check | Expect |
|-------|--------|
| `curl -H 'X-Forwarded-Prefix: /x"><script>alert(1)</script>' https://HOST/` | injected `<base>` is sanitized (no `"`, `<`, `>`); no script injection |

A scripted version of checks 1–3 (and 4) is exercised by the BFF unit tests in
`src/utils/base-path.spec.ts` and by running `node dist/main` with
`HAMI_WEBUI_BASE_PATH` / `X-Forwarded-Prefix` against a `public/` build.
