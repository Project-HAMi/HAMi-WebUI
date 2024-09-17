FROM golang:1.23.1 AS builder

WORKDIR /src

ENV GOPROXY=https://goproxy.cn,direct

COPY . .

RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler

RUN make build

FROM debian:stable-slim

COPY --from=builder /src/build/ /apps/
