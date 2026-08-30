---
title: HAMi-WebUI Single-Cluster Web Entry and Embedding Architecture
authors:
- "@Nimbus318"
approvers: []
creation-date: 2026-08-30
status: accepted
---

# HAMi-WebUI Single-Cluster Web Entry and Embedding Architecture

## Summary

HAMi-WebUI will remain a single-cluster, read-only observability UI. It must also
be easy for platform teams to embed in an existing portal without rebuilding the
frontend for each deployment.

The same-origin Web entry remains part of the product contract, but it does not
require a pass-through NestJS process. After v1.3.0, the unreleased Chart 1.x
main branch replaced the production NestJS runtime with a minimal static-file
and reverse-proxy Gateway while preserving the public Helm and HTTP contracts.
Chart 2 completes the transition with one Go application image and one
container, as recorded in
[`chart-2-single-image.md`](chart-2-single-image.md).

This proposal supersedes the multi-cluster direction in
`docs/proposals/webui-redesign.md`. Its goals around build consistency, metric
clarity, UI quality, and maintainability remain applicable.

## Context

The former production NestJS process served built Vue assets, provided SPA
history fallback, proxied `/api/vgpu/*` to the Go backend, and exposed a basic
health endpoint. It did not aggregate page data, own metric semantics, maintain
sessions, authorize users, or route between clusters.

The same-origin entry is useful. The dedicated NestJS runtime was not. It
required a second production runtime and dependency graph without providing an
independently scalable service boundary.

Removing NestJS is primarily a maintainability, release, error-semantics, and
security-boundary improvement. It must not be presented as a fix for dashboard
request fan-out, frontend bundle size, or Prometheus query cost; those are
separate workstreams.

## Product boundaries

### Goals

- Preserve a stable same-origin Web and API contract.
- Support deployment at `/` and at a configurable URL base path.
- Support deliberate iframe embedding in an existing platform.
- Keep HAMi-WebUI single-cluster and read-only.
- Make authentication an explicit deployment boundary.
- Reduce production runtime and release complexity without breaking Chart 1.x
  users.

### Non-goals

- A built-in OAuth provider, login system, RBAC model, or server-side session.
- A central multi-cluster control plane or cluster registry.
- Workload creation, mutation, or lifecycle management.
- A widget SDK or micro-frontend framework in the first embedding iteration.
- A change to RBAC, NetworkPolicy, external authentication, or the default
  iframe policy as part of runtime consolidation.
- Metric correctness, dashboard aggregation, and exporter query optimization;
  those continue as independent workstreams.

## Decision

### Single-cluster, read-only application

Each HAMi-WebUI deployment observes one Kubernetes cluster. Platforms that
manage several clusters may embed or link to one deployment per cluster.
HAMi-WebUI will not store remote-cluster credentials or aggregate clusters in
the current architecture.

### External authentication boundary

HAMi-WebUI will not implement built-in OAuth or RBAC. Deployments that require
access control must place an authenticated Ingress or identity-aware reverse
proxy in front of the Web entry. That component owns login, session, user
policy, and access logging.

HAMi-WebUI documentation must describe this trusted-boundary assumption. The
application must not treat forwarded identity headers as authorization because
it has no internal permission model.

### Embedding contract

The supported initial embedding unit is the whole SPA, including deep links.
The contract includes:

- runtime base-path support without rebuilding the frontend;
- SPA deep-link refresh and relative static assets;
- API requests under the same base path and origin;
- a configurable CSP `frame-ancestors` policy;
- no hard-coded `X-Frame-Options` policy that prevents explicitly configured
  embedding; and
- documented external-auth and cookie requirements for cross-origin deployments.

For Chart 1.x, no restrictive framing header will be introduced by default
because that would break unknown existing integrations. This preserves
existing behavior; it is not a secure default. Without a framing policy, a
publicly reachable deployment can be embedded by an arbitrary origin and is
exposed to clickjacking risk.

The Chart accepts structured `frame-ancestors` sources and the Gateway renders
them as CSP. The setting has three states: omitted or `null` emits no
framing header to preserve Chart 1.x behavior; an empty list emits `'none'`; and
a non-empty list emits only its validated sources. `'self'` is allowed as an
explicit source and is never added implicitly. Deployers may instead enforce
the policy at the Ingress, but must not configure conflicting CSP or
`X-Frame-Options` headers at both layers. Multiple CSP headers are enforced
together rather than one overriding another.

Embedding tests must cover the unset, empty, explicit `'self'`, allowed
cross-origin, and unlisted cross-origin cases. The implementation review must
also inventory browser-reachable state-changing APIs. Adding such an API later
requires a new review of the default framing policy.

