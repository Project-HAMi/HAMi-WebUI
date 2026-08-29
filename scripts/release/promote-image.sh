#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

source_repository="${1:?source repository is required}"
source_digest="${2:?source digest is required}"
target_reference="${3:?target reference is required}"
release_validate_digest "${source_digest}"

source_reference="${source_repository}@${source_digest}"
observed_source_digest="$(release_manifest_digest "${source_reference}")" || \
  release_die "cannot inspect candidate image: ${source_reference}"
[[ "${observed_source_digest}" == "${source_digest}" ]] || \
  release_die "candidate image digest changed: ${source_reference}"

error_file="$(mktemp)"
raw_file="$(mktemp)"
trap 'rm -f "${error_file}" "${raw_file}"' EXIT
if docker buildx imagetools inspect "${target_reference}" --raw >"${raw_file}" 2>"${error_file}"; then
  target_digest="sha256:$(release_sha256_file "${raw_file}")"
  [[ "${target_digest}" == "${source_digest}" ]] || \
    release_die "refusing to replace ${target_reference}: it points to ${target_digest}"
  echo "Image tag already points to the verified digest: ${target_reference}"
  exit 0
fi

release_is_not_found_error "${error_file}" || {
  cat "${error_file}" >&2
  release_die "cannot determine whether ${target_reference} exists"
}

docker buildx imagetools create --tag "${target_reference}" "${source_reference}"
target_digest="$(release_manifest_digest "${target_reference}")" || \
  release_die "cannot inspect promoted image: ${target_reference}"
[[ "${target_digest}" == "${source_digest}" ]] || \
  release_die "promotion changed the manifest digest for ${target_reference}"
echo "Promoted ${target_reference} to ${source_digest}."
