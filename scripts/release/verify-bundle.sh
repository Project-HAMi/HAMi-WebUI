#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

bundle_dir="${1:?bundle directory is required}"
release_require_command jq

[[ -f "${bundle_dir}/SHA256SUMS" ]] || release_die "SHA256SUMS is missing"
[[ -f "${bundle_dir}/release-manifest.json" ]] || release_die "release-manifest.json is missing"
(
  cd "${bundle_dir}"
  sha256sum --check SHA256SUMS
)

chart_file="$(jq -r '.release.chart.file' "${bundle_dir}/release-manifest.json")"
expected_sha="$(jq -r '.release.chart.sha256' "${bundle_dir}/release-manifest.json")"
[[ -f "${bundle_dir}/${chart_file}" ]] || release_die "canonical chart is missing"
[[ "$(release_sha256_file "${bundle_dir}/${chart_file}")" == "${expected_sha}" ]] || \
  release_die "canonical chart hash does not match the release manifest"

echo "Release bundle hashes are valid."
