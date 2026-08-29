# Serve and embed HAMi-WebUI under a URL prefix

HAMi-WebUI can be served below a URL prefix such as `/hami/` and embedded as a
whole application in an internal platform. The prefix and framing policy are
runtime settings: the frontend image does not need to be rebuilt.

HAMi-WebUI remains a single-cluster, read-only UI. It does not provide login,
RBAC, or a multi-cluster gateway. Put an authenticated Ingress or identity-aware
proxy in front of deployments that are not already on a trusted network.

## Configure the public path and parent origins

Configure the same prefix in the Web entry and Ingress. The proxy must preserve
the prefix when forwarding the request.

```yaml
frontend:
  basePath: /hami/
  frameAncestors:
    - "'self'"
    - https://portal.example.com

ingress:
  enabled: true
  hosts:
    - host: hami.example.com
      paths:
        - path: /hami
          pathType: Prefix
```

The official Go frontend accepts `/hami/`, its SPA deep links, static assets,
and `/hami/api/vgpu/v1/*`. The unprefixed `/health_check` endpoint remains
available for Kubernetes probes. HAMi-WebUI deliberately does not trust
`X-Forwarded-Prefix`; configure a non-stripping proxy instead.

The embedding platform can then use a normal iframe:

```html
<iframe
  src="https://hami.example.com/hami/admin/vgpu/monitor/overview"
  title="HAMi GPU resources"
  style="width: 100%; min-height: 800px; border: 0"
></iframe>
```

## Choose a framing policy

`frontend.frameAncestors` maps to the HTTP Content Security Policy
`frame-ancestors` directive:

| Value | Behavior |
| --- | --- |
| `null` (default) | Send no application framing policy. This preserves Chart 1.x compatibility but allows any parent permitted by the surrounding proxy. |
| `[]` | Block every iframe parent with `frame-ancestors 'none'`. Top-level access still works. |
| `["'self'"]` | Allow only a parent with the same origin. In YAML, keep both layers of quoting as shown. |
| Exact HTTP(S) origins | Allow only the listed origins, including any explicit port. |

To keep the policy explicit and auditable, HAMi-WebUI rejects wildcards,
scheme-only sources, paths, queries, fragments, and user information. Invalid
values make the Web entry fail at startup instead of silently weakening the
policy.

Configure framing in one place. If an Ingress also emits
`Content-Security-Policy` or `X-Frame-Options`, browser enforcement can become
more restrictive than this value. The CSP specification applies every policy
rather than choosing the last one.

For nested iframes, every ancestor origin must appear in `frameAncestors`, not
only the immediate parent. The embedding platform's own CSP must also permit
the HAMi-WebUI origin through `frame-src` or its applicable fallback directive.

## Authentication and cookies

The iframe loads the WebUI and its API from the same origin, so CORS is not
required. Authentication remains the responsibility of the proxy in front of
that origin.

Prefer a same-site deployment when the proxy uses cookies. Cross-site iframes
may require `Secure` and `SameSite=None`, and browsers or enterprise policy can
still block third-party cookies. Verify the actual login and refresh flow in the
target browser; do not treat forwarded identity headers as authorization inside
HAMi-WebUI.

## Upgrade and rollback boundary

Runtime base paths and application-managed framing require an official Go
frontend image from a release that documents these values. Released Chart
versions through v1.3.0 used the previous Node image; that image and arbitrary
custom frontend images may ignore these environment variables and are supported
only at `/` unless they implement the same contract.

If a deployment relies on the application framing policy, configure an
equivalent policy at the authenticated proxy before rolling back to an older
frontend image.

For the underlying browser behavior, see the W3C
[Content Security Policy specification](https://www.w3.org/TR/CSP3/#directive-frame-ancestors).
