#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

repository="${1:?GitHub repository is required}"
tag="${2:?release tag is required}"
target_sha="${3:?target SHA is required}"
release_validate_commit "${target_sha}"
[[ "${tag}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || release_die "invalid stable release tag: ${tag}"
release_require_command gh
release_require_command jq

resolve_tag() {
  local tag_record="$1"
  local annotated_tag
  local object_sha
  local object_type

  object_sha="$(jq -r '.object.sha' <<<"${tag_record}")"
  object_type="$(jq -r '.object.type' <<<"${tag_record}")"
  if [[ "${object_type}" == "tag" ]]; then
    annotated_tag="$(gh api "repos/${repository}/git/tags/${object_sha}")"
    object_sha="$(jq -r '.object.sha' <<<"${annotated_tag}")"
    object_type="$(jq -r '.object.type' <<<"${annotated_tag}")"
  fi
  [[ "${object_type}" == "commit" ]] || \
    release_die "Git tag ${tag} points to unsupported object type ${object_type}"
  printf '%s\n' "${object_sha}"
}

tag_error="$(mktemp)"
trap 'rm -f "${tag_error}"' EXIT
if tag_record="$(gh api "repos/${repository}/git/ref/tags/${tag}" 2>"${tag_error}")"; then
  observed_sha="$(resolve_tag "${tag_record}")"
  [[ "${observed_sha}" == "${target_sha}" ]] || \
    release_die "Git tag ${tag} already points to ${observed_sha}"
  echo "Git tag ${tag} already reserves ${target_sha}."
  exit 0
elif ! release_is_not_found_error "${tag_error}"; then
  cat "${tag_error}" >&2
  release_die "cannot determine whether Git tag ${tag} exists"
fi

# This is deliberately create-only. Repository rules grant GitHub Actions a
# bypass for tag creation, never for tag updates or deletion.
gh api --method POST "repos/${repository}/git/refs" \
  -f ref="refs/tags/${tag}" \
  -f sha="${target_sha}" >/dev/null

tag_record="$(gh api "repos/${repository}/git/ref/tags/${tag}")"
observed_sha="$(resolve_tag "${tag_record}")"
[[ "${observed_sha}" == "${target_sha}" ]] || \
  release_die "new Git tag ${tag} does not resolve to ${target_sha}"

echo "Git tag ${tag} now reserves ${target_sha}."
