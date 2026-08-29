---
title: HAMi-WebUI Single-Cluster Web Entry and Embedding Architecture
authors:
- "@Nimbus318"
approvers: []
creation-date: 2026-08-30
status: proposed
---

# HAMi-WebUI Single-Cluster Web Entry and Embedding Architecture

## Summary

HAMi-WebUI will remain a single-cluster, read-only observability UI. It must also
be easy for platform teams to embed in an existing portal without rebuilding the
frontend for each deployment.

The same-origin Web entry remains part of the product contract, but the current
pass-through NestJS process is not required to provide it. During HAMi-WebUI
Chart package 1.x, the production NestJS runtime will be replaced by a minimal
static-file and reverse-proxy Gateway while preserving the public Helm and HTTP
contracts.

A single Go application image is deferred to Chart version 2.0.0 and will be
considered only after compatibility and adoption conditions are met.

This proposal supersedes the multi-cluster direction in
`docs/proposals/webui-redesign.md`. Its goals around build consistency, metric
clarity, UI quality, and maintainability remain applicable.

## Context

The production NestJS process currently serves built Vue assets, provides SPA
history fallback, proxies `/api/vgpu/*` to the Go backend, and exposes a basic
health endpoint. It does not aggregate page data, own metric semantics, maintain
sessions, authorize users, or route between clusters.

The same-origin entry is useful. The dedicated NestJS runtime is not. Keeping it
requires a second production runtime, image, dependency graph, vulnerability
surface, and release artifact without providing an independently scalable
service boundary.

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
- An immediate migration to one Go image.
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

The Chart will accept structured `frame-ancestors` sources and the Gateway will
render them as CSP. The setting has three states: omitted or `null` emits no
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

### Chart 1.x Web Gateway

Chart 1.x will continue to deploy separate frontend and backend containers.
The frontend container will use a minimal Nginx, Caddy, or equivalent Gateway
instead of a production NestJS runtime.

The following public contracts remain stable:

- `image.frontend.*` including digest precedence, `resources.frontend`, and
  `env.frontend` Helm values;
- existing `service.*`, `ingress.*`, and security-context configuration;
- the frontend container port `3000` and configurable Service port;
- `/api/vgpu/*`;
- `/health_check` as a frontend-container liveness endpoint;
- existing SPA routes and Helm upgrade and rollback behavior; and
- amd64 and arm64 images.

The Gateway implementation PR must publish a versioned frontend-container
contract before changing the default image. At minimum, that contract defines
the OCI entrypoint behavior, port, health endpoint, SPA and API paths, and how
base-path and framing configuration are supplied. The Chart must stop forcing
the Nest-specific `node /apps/dist/main` command so a conforming image can use
its own entrypoint.

The Helm value remains an extension point, not a promise that every existing
custom image supports new Gateway features. Compatibility is guaranteed for
the previous official frontend image at the root base path and for custom
images that implement the published container contract. Both pinning the
previous official image digest in the new Chart and `helm rollback` to the
previous Chart must be tested.

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
HTTP error status. The Chart will add frontend readiness and liveness probes so
a failed Gateway cannot leave the Pod ready merely because the backend is
healthy.

Go port `8000` is a backend listener, not a metrics-only listener: it serves the
API, `/readyz`, and `/metrics`. Chart 1.x will add a separately labelled internal
backend ClusterIP Service and make ServiceMonitor select only that Service. The
primary Web Service's existing port `8000` is a compatibility contract and will
not be silently removed. It will become a documented, deprecated
`service.legacyBackendPort` option that remains enabled by default in Chart 1.x,
can be disabled by security-sensitive deployments, and is removed only in Chart
version 2.0.0. Distinct Service labels must ensure that each Ready backend Pod
has one scrape target and is not discovered again through the primary Service.

The Gateway implementation will be selected with a small checked-in spike. The
choice must support a non-root, read-only container; amd64 and arm64; runtime
base-path and framing configuration; deterministic proxy status codes; and a
small, actively maintained runtime surface. Technology preference alone is not
a selection criterion. Proxy timeouts must be explicit and must not be shorter
than the configured Go HTTP timeout.

NestJS source and dependencies will be deleted only after the minimal Gateway is
the tested default and rollback through the previous frontend image has been
verified.

### Observable contract

