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
manifest="${bundle_dir}/release-manifest.json"
release_validate_commit "${expected_sha}"
release_require_command gh
release_require_command jq
[[ -f "${manifest}" ]] || release_die "release manifest is missing"

release_json="$(gh api "repos/${repository}/releases/${release_id}")"
[[ "$(jq -r '.tag_name' <<<"${release_json}")" == "${expected_tag}" ]] || {
  echo "release tag changed before publication" >&2
  exit 1
}
[[ "$(jq -r '.target_commitish' <<<"${release_json}")" == "${expected_sha}" ]] || {
  echo "release target changed before publication" >&2
  exit 1
}

# For an existing tag, GitHub ignores target_commitish. Resolve the ref again at
# the last possible moment so publishing cannot make the wrong commit immutable.
tag_object="$(gh api "repos/${repository}/git/ref/tags/${expected_tag}")"
tag_sha="$(jq -r '.object.sha' <<<"${tag_object}")"
tag_type="$(jq -r '.object.type' <<<"${tag_object}")"
if [[ "${tag_type}" == "tag" ]]; then
  annotated_tag="$(gh api "repos/${repository}/git/tags/${tag_sha}")"
  tag_sha="$(jq -r '.object.sha' <<<"${annotated_tag}")"
  tag_type="$(jq -r '.object.type' <<<"${annotated_tag}")"
fi
[[ "${tag_type}" == "commit" && "${tag_sha}" == "${expected_sha}" ]] || \
  release_die "release tag no longer resolves to the verified commit"

# A draft remains mutable until publication. Recheck its exact asset set and
# bytes immediately before the final PATCH.
bash "${script_dir}/verify-github-release-assets.sh" \
  "${bundle_dir}" "${repository}" "${release_id}"

if [[ "$(jq -r '.draft' <<<"${release_json}")" == "true" ]]; then
  # This is intentionally the final mutation in the stable release workflow.
  gh api --method PATCH "repos/${repository}/releases/${release_id}" \
    -F draft=false \
    -F prerelease=false \
    -f make_latest=true >/dev/null
else
  echo "GitHub Release ${expected_tag} is already published; publication is a no-op."
fi
