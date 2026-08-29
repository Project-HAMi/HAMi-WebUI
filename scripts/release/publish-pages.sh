#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

chart_path="${1:?chart package is required}"
version="${2:?chart version is required}"
pages_dir="${3:?gh-pages checkout is required}"
base_url="${4:?GitHub Pages base URL is required}"
chart_name="hami-webui"
chart_file="${chart_name}-${version}.tgz"
expected_sha="$(release_sha256_file "${chart_path}")"
expected_url="${base_url}/${chart_file}"
index_path="${pages_dir}/index.yaml"
published_path="${pages_dir}/${chart_file}"

release_require_command git
release_require_command helm
release_require_command yq
[[ -f "${index_path}" ]] || release_die "gh-pages index.yaml is missing"

entry_count="$(VERSION="${version}" yq '[.entries."hami-webui"[] | select(.version == strenv(VERSION))] | length' "${index_path}")"
[[ "${entry_count}" == "0" || "${entry_count}" == "1" ]] || \
  release_die "index contains duplicate entries for ${version}"

if [[ -f "${published_path}" ]]; then
  [[ "$(release_sha256_file "${published_path}")" == "${expected_sha}" ]] || \
    release_die "Pages package ${chart_file} already exists with different bytes"
elif [[ "${entry_count}" == "1" ]]; then
  release_die "index contains ${version}, but its Pages package is missing"
else
  cp "${chart_path}" "${published_path}"
fi

if [[ "${entry_count}" == "1" ]]; then
  indexed_digest="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .digest' "${index_path}")"
  indexed_url="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .urls[0]' "${index_path}")"
  [[ "${indexed_digest}" == "${expected_sha}" ]] || \
    release_die "index digest for ${version} differs from the canonical package"
  [[ "${indexed_url}" == "${expected_url}" ]] || \
    release_die "index URL for ${version} is not the immutable Pages package"
else
  staging_dir="$(mktemp -d)"
  trap 'rm -rf "${staging_dir}"' EXIT
  cp "${chart_path}" "${staging_dir}/${chart_file}"
  helm repo index "${staging_dir}" --url "${base_url}" --merge "${index_path}"
  cp "${staging_dir}/index.yaml" "${index_path}"

  indexed_digest="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .digest' "${index_path}")"
  indexed_url="$(VERSION="${version}" yq -r '.entries."hami-webui"[] | select(.version == strenv(VERSION)) | .urls[0]' "${index_path}")"
  [[ "${indexed_digest}" == "${expected_sha}" ]] || release_die "generated index digest is incorrect"
  [[ "${indexed_url}" == "${expected_url}" ]] || release_die "generated index URL is incorrect"
fi

git -C "${pages_dir}" add --force -- "${chart_file}"
git -C "${pages_dir}" add -- index.yaml

if ! git -C "${pages_dir}" diff --cached --quiet -- "${chart_file}" index.yaml; then
  git -C "${pages_dir}" commit -m "release(chart): publish ${version}"
  [[ -n "${GITHUB_TOKEN:-}" ]] || release_die "GITHUB_TOKEN is required to publish gh-pages"
  auth_header="$(printf 'x-access-token:%s' "${GITHUB_TOKEN}" | base64 -w 0)"
  git -C "${pages_dir}" \
    -c "http.https://github.com/.extraheader=AUTHORIZATION: basic ${auth_header}" \
    push origin HEAD:gh-pages
else
  echo "Pages package and index are already correct."
fi

echo "Pages source contains ${expected_sha}; an explicit Pages deployment is still required."
