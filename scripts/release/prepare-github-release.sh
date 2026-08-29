#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

bundle_dir="${1:?bundle directory is required}"
repository="${2:?GitHub repository is required}"
tag="${3:?release tag is required}"
target_sha="${4:?target SHA is required}"
release_validate_commit "${target_sha}"
release_require_command gh
release_require_command jq

tag_error="$(mktemp)"
if tag_record="$(gh api "repos/${repository}/git/ref/tags/${tag}" 2>"${tag_error}")"; then
  tag_sha="$(jq -r '.object.sha' <<<"${tag_record}")"
  tag_type="$(jq -r '.object.type' <<<"${tag_record}")"
  if [[ "${tag_type}" == "tag" ]]; then
    tag_sha="$(gh api "repos/${repository}/git/tags/${tag_sha}" --jq '.object.sha')"
  fi
  [[ "${tag_sha}" == "${target_sha}" ]] || \
    release_die "Git tag ${tag} already points to ${tag_sha}"
elif ! release_is_not_found_error "${tag_error}"; then
  cat "${tag_error}" >&2
  release_die "cannot determine whether Git tag ${tag} exists"
fi
rm -f "${tag_error}"

release_json="$(gh api --paginate "repos/${repository}/releases?per_page=100" | \
  jq -s --arg tag "${tag}" '[.[][] | select(.tag_name == $tag)]')"
release_count="$(jq 'length' <<<"${release_json}")"
[[ "${release_count}" == "0" || "${release_count}" == "1" ]] || \
  release_die "multiple GitHub Releases use ${tag}"

if [[ "${release_count}" == "0" ]]; then
  release_record="$(gh api --method POST "repos/${repository}/releases" \
    -f tag_name="${tag}" \
    -f target_commitish="${target_sha}" \
    -f name="${tag}" \
    -F draft=true \
    -F prerelease=false \
    -F generate_release_notes=true)"
else
  release_record="$(jq '.[0]' <<<"${release_json}")"
  existing_target="$(jq -r '.target_commitish' <<<"${release_record}")"
  [[ "${existing_target}" == "${target_sha}" ]] || \
    release_die "GitHub Release ${tag} targets ${existing_target}, not ${target_sha}"
fi

release_id="$(jq -r '.id' <<<"${release_record}")"
is_draft="$(jq -r '.draft' <<<"${release_record}")"

for asset_path in \
  "${bundle_dir}/$(jq -r '.release.chart.file' "${bundle_dir}/release-manifest.json")" \
  "${bundle_dir}/SHA256SUMS" \
  "${bundle_dir}/release-manifest.json"; do
  asset_name="$(basename "${asset_path}")"
  expected_digest="sha256:$(release_sha256_file "${asset_path}")"
  asset_json="$(gh api "repos/${repository}/releases/${release_id}/assets?per_page=100" | \
    jq --arg name "${asset_name}" '[.[] | select(.name == $name)]')"
  asset_count="$(jq 'length' <<<"${asset_json}")"
  [[ "${asset_count}" == "0" || "${asset_count}" == "1" ]] || \
    release_die "duplicate GitHub Release asset: ${asset_name}"

  if [[ "${asset_count}" == "1" ]]; then
    existing_digest="$(jq -r '.[0].digest // empty' <<<"${asset_json}")"
    if [[ -z "${existing_digest}" ]]; then
      download_path="$(mktemp)"
      asset_api_url="$(jq -r '.[0].url' <<<"${asset_json}")"
      gh api -H 'Accept: application/octet-stream' "${asset_api_url}" >"${download_path}"
      existing_digest="sha256:$(release_sha256_file "${download_path}")"
      rm -f "${download_path}"
    fi
    [[ "${existing_digest}" == "${expected_digest}" ]] || \
      release_die "GitHub Release asset ${asset_name} exists with different bytes"
    continue
  fi

  [[ "${is_draft}" == "true" ]] || \
    release_die "refusing to add a missing asset to an already-published release"
  upload_url="$(jq -r '.upload_url' <<<"${release_record}" | sed 's/{.*$//')"
  gh api --method POST \
    -H 'Content-Type: application/octet-stream' \
    "${upload_url}?name=${asset_name}" \
    --input "${asset_path}" >/dev/null
done

# Finish the mutable draft checkpoint before any tag, registry, or Pages write.
# The same check runs again immediately before publication to catch races.
bash "${script_dir}/verify-github-release-assets.sh" \
  "${bundle_dir}" "${repository}" "${release_id}"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "release_id=${release_id}"
    echo "already_published=$([[ "${is_draft}" == "true" ]] && echo false || echo true)"
  } >>"${GITHUB_OUTPUT}"
fi

echo "GitHub Release ${tag} is staged with verified assets (draft=${is_draft})."
