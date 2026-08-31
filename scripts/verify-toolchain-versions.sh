#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
node_version="$(<"${repo_root}/.node-version")"
go_version="$(<"${repo_root}/server/.go-version")"

grep -Fq "node:${node_version}-bookworm@" "${repo_root}/Dockerfile"
grep -Fq "golang:${go_version}-bookworm@" "${repo_root}/Dockerfile"
grep -Fq "golang:${go_version}-bookworm@" "${repo_root}/server/Dockerfile"
grep -Fq "server/hack/install-protoc.sh" "${repo_root}/Dockerfile"
grep -Fq "hack/install-protoc.sh" "${repo_root}/server/Dockerfile"
go_workflow="${repo_root}/.github/workflows/pr-go.yaml"
ci_installer="bash hack/install-protoc.sh \"\${RUNNER_TEMP}/protoc\""
[[ "$(grep -Fc "${ci_installer}" "${go_workflow}")" == "2" ]]
if grep -Fq "arduino/setup-protoc" "${go_workflow}"; then
  echo "pr-go.yaml still depends on the Node.js setup-protoc action" >&2
  exit 1
fi

echo "Toolchain versions and the protoc installer match every build path."
