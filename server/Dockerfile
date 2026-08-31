FROM --platform=$BUILDPLATFORM golang:1.26.7-bookworm@sha256:e8c859f5632dcfde7b32d2012b4351728f6437930887c2f6a91ea242459e5514 AS builder

WORKDIR /src

ARG BUILDARCH
ARG TARGETARCH
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
    test -s /etc/ssl/certs/ca-certificates.crt && \
    rm -rf /var/lib/apt/lists/*

# Install the same checksum-verified protoc release used by CI.
COPY hack/install-protoc.sh /tmp/install-protoc.sh
RUN bash /tmp/install-protoc.sh /usr/local "${BUILDARCH}" && \
    rm /tmp/install-protoc.sh

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

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /src/build/ /apps/
