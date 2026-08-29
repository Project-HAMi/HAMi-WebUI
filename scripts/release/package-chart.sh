#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

candidate_manifest="${1:?candidate manifest is required}"
release_sha="${2:?release SHA is required}"
output_dir="${3:?output directory is required}"
chart_dir="${4:-charts/hami-webui}"

release_require_command helm
release_require_command jq
release_require_command yq
release_validate_commit "${release_sha}"

[[ ! -e "${output_dir}" ]] || release_die "output directory already exists: ${output_dir}"
mkdir -p "${output_dir}"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
cp -R "${chart_dir}" "${work_dir}/hami-webui"

lock_before="$(release_sha256_file "${work_dir}/hami-webui/Chart.lock")"
helm dependency build --skip-refresh "${work_dir}/hami-webui"
lock_after="$(release_sha256_file "${work_dir}/hami-webui/Chart.lock")"
[[ "${lock_before}" == "${lock_after}" ]] || \
  release_die "helm dependency build changed Chart.lock"

helm lint "${work_dir}/hami-webui"
helm template release-verification "${work_dir}/hami-webui" >/dev/null
helm package "${work_dir}/hami-webui" --destination "${output_dir}"

version="$(yq -r '.version' "${work_dir}/hami-webui/Chart.yaml")"
chart_name="hami-webui-${version}.tgz"
chart_path="${output_dir}/${chart_name}"
[[ -f "${chart_path}" ]] || release_die "expected chart package was not produced: ${chart_name}"
[[ "$(find "${output_dir}" -maxdepth 1 -name '*.tgz' -type f | wc -l | tr -d ' ')" == "1" ]] || \
  release_die "chart must be packaged exactly once"

chart_sha="$(release_sha256_file "${chart_path}")"
dependency_json='[]'
for dependency in "${work_dir}/hami-webui"/charts/*.tgz; do
  [[ -f "${dependency}" ]] || release_die "locked dependency archives were not built"
  dependency_json="$(jq \
    --arg name "$(basename "${dependency}")" \
    --arg sha256 "$(release_sha256_file "${dependency}")" \
    '. + [{name: $name, sha256: $sha256}]' <<<"${dependency_json}")"
done

jq \
  --arg release_sha "${release_sha}" \
  --arg version "${version}" \
  --arg tag "v${version}" \
  --arg chart_file "${chart_name}" \
  --arg chart_sha256 "${chart_sha}" \
  --arg chart_lock_sha256 "${lock_after}" \
  --arg helm_version "$(helm version --short)" \
  --argjson dependencies "${dependency_json}" \
  '. + {
    release: {
      sourceSha: $release_sha,
      version: $version,
      tag: $tag,
      chart: {
        file: $chart_file,
        sha256: $chart_sha256,
        chartLockSha256: $chart_lock_sha256,
        dependencies: $dependencies,
        helmVersion: $helm_version
      }
    }
  }' "${candidate_manifest}" >"${output_dir}/release-manifest.json"

(
  cd "${output_dir}"
  {
    printf '%s  %s\n' "${chart_sha}" "${chart_name}"
    printf '%s  %s\n' "$(release_sha256_file release-manifest.json)" "release-manifest.json"
  } >SHA256SUMS
  sha256sum --check SHA256SUMS
)

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "version=${version}"
    echo "tag=v${version}"
    echo "chart_file=${chart_name}"
    echo "chart_sha256=${chart_sha}"
  } >>"${GITHUB_OUTPUT}"
fi

echo "Canonical chart package created: ${chart_name} (${chart_sha})"