PR #113 is valuable prior work for runtime base-path resolution, tests, and
documentation and should be reused with contributor credit. URL sub-path
support alone is not complete iframe support: framing policy, external
authentication, cookies, navigation, and deep-link behavior must also be
verified.

### Post-v1.3 Web Gateway transition

This section records the compatibility bridge that preceded Chart 2; it is not
the current deployment contract.

The unreleased post-v1.3 transition retained separate frontend and backend
containers. Its frontend container used the standard-library Go Web entry
instead of a production NestJS runtime.

The following public contracts remain stable:

- `image.frontend.*` including digest precedence, `resources.frontend`, and
  `env.frontend` Helm values;
- existing `service.*`, `ingress.*`, and security-context configuration;
- the frontend container port `3000` and configurable Service port;
- the versioned HAMi Web API below `/api/vgpu/v1/*`;
- `/health_check` as a frontend-container liveness endpoint;
- existing SPA routes and Helm upgrade and rollback behavior; and
- amd64 and arm64 images.

For the new official Go Web entry, the prefix is an allowlisted application API
contract, not a tunnel to every backend endpoint. Unsupported API versions and
`/metrics`, `/readyz`, and `/q` remain backend-only even when requested through
`/api/vgpu`; no browser-facing route exposes them.

The Gateway implementation publishes a versioned frontend-container contract
covering the OCI entrypoint behavior, port, health endpoint, SPA and API paths,
and how base-path and framing configuration are supplied. The Chart no longer
forces the Nest-specific `node /apps/dist/main` command, so a conforming image
can use its own entrypoint.

The Helm value remains an extension point, not a promise that every existing
custom image supports new Gateway features. Compatibility is guaranteed for
the previous official frontend image at the root base path and for custom
images that implement the published container contract. Both pinning the
previous official image digest in the new Chart and `helm rollback` to the
previous Chart must be tested.

The previous Node-based official image retains its legacy wildcard
`/api/vgpu/*` proxy and HTTP 200 / `code: 599` proxy-error behavior when pinned
for rollback. Image rollback restores the previous availability contract; it
does not provide the new allowlist, status-code, caching, static-404, or
graceful-shutdown guarantees.

The current `/admin` SPA path segment is a legacy route name, not an
authentication boundary. Canonical route cleanup is separate work and must use
redirects rather than breaking existing deep links.

The Gateway must serve hashed assets with immutable caching, serve `index.html`
without long-lived caching, enable compression, return SPA fallback only for
frontend routes, preserve upstream API methods, bodies, headers, and status
codes, and return `502` or `504` for backend connection and timeout failures.
Unknown API and static-asset paths must not return `index.html` with HTTP 200.

`/health_check` reports whether the frontend Gateway can serve requests; it is
not a claim that the backend is healthy. Backend readiness remains the Go
server's `/readyz` responsibility, while individual API failures retain their
HTTP error status. The Chart uses separate frontend readiness and liveness
probes so a failed Gateway cannot leave the Pod ready merely because the
backend is healthy.

Go port `8000` is a backend listener, not a metrics-only listener: it serves the
API, `/readyz`, and `/metrics`. The post-v1.3 transition provided a separately
labelled internal backend ClusterIP Service, and ServiceMonitor selected only
that Service. The primary Web Service's existing port `8000` remained a
compatibility contract through the documented, deprecated
`service.legacyBackendPort` option. It was enabled by default during the
transition, could be disabled by security-sensitive deployments, and is removed
in Chart version 2.0.0. Distinct Service labels ensure that each Ready backend
Pod has one scrape target and is not discovered again through the primary
Service.

The selected Web entry supports a non-root, read-only container; amd64 and
arm64; deterministic proxy status codes; runtime base-path and framing
configuration; and a small standard-library runtime surface. The default proxy
timeout is explicit and longer than the default backend HTTP timeout. The
post-v1.3 transition used a reverse-proxy API adapter; Chart 2 reuses the same
static, routing, cache, compression, and framing handler with the in-process API
handler. This keeps the compatibility bridge from becoming throwaway
infrastructure.

The checked-in NestJS source and dependencies were removed only after the
minimal Gateway became the tested default and rollback through the previous
frontend image was verified.

### Observable contract

The behavior below applies to the official Go Web entry in both the post-v1.3
transition and the Chart 2 application unless a row is explicitly scoped to
the reverse-proxy bridge. The v1.3.0 frontend image is a rollback path under its
documented legacy semantics.

