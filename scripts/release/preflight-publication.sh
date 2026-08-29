#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

bundle_dir="${1:?bundle directory is required}"
repository="${2:?GitHub repository is required}"
pages_dir="${3:?gh-pages checkout is required}"
pages_url="${4:?GitHub Pages base URL is required}"
manifest="${bundle_dir}/release-manifest.json"

release_require_command docker
release_require_command gh
release_require_command helm
release_require_command jq
release_require_command yq
[[ -f "${manifest}" ]] || release_die "release manifest is missing"

version="$(jq -r '.release.version' "${manifest}")"
tag="$(jq -r '.release.tag' "${manifest}")"
release_sha="$(jq -r '.release.sourceSha' "${manifest}")"
chart_file="$(jq -r '.release.chart.file' "${manifest}")"
chart_path="${bundle_dir}/${chart_file}"
chart_sha="$(jq -r '.release.chart.sha256' "${manifest}")"
release_validate_commit "${release_sha}"
[[ "$(release_sha256_file "${chart_path}")" == "${chart_sha}" ]] || \
  release_die "canonical chart changed before destination preflight"

# The first OCI push would create a private package by default. Require the
# package to be bootstrapped, public, and linked to this repository before any
# stable image or chart destination is touched.
owner="${repository%%/*}"
if ! package_record="$(gh api "orgs/${owner}/packages/container/charts%2Fhami-webui")"; then
  release_die "OCI package charts/hami-webui is missing or unreadable; run bootstrap-oci first"
fi
jq -e --arg repository "${repository}" '
  .name == "charts/hami-webui" and
  .package_type == "container" and
  .visibility == "public" and
  .repository.full_name == $repository
' <<<"${package_record}" >/dev/null || \
  release_die "OCI package must be public and linked to ${repository}"

# The controller explicitly deploys a Pages artifact. Legacy branch mode would
# accept the gh-pages commit but never publish this workflow's deployment.
if ! pages_record="$(gh api "repos/${repository}/pages")"; then
  release_die "cannot verify GitHub Pages configuration"
fi
expected_pages_url="${pages_url%/}/"
jq -e --arg url "${expected_pages_url}" '
  .build_type == "workflow" and
  .public == true and
  .https_enforced == true and
  .html_url == $url
' <<<"${pages_record}" >/dev/null || \
  release_die "GitHub Pages must be public, HTTPS-only, workflow-built, and served from ${expected_pages_url}"

inspect_optional_image() {
  local reference="$1"
  local expected_digest="$2"
  local raw_file
  local error_file
  local observed_digest
  raw_file="$(mktemp)"
  error_file="$(mktemp)"

  if docker buildx imagetools inspect "${reference}" --raw >"${raw_file}" 2>"${error_file}"; then
    observed_digest="sha256:$(release_sha256_file "${raw_file}")"
    [[ "${observed_digest}" == "${expected_digest}" ]] || \
      release_die "${reference} exists with ${observed_digest}, expected ${expected_digest}"
  elif ! release_is_not_found_error "${error_file}"; then
    cat "${error_file}" >&2
    release_die "cannot determine whether image ${reference} exists"
  fi
  rm -f "${raw_file}" "${error_file}"
}

# Inspect every source digest first so a missing later candidate cannot leave
# only a subset of stable image tags behind.
while IFS=$'\t' read -r image_repository image_digest; do
  release_validate_digest "${image_digest}"
  observed_digest="$(release_manifest_digest "${image_repository}@${image_digest}")" || \
    release_die "candidate image is unavailable: ${image_repository}@${image_digest}"
  [[ "${observed_digest}" == "${image_digest}" ]] || \
    release_die "candidate image digest changed: ${image_repository}"
  inspect_optional_image "${image_repository}:${tag}" "${image_digest}"
done < <(jq -r '.images | to_entries[].value | to_entries[].value | [.repository, .digest] | @tsv' "${manifest}")

# OCI: absence is acceptable; an existing version must pull back as the exact
# canonical tgz. Authentication was established by the caller.
oci_pull_dir="$(mktemp -d)"
oci_error="$(mktemp)"
if helm pull oci://ghcr.io/project-hami/charts/hami-webui \
    --version "${version}" --destination "${oci_pull_dir}" 2>"${oci_error}"; then
  [[ "$(release_sha256_file "${oci_pull_dir}/${chart_file}")" == "${chart_sha}" ]] || \
    release_die "OCI chart ${version} exists with different bytes"
elif ! release_is_not_found_error "${oci_error}"; then
  cat "${oci_error}" >&2
  release_die "cannot determine whether OCI chart ${version} exists"
fi
rm -rf "${oci_pull_dir}"
rm -f "${oci_error}"

# Classic Pages: package and index entry must be both absent or both exact.
pages_package="${pages_dir}/${chart_file}"
pages_index="${pages_dir}/index.yaml"
[[ -f "${pages_index}" ]] || release_die "gh-pages index.yaml is missing"
entry_count="$(VERSION="${version}" yq '[.entries."hami-webui"[] | select(.version == strenv(VERSION))] | length' "${pages_index}")"
[[ "${entry_count}" == "0" || "${entry_count}" == "1" ]] || \
  release_die "Pages index has duplicate ${version} entries"
if [[ -f "${pages_package}" ]]; then
  [[ "$(release_sha256_file "${pages_package}")" == "${chart_sha}" ]] || \
    release_die "Pages package ${chart_file} exists with different bytes"
  [[ "${entry_count}" == "1" ]] || release_die "Pages package exists without an index entry"
