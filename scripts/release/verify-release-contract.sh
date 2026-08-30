#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

candidate_manifest="${1:?candidate manifest is required}"
candidate_run_id="${2:?candidate run id is required}"
release_sha="${3:?release SHA is required}"
chart_dir="${4:-charts/hami-webui}"

release_require_command git
release_require_command jq
release_require_command yq
bash "${script_dir}/../verify-install-docs.sh" \
  "${chart_dir}" \
  "${script_dir}/../../docs/installation/helm/index.md"
release_validate_commit "${release_sha}"
[[ "${candidate_run_id}" =~ ^[0-9]+$ ]] || release_die "candidate run id must be numeric"
jq -e . "${candidate_manifest}" >/dev/null

schema_version="$(jq -r '.schemaVersion' "${candidate_manifest}")"
repository="$(jq -r '.repository' "${candidate_manifest}")"
workflow="$(jq -r '.workflow' "${candidate_manifest}")"
manifest_run_id="$(jq -r '.runId' "${candidate_manifest}")"
manifest_run_attempt="$(jq -r '.runAttempt' "${candidate_manifest}")"
candidate_sha="$(jq -r '.sourceSha' "${candidate_manifest}")"
candidate_tag="$(jq -r '.candidateTag' "${candidate_manifest}")"

[[ "${schema_version}" == "2" ]] || release_die "unsupported candidate manifest schema: ${schema_version}"
[[ "${repository}" == "${GITHUB_REPOSITORY:-Project-HAMi/HAMi-WebUI}" ]] || \
  release_die "candidate belongs to a different repository: ${repository}"
[[ "${workflow}" == ".github/workflows/release.yaml" ]] || \
  release_die "candidate was not built by the stable release controller"
[[ "${manifest_run_id}" == "${candidate_run_id}" ]] || \
  release_die "candidate run id does not match its manifest"
[[ "${manifest_run_attempt}" =~ ^[1-9][0-9]*$ ]] || \
  release_die "candidate run attempt must be a positive integer"
release_validate_commit "${candidate_sha}"
expected_candidate_tag="candidate-${candidate_sha:0:12}-${candidate_run_id}-${manifest_run_attempt}"
[[ "${candidate_tag}" == "${expected_candidate_tag}" ]] || \
  release_die "candidate tag does not bind the source, run, and attempt"
jq -e '
  (.images | keys) == ["webui"] and
  ((.images.webui | keys | sort) == ["dockerhub", "ghcr"])
' "${candidate_manifest}" >/dev/null || \
  release_die "candidate manifest must contain exactly one webui image for Docker Hub and GHCR"

git cat-file -e "${candidate_sha}^{commit}"
git cat-file -e "${release_sha}^{commit}"
git merge-base --is-ancestor "${candidate_sha}" "${release_sha}" || \
  release_die "candidate source is not an ancestor of the release commit"

while IFS= read -r changed_path; do
  case "${changed_path}" in
    charts/hami-webui/Chart.yaml | charts/hami-webui/values.yaml | charts/hami-webui/README.md | CHANGELOG.md | docs/releases/*)
      ;;
    *)
      release_die "non-release input changed after the candidate build: ${changed_path}"
      ;;
  esac
done < <(git diff --name-only "${candidate_sha}..${release_sha}")

# The two YAML files are release metadata only for the fields below. Allowing a
# whole-file path must not permit dependency, repository, or runtime defaults to
# change after the images were built.
candidate_chart_contract="$(git show "${candidate_sha}:charts/hami-webui/Chart.yaml" | \
  yq -o=json 'del(.version, .appVersion)' -)"
release_chart_contract="$(yq -o=json 'del(.version, .appVersion)' "${chart_dir}/Chart.yaml")"
[[ "${candidate_chart_contract}" == "${release_chart_contract}" ]] || \
  release_die "Chart.yaml changed beyond version/appVersion after the candidate build"

if ! yq -e '
  .image.repository != null and
  .image.frontend == null and
  .image.backend == null
' "${chart_dir}/values.yaml" >/dev/null; then
  release_die "Chart 1.x two-image values cannot be published by the unified-image controller"
fi

candidate_values_contract="$(git show "${candidate_sha}:charts/hami-webui/values.yaml" | \
  yq -o=json 'del(.image.tag, .image.digest)' -)"
release_values_contract="$(yq -o=json 'del(.image.tag, .image.digest)' "${chart_dir}/values.yaml")"
[[ "${candidate_values_contract}" == "${release_values_contract}" ]] || \
  release_die "values.yaml changed beyond release image identities after the candidate build"

version="$(yq -r '.version' "${chart_dir}/Chart.yaml")"
app_version="$(yq -r '.appVersion' "${chart_dir}/Chart.yaml")"
image_repository="$(yq -r '.image.repository' "${chart_dir}/values.yaml")"
image_tag="$(yq -r '.image.tag' "${chart_dir}/values.yaml")"
image_digest="$(yq -r '.image.digest' "${chart_dir}/values.yaml")"
expected_image_digest="$(jq -r '.images.webui.dockerhub.digest' "${candidate_manifest}")"

[[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || \
  release_die "stable chart version must be x.y.z: ${version}"
[[ "${version%%.*}" -ge 2 ]] || \
  release_die "the unified-image release contract requires Chart major version 2 or newer"
[[ "${app_version}" == "${version}" ]] || \
  release_die "Chart.appVersion must equal Chart.version"
[[ "${image_repository}" == "projecthami/hami-webui" ]] || \
  release_die "unexpected default image repository: ${image_repository}"
[[ "${image_tag}" == "v${version}" ]] || release_die "image tag must be v${version}"
release_validate_digest "${image_digest}"
[[ "${image_digest}" == "${expected_image_digest}" ]] || \
  release_die "default image digest does not match the candidate"

for registry in dockerhub ghcr; do
  registry_repository="$(jq -r ".images.webui.${registry}.repository" "${candidate_manifest}")"
  image_reference="$(jq -r ".images.webui.${registry}.ref" "${candidate_manifest}")"
  registry_digest="$(jq -r ".images.webui.${registry}.digest" "${candidate_manifest}")"
  case "${registry}" in
    dockerhub) expected_repository="projecthami/hami-webui" ;;
    ghcr) expected_repository="ghcr.io/project-hami/hami-webui" ;;
  esac
  [[ "${registry_repository}" == "${expected_repository}" ]] || \
    release_die "unexpected webui/${registry} repository: ${registry_repository}"
  [[ "${image_reference}" == "${registry_repository}:${candidate_tag}" ]] || \
    release_die "webui/${registry} does not use the sealed candidate tag"
  release_validate_digest "${registry_digest}"
done

[[ -f "${chart_dir}/Chart.lock" ]] || release_die "Chart.lock is required"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "version=${version}"
    echo "tag=v${version}"
    echo "candidate_source_sha=${candidate_sha}"
    echo "release_sha=${release_sha}"
  } >>"${GITHUB_OUTPUT}"
fi

echo "Release contract is valid for v${version}."