| Scenario | Required result |
| --- | --- |
| `/` and an existing SPA deep link | Serve the SPA at the root or configured base path |
| Missing static asset | Return HTTP 404; never return `index.html` |
| Existing `/api/vgpu/v1/*` request | Preserve method, query, body, relevant headers, response body, and upstream status |
| Unknown application request below `/api/vgpu/v1/*` | Preserve the backend's non-2xx JSON response |
| Unsupported version or backend-only `/metrics`, `/readyz`, or `/q` path, directly or below `/api/vgpu` | Return HTTP 404 without proxying |
| Post-v1.3 reverse-proxy bridge: backend connection refused / timed out | Return HTTP 502 / 504 with a non-success JSON response |
| `/health_check` | Return HTTP 200 when the public Web entry can serve requests |
| `index.html` / hashed asset | No long-lived cache / immutable cache with compression |
| Iframe parent | Unset preserves the existing behavior; `[]` blocks all parents; explicit `'self'` and origin sources allow only those parents |
| Primary Web / internal backend Services | The primary Chart 2 Service exposes only `3000`; ServiceMonitor discovers each Ready Pod exactly once through the internal `8000` Service |
| Architecture | The official application image passes non-root, read-only-filesystem, amd64, and arm64 smoke tests |
| Migration | Chart 2 upgrade with fresh values and full Helm rollback to Chart 1.3 both pass at the root base path |

### Chart version 2.0.0 follow-up

The unified application and image passed the HTTP, browser, non-root,
read-only-filesystem, and multi-architecture gates before becoming the Chart 2
runtime. Keeping an unreleased two-image transition for another release would
have preserved publication and operational complexity without an independent
scaling boundary, so maintainers accepted the explicit major-version migration.

Chart 2 exposes named `http` and `metrics` container ports from one Go
application. The names describe traffic, not an authorization boundary. The
primary Service exposes only port `3000`; a separate internal Service keeps
port `8000` discoverable for readiness and metrics. Removed Chart 1.x values
fail with a clear migration message rather than being silently ignored.

The release remains gated on Helm upgrade and rollback verification plus final
maintainer browser acceptance. The complete Chart 2 decision and migration
boundary are recorded in [`chart-2-single-image.md`](chart-2-single-image.md).

## Consequences

- Node.js remains a frontend build dependency, but the production runtime and
  checked-in NestJS dependency graph are removed.
- Chart 1.x retained two containers as a compatibility bridge. Chart 2 has one
  image, one container resource budget, and no same-Pod proxy hop.
- Leaving the framing allowlist empty preserves compatibility but leaves the
  existing clickjacking risk to the deployment boundary.
- Custom frontend-only images remain a Chart 1.x rollback concern. Chart 2
  accepts a complete application image, not an independently replaceable
  frontend container.
- Built-in authentication and multi-cluster control remain intentionally
  deferred.

## Implementation sequence

1. Add a Gateway-independent test harness and passing baseline tests for the
   current root SPA, deep-link refresh, API method/body/status forwarding,
   `/health_check`, and current iframe behavior. Do not lock known incorrect
   error or caching behavior into the baseline.
2. Introduce the selected Go Web entry at the root path with target-behavior
   tests for static 404s, `502`/`504`, caching, compression, probes, non-root and
   read-only execution, and previous-image compatibility.
3. Add runtime base-path and framing allow/deny behavior as a separate
   compatibility increment with browser-level embedding tests.
4. Add the internal backend Service and deprecated legacy-port switch with
   explicit upgrade notes and per-Ready-Pod exactly-once ServiceMonitor tests.
5. Verify Kubernetes install, upgrade, Helm rollback, image rollback, and
   backend failure recovery in an integration environment.
6. Delete the unused NestJS runtime and dependencies after parity and rollback
   are established.
7. Adopt the independently verified unified image through the explicit Chart 2
   values and rollback boundary.

Gateway pull requests do not require GPU hardware. Every stable release remains
subject to the risk-based Kubernetes, GPU, and maintainer browser acceptance in
[`docs/releasing.md`](../releasing.md); this proposal does not weaken that gate.

## Alternatives considered

### Keep NestJS as a future BFF

Rejected for the current product. Future OAuth or multi-cluster ideas do not
justify retaining an empty runtime boundary. A new Gateway or BFF can be
designed if a concrete client-specific or session-owning requirement appears.

### Move directly to Go static embedding in Chart 1.x

Rejected for the Chart 1.x compatibility bridge. The same consolidation was
later adopted at the explicit Chart 2 major-version boundary.

### Add multi-cluster routing to the current Web process

Rejected. A multi-cluster control plane requires credential storage, tenant
authorization, audit, availability, query budgets, and partial-failure
semantics and is outside HAMi-WebUI's confirmed scope.
