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

FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS web-entry-builder

WORKDIR /src/server

ARG TARGETOS=linux
ARG TARGETARCH

COPY server/go.mod server/go.sum ./
COPY server/cmd/web-entry/ ./cmd/web-entry/
COPY server/internal/webentry/ ./internal/webentry/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOTOOLCHAIN=local \
    go build -mod=readonly -trimpath -o /out/web-entry ./cmd/web-entry

FROM scratch

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
