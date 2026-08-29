#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

component="${1:?component is required}"
source_sha="${2:?source SHA is required}"
candidate_tag="${3:?candidate tag is required}"
dockerhub_repository="${4:?Docker Hub repository is required}"
ghcr_repository="${5:?GHCR repository is required}"
output_path="${6:?output path is required}"

release_validate_commit "${source_sha}"
[[ "${component}" == "frontend" || "${component}" == "backend" ]] || \
  release_die "unexpected image component: ${component}"
[[ "${candidate_tag}" == candidate-* ]] || release_die "candidate tag must start with candidate-"

inspect_image() {
  local registry_name="$1"
  local repository="$2"
  local reference="${repository}:${candidate_tag}"
  local raw_file
  local digest
  raw_file="$(mktemp)"

  docker buildx imagetools inspect "${reference}" --raw >"${raw_file}"
  jq -e '
    any(.manifests[]?; .platform.os == "linux" and .platform.architecture == "amd64") and
    any(.manifests[]?; .platform.os == "linux" and .platform.architecture == "arm64")
  ' "${raw_file}" >/dev/null || release_die "${reference} is not a linux/amd64 + linux/arm64 index"
  digest="sha256:$(release_sha256_file "${raw_file}")"
  release_validate_digest "${digest}"
  jq -n \
    --arg registry "${registry_name}" \
    --arg repository "${repository}" \
    --arg ref "${reference}" \
    --arg digest "${digest}" \
    '{registry: $registry, repository: $repository, ref: $ref, digest: $digest}'
  rm -f "${raw_file}"
}

dockerhub_json="$(inspect_image dockerhub "${dockerhub_repository}")"
ghcr_json="$(inspect_image ghcr "${ghcr_repository}")"
mkdir -p "$(dirname "${output_path}")"
jq -n \
  --arg component "${component}" \
  --arg source_sha "${source_sha}" \
  --arg candidate_tag "${candidate_tag}" \
  --argjson dockerhub "${dockerhub_json}" \
  --argjson ghcr "${ghcr_json}" \
  '{
    component: $component,
    sourceSha: $source_sha,
    candidateTag: $candidate_tag,
    registries: {dockerhub: $dockerhub, ghcr: $ghcr}
  }' >"${output_path}"

echo "Recorded ${component} candidate digests."
