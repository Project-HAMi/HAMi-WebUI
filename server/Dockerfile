FROM --platform=$BUILDPLATFORM golang:1.25.1 AS builder

WORKDIR /src

ARG TARGETARCH

# 1. Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler

# 2. Install Go tools (Cached layer)
RUN go install github.com/google/wire/cmd/wire@latest && \
    go install github.com/envoyproxy/protoc-gen-validate@latest && \
    go install github.com/go-kratos/kratos/cmd/kratos/v2@latest && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest && \
    go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest

# 3. Download Go modules (Cached layer)
COPY go.mod go.sum ./
RUN go mod download

# 4. Build (Leverage build cache)
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make generate build-linux TARGET_ARCH=${TARGETARCH}

FROM debian:stable-slim

COPY --from=builder /src/build/ /apps/