| Scenario | Required result |
| --- | --- |
| `/` and an existing SPA deep link | Serve the SPA at the root or configured base path |
| Missing static asset | Return HTTP 404; never return `index.html` |
| Existing `/api/vgpu/*` request | Preserve method, body, relevant headers, response body, and upstream status |
| Unknown `/api/vgpu/*` request | Preserve the backend's non-2xx JSON response |
| Backend connection refused / timed out | Return HTTP 502 / 504 with a non-success JSON response |
| `/health_check` | Return HTTP 200 when the Gateway can serve, independent of backend readiness |
| `index.html` / hashed asset | No long-lived cache / immutable cache with compression |
| Iframe parent | Unset preserves the existing behavior; `[]` blocks all parents; explicit `'self'` and origin sources allow only those parents |
| Primary Web / internal backend Services | Disabling the legacy backend port removes `8000` from Web exposure; ServiceMonitor discovers each Ready backend Pod exactly once |
| Architecture | The new official Gateway frontend image passes non-root, read-only-filesystem, amd64, and arm64 smoke tests |
| Migration | New Chart plus previous official frontend digest, and Helm rollback, both pass at the root base path |

### Chart version 2.0.0 gates

A unified Go application image is an option, not a committed Chart 1.x
deliverable. Exploration may begin when all of the following are true:

1. The minimal Gateway has been stable for at least one release cycle.
2. Maintainers have collected feedback on custom `image.frontend`, branding,
   base-path, and iframe usage.
3. There is a testable hypothesis for release or operational benefit.
4. Maintainers accept a Chart major-version migration.

The unified image may become the default only after its benefit is measured;
upgrade, both rollback paths, multi-architecture, and read-only-filesystem tests
pass; and the release controller is migrated from two components to one without
weakening fail-closed digest or publication checks.

If adopted, Chart version 2.0.0 may expose named `web` and `metrics` listeners
from one Go application image. It must not call them `public` and `admin`,
because those names imply an authorization boundary that does not exist.
Removed Chart 1.x values must fail with a clear migration message rather than
being silently ignored.

## Consequences

- The production Node.js and NestJS dependency graph can be removed after the
  Gateway migration, reducing runtime and release maintenance.
- Chart 1.x deliberately retains two containers, two images, and one same-Pod
  proxy hop. Those costs are accepted to preserve upgrade and customization
  contracts.
- Leaving the framing allowlist empty preserves compatibility but leaves the
  existing clickjacking risk to the deployment boundary.
- Custom frontend images are supported only through the documented container
  contract; arbitrary internal layouts are not a compatibility promise.
- A single Go image, Chart version 2.0.0, built-in authentication, and
  multi-cluster control remain intentionally deferred.

## Migration plan

1. Add a Gateway-independent test harness and passing baseline tests for the
   current root SPA, deep-link refresh, API method/body/status forwarding,
   `/health_check`, and current iframe behavior. Do not lock known incorrect
   error or caching behavior into the baseline.
2. Select and document the minimal Gateway implementation using the acceptance
   criteria above.
3. Introduce target-behavior tests together with the Gateway: static 404s,
   `502`/`504`, caching, compression, root and sub-path deployment, framing
   allow/deny, probes, security context, and both rollback paths.
4. Verify Kubernetes install, upgrade, Helm rollback, image rollback, and
   backend failure recovery in an integration environment.
5. Delete the unused NestJS runtime and dependencies after parity is established.
6. Add the internal backend Service and deprecated legacy-port switch with
   explicit upgrade notes and per-Ready-Pod exactly-once ServiceMonitor tests.
7. Treat a single Go image as separate Chart version 2.0.0 work after the
   exploration gates are satisfied.

Gateway pull requests do not require GPU hardware. Every stable release remains
subject to the risk-based Kubernetes, GPU, and maintainer browser acceptance in
[`docs/releasing.md`](../releasing.md); this proposal does not weaken that gate.

## Alternatives considered

### Keep NestJS as a future BFF

Rejected for the current product. Future OAuth or multi-cluster ideas do not
justify retaining an empty runtime boundary. A new Gateway or BFF can be
designed if a concrete client-specific or session-owning requirement appears.

### Move directly to Go static embedding in Chart 1.x

Deferred. It would remove the public `image.frontend` customization and change
Chart, release, and rollback contracts before their real usage is known.

### Add multi-cluster routing to the current Web process

Rejected. A multi-cluster control plane requires credential storage, tenant
authorization, audit, availability, query budgets, and partial-failure
semantics and is outside HAMi-WebUI's confirmed scope.
