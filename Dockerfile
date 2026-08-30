FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm@sha256:be23f54a88d34e8824c741b19b91064094f92c1c97b194144bfc8b50d67258e2 AS web-builder

WORKDIR /src

# Enable corepack to use pnpm version from package.json packageManager field
RUN corepack enable

# Copy dependency manifests before application sources so source-only changes
# can reuse the dependency layer.
COPY .browserslistrc package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/web/package.json packages/web/

# Install dependencies
RUN pnpm install --frozen-lockfile --filter hami-webui-web...

COPY packages/web/ packages/web/
COPY scripts/precompress-web-assets.mjs scripts/precompress-web-assets.mjs

# Build the browser application. Node.js is a build-time dependency only; the
# production image runs the Go Web entry below.
RUN pnpm --filter hami-webui-web run build

# Pre-compress immutable browser assets once at build time. The Web entry serves
# these siblings when the client advertises gzip support.
RUN pnpm run precompress:web-assets

FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS go-base

WORKDIR /src/server

FROM go-base AS web-entry-builder

ARG TARGETOS=linux
ARG TARGETARCH

COPY server/go.mod server/go.sum ./
COPY server/cmd/web-entry/ ./cmd/web-entry/
COPY server/internal/webentry/ ./internal/webentry/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOTOOLCHAIN=local \
    go build -mod=readonly -trimpath -o /out/web-entry ./cmd/web-entry

FROM scratch AS frontend-runtime

WORKDIR /apps

# Keep HTTPS proxy targets and the existing TZ environment extension point
# functional without carrying a production Linux package layer.
COPY --from=web-entry-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=web-entry-builder /usr/share/zoneinfo/ /usr/share/zoneinfo/
COPY --from=web-entry-builder --chown=65532:65532 /out/web-entry /apps/web-entry
COPY --from=web-builder --chown=65532:65532 /src/public/ /apps/public/

USER 65532:65532

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/apps/web-entry", "--healthcheck"]

ENTRYPOINT ["/apps/web-entry"]

FROM go-base AS unified-builder

ARG BUILDARCH
ARG TARGETARCH
ARG PROTOC_VERSION=28.3
ARG PROTOC_SHA256_AMD64=0ad949f04a6a174da83cdcbdb36dee0a4925272a5b6d83f79a6bf9852076d53f
ARG PROTOC_SHA256_ARM64=1de522032a8b194002fe35cab86d747848238b5e4de4f99648372079f5b46f9a
ARG DEBIAN_SNAPSHOT=20260824T000000Z
ARG UNZIP_VERSION=6.0-28

# Match the pinned backend toolchain while the released backend image remains
# independently buildable from server/Dockerfile.
RUN sed -i -E \
      -e "s|https?://deb.debian.org/debian-security|https://snapshot.debian.org/archive/debian-security/${DEBIAN_SNAPSHOT}|g" \
      -e "s|https?://deb.debian.org/debian|https://snapshot.debian.org/archive/debian/${DEBIAN_SNAPSHOT}|g" \
    /etc/apt/sources.list.d/debian.sources && \
    apt-get -o Acquire::Check-Valid-Until=false update && \
    apt-get install -y --no-install-recommends "unzip=${UNZIP_VERSION}" && \
    test -s /etc/ssl/certs/ca-certificates.crt && \
    rm -rf /var/lib/apt/lists/*

RUN case "${BUILDARCH}" in \
      amd64) protoc_arch="x86_64"; protoc_sha="${PROTOC_SHA256_AMD64}" ;; \
      arm64) protoc_arch="aarch_64"; protoc_sha="${PROTOC_SHA256_ARM64}" ;; \
      *) echo "unsupported build architecture: ${BUILDARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSLo /tmp/protoc.zip \
      "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${protoc_arch}.zip" && \
    echo "${protoc_sha}  /tmp/protoc.zip" | sha256sum -c - && \
    unzip -q /tmp/protoc.zip -d /usr/local && \
    rm /tmp/protoc.zip

COPY server/Makefile ./
RUN make install-deps

COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make generate verify-mod build-linux TARGET_ARCH=${TARGETARCH} DIRS=hami-webui

FROM scratch AS unified

WORKDIR /apps

COPY --from=unified-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=unified-builder /usr/share/zoneinfo/ /usr/share/zoneinfo/
COPY --from=unified-builder --chown=65532:65532 /src/server/build/hami-webui /apps/hami-webui
COPY --from=web-builder --chown=65532:65532 /src/public/ /apps/public/

USER 65532:65532

EXPOSE 3000 8000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/apps/hami-webui", "--healthcheck"]

ENTRYPOINT ["/apps/hami-webui"]
CMD ["--conf", "/apps/config/config.yaml"]

# Keep the released frontend image as the implicit Docker build target until
# the Chart and release controller migrate atomically in their own PRs.
FROM frontend-runtime AS frontend
