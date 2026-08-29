#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

chart_path="${1:?chart package is required}"
version="${2:?chart version is required}"
base_url="${3:?GitHub Pages base URL is required}"
chart_file="hami-webui-${version}.tgz"
expected_sha="$(release_sha256_file "${chart_path}")"
expected_url="${base_url}/${chart_file}"
download_dir="$(mktemp -d)"
trap 'rm -rf "${download_dir}"' EXIT

verified=false
for _ in $(seq 1 30); do
  if curl --fail --location --silent --show-error \
      --header 'Cache-Control: no-cache' \
      "${expected_url}?release=${version}" \
      --output "${download_dir}/${chart_file}" && \
     [[ "$(release_sha256_file "${download_dir}/${chart_file}")" == "${expected_sha}" ]] && \
     curl --fail --location --silent --show-error \
      --header 'Cache-Control: no-cache' \
      "${base_url}/index.yaml?release=${version}" \
      --output "${download_dir}/index.yaml" && \
     [[ "$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .digest' "${download_dir}/index.yaml")" == "${expected_sha}" ]]; then
    verified=true
    break
  fi
  sleep 10
done
[[ "${verified}" == "true" ]] || \
  release_die "GitHub Pages did not serve the verified chart within five minutes"

echo "Pages package and index publicly serve ${expected_sha}."
