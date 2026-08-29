#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

bundle_dir="${1:?release bundle directory is required}"
repository="${2:?GitHub repository is required}"
release_id="${3:?release id is required}"
manifest="${bundle_dir}/release-manifest.json"
release_require_command gh
release_require_command jq
[[ -f "${manifest}" ]] || release_die "release manifest is missing"

chart_path="${bundle_dir}/$(jq -r '.release.chart.file' "${manifest}")"
expected_assets=("${chart_path}" "${bundle_dir}/SHA256SUMS" "${manifest}")
for asset_path in "${expected_assets[@]}"; do
  [[ -f "${asset_path}" ]] || release_die "sealed release asset is missing: ${asset_path}"
done

assets_json="$(gh api "repos/${repository}/releases/${release_id}/assets?per_page=100")"
[[ "$(jq 'length' <<<"${assets_json}")" == "${#expected_assets[@]}" ]] || \
  release_die "release must contain exactly the sealed assets"
for asset_path in "${expected_assets[@]}"; do
  asset_name="$(basename "${asset_path}")"
  expected_digest="sha256:$(release_sha256_file "${asset_path}")"
  asset_matches="$(jq --arg name "${asset_name}" '[.[] | select(.name == $name)]' <<<"${assets_json}")"
  [[ "$(jq 'length' <<<"${asset_matches}")" == "1" ]] || \
    release_die "release asset set is missing or duplicates ${asset_name}"
  existing_digest="$(jq -r '.[0].digest // empty' <<<"${asset_matches}")"
  if [[ -z "${existing_digest}" ]]; then
    downloaded_asset="$(mktemp)"
    asset_api_url="$(jq -r '.[0].url' <<<"${asset_matches}")"
    gh api -H 'Accept: application/octet-stream' "${asset_api_url}" >"${downloaded_asset}"
    existing_digest="sha256:$(release_sha256_file "${downloaded_asset}")"
    rm -f "${downloaded_asset}"
  fi
  [[ "${existing_digest}" == "${expected_digest}" ]] || \
    release_die "release asset ${asset_name} differs from the sealed bundle"
done

echo "GitHub Release contains exactly the three sealed assets."
