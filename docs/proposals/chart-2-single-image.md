---
title: Chart 2 Single-Image Runtime
authors:
- "@Nimbus318"
approvers: []
creation-date: 2026-08-31
status: accepted
---

# Chart 2 Single-Image Runtime

## Context

Chart 1.x ran a Web entry and the HAMi API as two containers and published two
images. They shared one Pod and release lifecycle, so the split did not provide
independent scaling or isolation. It did add a second image, resource contract,
upgrade surface, and production runtime to operate.

The Go application can now serve the built Vue assets and invoke the API handler
in-process. Node.js remains necessary to build and test the Vue application, but
is not needed in the production image.

## Decision

Chart 2 deploys one `projecthami/hami-webui` image and one container. The process
keeps two HTTP listeners with different exposure:

- port `3000` is the supported browser entry for the SPA and
  `/api/vgpu/v1/*`;
- port `8000` is the internal readiness, metrics, and diagnostics listener. A
  separate internal Service exposes it for metrics scraping; it is not an
  authentication boundary or a public API contract.

The primary Service exposes only port `3000`. The existing internal Service and
ServiceMonitor topology remain so Prometheus continues to discover one target
per Ready Pod.

Chart 2 uses flat `image`, `resources`, and `env` values. It retains
`frontend.basePath`, `frontend.frameAncestors`, external Prometheus TLS, and the
existing Service, Ingress, ServiceMonitor, scheduling, and security-context
extension points. This decision does not change RBAC, NetworkPolicy, external
authentication, or the default iframe policy.

## Migration and rollback

Chart 1.x nested image, resource, environment, probe, proxy, gRPC, and legacy
backend-port values are rejected instead of silently ignored. Operators must
create a fresh Chart 2 values file and upgrade with `--reset-values`.

The supported rollback is `helm rollback` to the previous Chart 1.3 revision,
which restores its templates, values, and image pair together. Mixing a Chart
1.x frontend or backend image into the Chart 2 Pod is not supported.

## Consequences

- The Web UI, API, and static assets are released and rolled back atomically.
- Production has one image, one SBOM/CVE surface, and one container resource
  budget.
- Browser and internal listener semantics remain distinct even though they run
  in one process.
- The major Chart version and fail-closed value validation make the breaking
  deployment contract explicit.
