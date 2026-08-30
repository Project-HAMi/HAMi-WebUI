#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

repository="${1:-Project-HAMi/HAMi-WebUI}"
dockerhub_repository="${2:-projecthami/hami-webui}"
ghcr_package="${3:-hami-webui}"

release_require_command curl
release_require_command gh
release_require_command jq

dockerhub_namespace="${dockerhub_repository%%/*}"
dockerhub_name="${dockerhub_repository#*/}"
[[ -n "${dockerhub_namespace}" && -n "${dockerhub_name}" && \
   "${dockerhub_repository}" == */* ]] || \
  release_die "Docker Hub repository must be namespace/name: ${dockerhub_repository}"

if ! dockerhub_record="$(curl --fail --silent --show-error \
  "https://hub.docker.com/v2/repositories/${dockerhub_namespace}/${dockerhub_name}/")"; then
  release_die "Docker Hub repository ${dockerhub_repository} is missing or unreadable"
fi
jq -e \
  --arg namespace "${dockerhub_namespace}" \
  --arg name "${dockerhub_name}" '
    .namespace == $namespace and
    .name == $name and
    .is_private == false and
    .status == 1
  ' <<<"${dockerhub_record}" >/dev/null || \
  release_die "Docker Hub repository ${dockerhub_repository} must be active and public"

github_owner="${repository%%/*}"
if ! ghcr_record="$(gh api "orgs/${github_owner}/packages/container/${ghcr_package}")"; then
  release_die "GHCR package ${github_owner}/${ghcr_package} is missing or unreadable"
fi
jq -e \
  --arg package "${ghcr_package}" \
  --arg repository "${repository}" '
    .name == $package and
    .package_type == "container" and
    .visibility == "public" and
    .repository.full_name == $repository
  ' <<<"${ghcr_record}" >/dev/null || \
  release_die "GHCR package ${github_owner}/${ghcr_package} must be public and linked to ${repository}"

echo "Unified image publication targets are public and correctly linked."