elif [[ "${entry_count}" == "1" ]]; then
  release_die "Pages index entry exists without its package"
fi
if [[ "${entry_count}" == "1" ]]; then
  indexed_digest="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .digest' "${pages_index}")"
  indexed_url="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .urls[0]' "${pages_index}")"
  [[ "${indexed_digest}" == "${chart_sha}" ]] || release_die "Pages index digest conflicts with the canonical chart"
  [[ "${indexed_url}" == "${pages_url}/${chart_file}" ]] || release_die "Pages index URL conflicts with the stable contract"
fi

# Git tag: absence or the exact release commit only.
tag_exists=false
tag_error="$(mktemp)"
if tag_record="$(gh api "repos/${repository}/git/ref/tags/${tag}" 2>"${tag_error}")"; then
  tag_exists=true
  tag_sha="$(jq -r '.object.sha' <<<"${tag_record}")"
  tag_type="$(jq -r '.object.type' <<<"${tag_record}")"
  if [[ "${tag_type}" == "tag" ]]; then
    tag_sha="$(gh api "repos/${repository}/git/tags/${tag_sha}" --jq '.object.sha')"
  fi
  [[ "${tag_sha}" == "${release_sha}" ]] || release_die "Git tag ${tag} points to ${tag_sha}"
elif ! release_is_not_found_error "${tag_error}"; then
  cat "${tag_error}" >&2
  release_die "cannot determine whether Git tag ${tag} exists"
fi
rm -f "${tag_error}"

# GitHub Release and all expected assets: absence is acceptable. Existing
# assets must be exact; a published release may not be repaired in place.
release_matches="$(gh api --paginate "repos/${repository}/releases?per_page=100" | \
  jq -s --arg tag "${tag}" '[.[][] | select(.tag_name == $tag)]')"
release_count="$(jq 'length' <<<"${release_matches}")"
[[ "${release_count}" == "0" || "${release_count}" == "1" ]] || \
  release_die "multiple GitHub Releases use ${tag}"
if [[ "${release_count}" == "1" ]]; then
  release_record="$(jq '.[0]' <<<"${release_matches}")"
  release_id="$(jq -r '.id' <<<"${release_record}")"
  release_target="$(jq -r '.target_commitish' <<<"${release_record}")"
  is_draft="$(jq -r '.draft' <<<"${release_record}")"
  is_prerelease="$(jq -r '.prerelease' <<<"${release_record}")"
  is_immutable="$(jq -r '.immutable // false' <<<"${release_record}")"
  [[ "${release_target}" == "${release_sha}" ]] || \
    release_die "GitHub Release ${tag} targets ${release_target}"
  [[ "${is_draft}" == "true" || "${tag_exists}" == "true" ]] || \
    release_die "published GitHub Release ${tag} has no Git tag"
  [[ "${is_draft}" == "true" || "${is_prerelease}" == "false" ]] || \
    release_die "published GitHub Release ${tag} is a prerelease"
  [[ "${is_draft}" == "true" || "${is_immutable}" == "true" ]] || \
    release_die "published GitHub Release ${tag} is mutable"

  assets_json="$(gh api "repos/${repository}/releases/${release_id}/assets?per_page=100")"
  expected_asset_names="$(jq -n \
    --arg chart "$(basename "${chart_path}")" \
    '[ $chart, "SHA256SUMS", "release-manifest.json" ]')"
  unknown_assets="$(jq --argjson expected "${expected_asset_names}" \
    '[.[] | select(.name as $name | ($expected | index($name)) == null)] | length' \
    <<<"${assets_json}")"
  [[ "${unknown_assets}" == "0" ]] || \
    release_die "GitHub Release ${tag} contains an unexpected asset"
  [[ "${is_draft}" == "true" || "$(jq 'length' <<<"${assets_json}")" == "3" ]] || \
    release_die "published GitHub Release ${tag} does not contain exactly three assets"

  for asset_path in "${chart_path}" "${bundle_dir}/SHA256SUMS" "${manifest}"; do
    asset_name="$(basename "${asset_path}")"
    expected_digest="sha256:$(release_sha256_file "${asset_path}")"
    asset_matches="$(jq --arg name "${asset_name}" '[.[] | select(.name == $name)]' <<<"${assets_json}")"
    asset_count="$(jq 'length' <<<"${asset_matches}")"
    [[ "${asset_count}" == "0" || "${asset_count}" == "1" ]] || \
      release_die "duplicate GitHub Release asset: ${asset_name}"
    if [[ "${asset_count}" == "0" ]]; then
      [[ "${is_draft}" == "true" ]] || \
        release_die "published GitHub Release ${tag} is missing ${asset_name}"
      continue
    fi
    existing_digest="$(jq -r '.[0].digest // empty' <<<"${asset_matches}")"
    if [[ -z "${existing_digest}" ]]; then
      downloaded_asset="$(mktemp)"
      asset_api_url="$(jq -r '.[0].url' <<<"${asset_matches}")"
      gh api -H 'Accept: application/octet-stream' "${asset_api_url}" >"${downloaded_asset}"
      existing_digest="sha256:$(release_sha256_file "${downloaded_asset}")"
      rm -f "${downloaded_asset}"
    fi
    [[ "${existing_digest}" == "${expected_digest}" ]] || \
      release_die "GitHub Release asset ${asset_name} conflicts with the canonical bundle"
  done
fi

echo "All stable destinations are absent or exactly match the verified release."
