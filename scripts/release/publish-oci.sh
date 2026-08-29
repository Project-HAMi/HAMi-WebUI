#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

chart_path="${1:?chart package is required}"
version="${2:?chart version is required}"
oci_parent="${3:-oci://ghcr.io/project-hami/charts}"
chart_name="hami-webui"
expected_sha="$(release_sha256_file "${chart_path}")"
pull_dir="$(mktemp -d)"
error_file="$(mktemp)"
trap 'rm -rf "${pull_dir}"; rm -f "${error_file}"' EXIT

pull_chart() {
  local destination="$1"
  helm pull "${oci_parent}/${chart_name}" --version "${version}" --destination "${destination}"
}

if pull_chart "${pull_dir}" 2>"${error_file}"; then
  existing_path="${pull_dir}/${chart_name}-${version}.tgz"
  [[ "$(release_sha256_file "${existing_path}")" == "${expected_sha}" ]] || \
    release_die "OCI ${chart_name}:${version} exists with different bytes"
  echo "OCI chart already contains the canonical package."
else
  release_is_not_found_error "${error_file}" || {
    cat "${error_file}" >&2
    release_die "cannot determine whether OCI chart ${version} exists"
  }
  helm push "${chart_path}" "${oci_parent}"
fi

rm -rf "${pull_dir}"
pull_dir="$(mktemp -d)"
pull_chart "${pull_dir}"
pulled_path="${pull_dir}/${chart_name}-${version}.tgz"
[[ "$(release_sha256_file "${pulled_path}")" == "${expected_sha}" ]] || \
  release_die "authenticated OCI pull-back hash differs from the canonical package"

# A stable chart must be usable without the workflow's registry credentials.
public_registry_config="${pull_dir}/public-registry.json"
printf '{}\n' >"${public_registry_config}"
public_pull_dir="${pull_dir}/public"
mkdir -p "${public_pull_dir}"
HELM_REGISTRY_CONFIG="${public_registry_config}" \
  helm pull "${oci_parent}/${chart_name}" --version "${version}" --destination "${public_pull_dir}"
[[ "$(release_sha256_file "${public_pull_dir}/${chart_name}-${version}.tgz")" == "${expected_sha}" ]] || \
  release_die "public OCI pull-back hash differs from the canonical package"

echo "OCI chart ${version} is public and byte-identical to the canonical package."
