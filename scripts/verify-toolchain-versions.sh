#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
node_version="$(<"${repo_root}/.node-version")"
go_version="$(<"${repo_root}/server/.go-version")"

grep -Fq "node:${node_version}-bookworm@" "${repo_root}/Dockerfile"
grep -Fq "golang:${go_version}-bookworm@" "${repo_root}/Dockerfile"
grep -Fq "golang:${go_version}-bookworm@" "${repo_root}/server/Dockerfile"

echo "Toolchain versions match their pinned Docker images."
