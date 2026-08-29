#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

repository="${1:?GitHub repository is required}"
release_id="${2:?release id is required}"
expected_tag="${3:?release tag is required}"
expected_sha="${4:?release SHA is required}"
bundle_dir="${5:?release bundle directory is required}"
release_validate_commit "${expected_sha}"
release_require_command gh
release_require_command jq

release_json="$(gh api "repos/${repository}/releases/${release_id}")"
[[ "$(jq -r '.draft' <<<"${release_json}")" == "false" ]]
[[ "$(jq -r '.tag_name' <<<"${release_json}")" == "${expected_tag}" ]]
[[ "$(jq -r '.prerelease' <<<"${release_json}")" == "false" ]] || {
  echo "published GitHub Release is still marked as a prerelease" >&2
  exit 1
}
[[ "$(jq -r '.immutable' <<<"${release_json}")" == "true" ]] || {
  echo "stable contract failed: enable immutable releases and cut a new patch version" >&2
  exit 1
}

tag_object="$(gh api "repos/${repository}/git/ref/tags/${expected_tag}")"
tag_sha="$(jq -r '.object.sha' <<<"${tag_object}")"
tag_type="$(jq -r '.object.type' <<<"${tag_object}")"
if [[ "${tag_type}" == "tag" ]]; then
  annotated_tag="$(gh api "repos/${repository}/git/tags/${tag_sha}")"
  tag_sha="$(jq -r '.object.sha' <<<"${annotated_tag}")"
  tag_type="$(jq -r '.object.type' <<<"${annotated_tag}")"
fi
[[ "${tag_type}" == "commit" && "${tag_sha}" == "${expected_sha}" ]] || {
  echo "published tag does not resolve to the verified release commit" >&2
  exit 1
}

bash "${script_dir}/verify-github-release-assets.sh" \
  "${bundle_dir}" "${repository}" "${release_id}"

echo "Published immutable GitHub Release ${expected_tag} resolves to ${expected_sha}."
