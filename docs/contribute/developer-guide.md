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
export PATH="$(brew --prefix node@24)/bin:$PATH"
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

## Build and run HAMi-WebUI

Chart 2 deploys one application image and one container. Its Go process serves
the built Vue application on port `3000`, invokes the versioned API handler
in-process, and exposes readiness, metrics, and diagnostics on the internal port
`8000`.

Node.js and pnpm remain build and test dependencies for the Vue application.
They are not present in the production image.

### Backend development

The backend reads the active kubeconfig context and
[`server/config/config.yaml`](../../server/config/config.yaml). Select a
development cluster you are authorized to inspect and configure a reachable
Prometheus address before starting it.

Generate the API code and start the backend from the repository root:

```bash
make -C server run
```

Swagger is available at `http://localhost:8000/q/swagger-ui`.

### Frontend development

Start the backend first, then run `make dev` in the repository root. This starts
the Vite development server; NestJS is not part of the browser development
path. Vite forwards `/api/vgpu/v1/*` to `http://127.0.0.1:8000` and removes the
`/api/vgpu` prefix.

Open `http://localhost:3000/`. Set `HAMI_WEBUI_BACKEND_URL` when the backend
uses another origin:

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

To exercise the standalone Web entry while changing its static-serving
contract:

```bash
make build build-web-entry
server/build/web-entry --static-dir ./public
```

The backend must be available at `http://127.0.0.1:8000` for API requests in
this standalone mode.

### Chart 2 application contract

The official image owns its OCI entrypoint; the Chart does not inject an
implementation-specific command. The image runs as a numeric non-root user and
is verified on amd64 and arm64 with a read-only root filesystem.

The public listener exposes the SPA and only the versioned HAMi Web API below
`/api/vgpu/v1/*`. Backend-only `/metrics`, `/readyz`, and `/q` endpoints remain
on port `8000` for internal diagnostics and scraping. The browser API calls the
backend handler directly in-process; there is no loopback reverse proxy.

The unified application accepts these optional environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `HAMI_WEBUI_LISTEN_ADDRESS` | `:3000` | Public Web listener; the Chart requires `:3000` |
| `HAMI_WEBUI_STATIC_DIR` | `/apps/public` | Built Vue asset directory |
| `HAMI_WEBUI_BASE_PATH` | `/` | Public URL prefix; the reverse proxy must preserve it |
| `HAMI_WEBUI_FRAME_ANCESTORS_JSON` | unset | JSON framing allowlist; `[]` blocks all parents |
| `HAMI_WEBUI_HEALTHCHECK_URL` | `http://127.0.0.1:3000/health_check` | Target used only by `--healthcheck` |

`HAMI_WEBUI_BACKEND_URL` and `HAMI_WEBUI_PROXY_TIMEOUT` apply only to the
standalone Web-entry development binary. They are intentionally absent from the
Chart 2 in-process runtime. The public port, health endpoint, and versioned API
prefix are compatibility contracts; filesystem paths and executable names are
not.

The application can serve the SPA below a configured base path and emit a
validated CSP `frame-ancestors` policy. These are runtime settings, not Vite
build variables. The unprefixed `/health_check` remains stable for Chart probes.
When installing with Helm, use `frontend.basePath` and
`frontend.frameAncestors`; the Chart owns the corresponding generated
environment variables so its Ingress checks and access notes describe the
actual runtime. Direct image and local binary users can continue using the
environment variables above.

### Chart 2 Service and probe topology

The primary Service routes browser traffic to port `3000`. An independently
labelled `*-backend` ClusterIP Service routes port `8000` to the same container
and is the only Service selected by the included ServiceMonitor. The `backend`
component label belongs to this discovery Service, not to a separate workload
or authorization boundary. The primary Service never exposes port `8000` in
Chart 2.

The startup probe waits for `/readyz` on port `8000` for up to 300 seconds so
Kubernetes informer caches can synchronize. After startup, readiness and
liveness probe `/health_check` on port `3000`. This separates slow initial data
bootstrap from the ongoing ability to serve the Web entry.

## Build a Docker image

Build the same application image used by development publication, Chart 2, and
release candidates:

```bash
make build-image
```

The resulting image is tagged `hami-webui:dev`. Override `DOCKER_IMAGE`,
`VERSION`, or `PLATFORM` when a local integration environment needs another tag
or architecture:

```bash
make build-image \
  DOCKER_IMAGE=my-registry/hami-webui \
  VERSION=test \
  PLATFORM=linux/amd64
```

This target builds one local image and never pushes it. Stable multi-platform
images, the Helm chart, release tag, and release notes are published only
through the [verified release process](../releasing.md).

Chart 1.3 rollback restores its complete two-image deployment. Do not combine a
Chart 1.x frontend or backend image with the Chart 2 templates; see the
[Helm migration guide](../installation/helm/index.md#upgrade-from-chart-13-to-20).
