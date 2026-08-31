#!/usr/bin/env bash

set -euo pipefail

readonly PROTOC_VERSION="28.3"
readonly PROTOC_SHA256_AMD64="0ad949f04a6a174da83cdcbdb36dee0a4925272a5b6d83f79a6bf9852076d53f"
readonly PROTOC_SHA256_ARM64="1de522032a8b194002fe35cab86d747848238b5e4de4f99648372079f5b46f9a"

install_prefix="${1:-/usr/local}"
architecture="${2:-$(uname -m)}"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "the pinned protoc installer supports Linux only" >&2
  exit 1
fi

case "${architecture}" in
  amd64 | x86_64)
    release_arch="x86_64"
    archive_sha256="${PROTOC_SHA256_AMD64}"
    ;;
  arm64 | aarch64 | aarch_64)
    release_arch="aarch_64"
    archive_sha256="${PROTOC_SHA256_ARM64}"
    ;;
  *)
    echo "unsupported protoc architecture: ${architecture}" >&2
    exit 1
    ;;
esac

for command in curl sha256sum unzip; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "required command not found: ${command}" >&2
    exit 1
  }
done

install -d "${install_prefix}"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
archive="${work_dir}/protoc.zip"

curl --fail --location --silent --show-error \
  --output "${archive}" \
  "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-linux-${release_arch}.zip"
printf '%s  %s\n' "${archive_sha256}" "${archive}" | sha256sum --check --status
unzip -q -o "${archive}" -d "${install_prefix}"

installed_version="$("${install_prefix}/bin/protoc" --version)"
if [[ "${installed_version}" != "libprotoc ${PROTOC_VERSION}" ]]; then
  echo "unexpected protoc version: ${installed_version}" >&2
  exit 1
fi

printf 'Installed protoc %s for %s in %s\n' \
  "${PROTOC_VERSION}" "${architecture}" "${install_prefix}"
