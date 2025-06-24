FROM --platform=$BUILDPLATFORM golang:1.23.1 AS builder

WORKDIR /src

ARG TARGETARCH

COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler

RUN make build TARGET_ARCH=${TARGETARCH}

FROM --platform=$TARGETPLATFORM debian:stable-slim

COPY --from=builder /src/build/ /apps/
