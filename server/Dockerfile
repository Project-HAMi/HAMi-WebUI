FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS builder

WORKDIR /src

ARG BUILDARCH
ARG TARGETARCH
ARG PROTOC_VERSION=28.3
ARG PROTOC_SHA256_AMD64=0ad949f04a6a174da83cdcbdb36dee0a4925272a5b6d83f79a6bf9852076d53f
ARG PROTOC_SHA256_ARM64=1de522032a8b194002fe35cab86d747848238b5e4de4f99648372079f5b46f9a
ARG DEBIAN_SNAPSHOT=20260824T000000Z
ARG UNZIP_VERSION=6.0-28

# The Go image includes curl but not unzip. Resolve the exact package from a
# dated Debian snapshot so rebuilding the same source cannot pick up a newer
# repository state.
RUN sed -i -E \
      -e "s|https?://deb.debian.org/debian-security|https://snapshot.debian.org/archive/debian-security/${DEBIAN_SNAPSHOT}|g" \
      -e "s|https?://deb.debian.org/debian|https://snapshot.debian.org/archive/debian/${DEBIAN_SNAPSHOT}|g" \
      /etc/apt/sources.list.d/debian.sources && \
    apt-get -o Acquire::Check-Valid-Until=false update && \
    apt-get install -y --no-install-recommends "unzip=${UNZIP_VERSION}" && \
    rm -rf /var/lib/apt/lists/*

# Install the exact protoc release used by CI. The archive checksum makes this
# input immutable.
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

# Install code generators at versions compatible with the application's locked
# Go dependencies. Never resolve release inputs from @latest.
COPY Makefile ./
RUN make install-deps

# Download Go modules (cached layer).
COPY go.mod go.sum ./
RUN go mod download

# Generate and build. verify-mod fails if generation would require silently
# rewriting go.mod or go.sum.
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make generate verify-mod build-linux TARGET_ARCH=${TARGETARCH} DIRS=server

FROM debian:bookworm-20260824-slim@sha256:88200866dfff7ea7f5cbcb6ec7c8a701889efe6fe859fe64d6990e4b07ea4171

COPY --from=builder /src/build/ /apps/
