# Developer guide

This guide helps you get started developing HAMi-WebUI.

## Dependencies

Make sure you have the following dependencies installed before setting up your developer environment:

- [Git](https://git-scm.com/)
- Make
- [Go](https://golang.org/dl/) (use the version in [`server/.go-version`](../../server/.go-version))
- [Node.js](https://nodejs.org/) (use the version in [`.node-version`](../../.node-version))
- [pnpm](https://pnpm.io/) (use the version in the root `package.json`)
- [Protocol Buffers compiler](https://protobuf.dev/installation/) (`protoc`)
- Docker, only when building local container images


### macOS

We recommend using [Homebrew](https://brew.sh/) for installing any missing dependencies:

```
brew install git
brew install go
brew install node@24
brew install protobuf
corepack enable
```

## Download HAMi-WebUI

We recommend using the Git command-line interface to download the source code for the HAMi-WebUI project:

1. Open a terminal and run `git clone https://github.com/Project-HAMi/HAMi-WebUI.git`. This command downloads HAMi-WebUI to a new `hami-webui` directory in your current directory.
2. Open the `HAMi-WebUI` directory in your favorite code editor.

For alternative ways of cloning the HAMi-WebUI repository, refer to [GitHub's documentation](https://docs.github.com/en/github/creating-cloning-and-archiving-repositories/cloning-a-repository).

## Bootstrap local dependencies

Run dependency installation explicitly once from the repository root. Normal
build and development commands never update dependencies as a side effect.

```bash
make bootstrap
export PATH="$(go env GOPATH)/bin:$PATH"
make -C server bootstrap
pnpm exec playwright install chromium
```

`make bootstrap` uses the committed lockfile and fails instead of rewriting it.
The server bootstrap installs the pinned Go code generators; `protoc` remains a
system prerequisite. Re-run the relevant bootstrap command only after its
dependency or tool versions change. On Linux, use
`pnpm exec playwright install --with-deps chromium` when Chromium's system
libraries are not already installed.

## Build HAMi-WebUI

The Chart 1.x deployment consists of two containers:

- the Go API and metrics backend; and
- the Web entry, which serves the built Vue application and proxies
  `/api/vgpu/v1/*` to the backend.

Node.js is required to build the Vue application. The frontend image built from
this revision uses Go at runtime; the previous official Node image remains
supported for Chart 1.x rollback.

### Backend

Generate the API code and start the backend from the repository root:

```bash
make -C server run
```

By default, you can access the web-ui-server-swagger at `http://localhost:8000/q/swagger-ui`.

### Frontend

Start the backend first, then run `make dev` in the repository root. This
starts the Vite development server; NestJS is not part of the browser
development path. Vite forwards browser requests below `/api/vgpu/v1/*`
directly to the backend at `http://127.0.0.1:8000` and removes the
`/api/vgpu` prefix.

By default, you can access the web-ui at `http://localhost:3000/`.

Set `HAMI_WEBUI_BACKEND_URL` when the backend uses a different origin:

```bash
HAMI_WEBUI_BACKEND_URL=http://127.0.0.1:18000 make dev
```

Before opening a pull request, run the checks for the area you changed:

```bash
make verify
make -C server verify
bash scripts/verify-chart.sh  # when the Helm chart changes
```

The root verification target covers Vue lint and tests, the production asset
build, the Go Web entry, and its HTTP and Chromium contracts. The server target
generates code before building, vetting, and testing every Go package. CI calls
the same granular Make targets so local and pull-request behavior stay aligned.

To exercise the production Web entry locally:

```bash
make build build-web-entry
server/build/web-entry --static-dir ./public
```

The backend must be available at `http://127.0.0.1:8000` for API requests.

### Chart 1.x frontend image contract

The frontend image owns its OCI entrypoint; the Chart does not inject a
Node-specific command. A compatible custom image must:

- listen on port `3000`;
- return HTTP 200 from `/health_check` without depending on backend readiness;
- serve the SPA and its static assets;
- forward `/api/vgpu/v1/*` to the backend while preserving HTTP status codes;
  and
- terminate cleanly on `SIGTERM`.

The new official Go image additionally runs as a numeric non-root user and is
verified with a read-only root filesystem in CI. Those are official-image
security gates, not new requirements retroactively imposed on the previous
official image.

The new official Go Web entry exposes only the versioned HAMi Web API through
the same-origin entry. Backend-only `/metrics`, `/readyz`, and `/q` endpoints
remain available only on the backend listener for internal diagnostics and
scraping. Pinning the previous Node-based official image restores its legacy
wildcard `/api/vgpu/*` proxy and HTTP 200 / `code: 599` proxy-error behavior;
that rollback path does not provide the new routing or error guarantees. The
legacy process may also require forced termination when the Pod's termination
grace period expires.

The official Go Web entry also accepts these optional environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `HAMI_WEBUI_LISTEN_ADDRESS` | `:3000` | Standalone Web listener; Chart 1.x requires `:3000` |
| `HAMI_WEBUI_BACKEND_URL` | `http://127.0.0.1:8000` | Backend origin |
| `HAMI_WEBUI_STATIC_DIR` | `/apps/public` | Built Vue asset directory |
| `HAMI_WEBUI_PROXY_TIMEOUT` | `65s` | End-to-end backend request timeout; keep it longer than `backend.http.timeout` |
| `HAMI_WEBUI_BASE_PATH` | `/` | Public URL prefix; the reverse proxy must preserve it |
| `HAMI_WEBUI_FRAME_ANCESTORS_JSON` | unset | JSON framing allowlist; `[]` blocks all parents |
| `HAMI_WEBUI_HEALTHCHECK_URL` | `http://127.0.0.1:3000/health_check` | Target used only by `--healthcheck` |

The public port, health endpoint, and versioned API prefix are compatibility
contracts.
Filesystem paths and executable names inside the image are not.

The official Go frontend can serve the SPA below a configured base path and
emit a validated CSP `frame-ancestors` policy. These settings are runtime
configuration, not Vite build variables. The unprefixed `/health_check`
endpoint remains stable for Chart probes.

### Chart 1.x Service topology

The primary Service routes browser traffic to the Web entry on port `3000`.
An independently labelled `*-backend` ClusterIP Service routes port `8000` to
the backend container and is the only Service selected by the included
ServiceMonitor. The Deployment keeps `component: hami-webui` because both
containers share one Pod; the backend component label belongs to the Service
used for discovery, not to a separate backend workload.

The primary Service retains a deprecated port `8000` only when
`service.legacyBackendPort` is enabled for Chart 1.x compatibility. Do not
widen the ServiceMonitor selector to include both Services: that would create
duplicate scrape targets. The ClusterIP split also does not create an
authorization boundary; cluster-internal access policy remains a deployment
concern.

## Build a Docker image

To build a frontend development image for the local Docker platform, run:

```bash
make build-image
```

The resulting image is tagged as `hami-webui-frontend:dev`. It
contains the Go Web entry and built Vue assets, but no production Node.js
runtime.

Build the backend development image from the repository root:

```bash
make -C server build-image
```

The resulting image is tagged as `hami-webui-backend:dev`. Override
`DOCKER_IMAGE`, `VERSION`, or `PLATFORM` when a local integration environment
needs a different tag or architecture, for example:

```bash
make build-image DOCKER_IMAGE=my-registry/hami-webui-frontend VERSION=test PLATFORM=linux/amd64
```

These targets build one local development image and never push it. Stable
multi-platform images, the Helm chart, release tag, and release notes are
published only through the [verified release process](../releasing.md).
