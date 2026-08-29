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

[[ "${schema_version}" == "1" ]] || release_die "unsupported candidate manifest schema: ${schema_version}"
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

candidate_values_contract="$(git show "${candidate_sha}:charts/hami-webui/values.yaml" | \
  yq -o=json 'del(.image.frontend.tag, .image.frontend.digest, .image.backend.tag, .image.backend.digest)' -)"
release_values_contract="$(yq -o=json 'del(.image.frontend.tag, .image.frontend.digest, .image.backend.tag, .image.backend.digest)' "${chart_dir}/values.yaml")"
[[ "${candidate_values_contract}" == "${release_values_contract}" ]] || \
  release_die "values.yaml changed beyond release image identities after the candidate build"

version="$(yq -r '.version' "${chart_dir}/Chart.yaml")"
app_version="$(yq -r '.appVersion' "${chart_dir}/Chart.yaml")"
frontend_repository="$(yq -r '.image.frontend.repository' "${chart_dir}/values.yaml")"
backend_repository="$(yq -r '.image.backend.repository' "${chart_dir}/values.yaml")"
frontend_tag="$(yq -r '.image.frontend.tag' "${chart_dir}/values.yaml")"
backend_tag="$(yq -r '.image.backend.tag' "${chart_dir}/values.yaml")"
frontend_digest="$(yq -r '.image.frontend.digest' "${chart_dir}/values.yaml")"
backend_digest="$(yq -r '.image.backend.digest' "${chart_dir}/values.yaml")"
expected_frontend_digest="$(jq -r '.images.frontend.dockerhub.digest' "${candidate_manifest}")"
expected_backend_digest="$(jq -r '.images.backend.dockerhub.digest' "${candidate_manifest}")"

[[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || \
  release_die "stable chart version must be x.y.z: ${version}"
[[ "${app_version}" == "${version}" ]] || \
  release_die "Chart.appVersion must equal Chart.version"
[[ "${frontend_repository}" == "projecthami/hami-webui-fe-oss" ]] || \
  release_die "unexpected default frontend repository: ${frontend_repository}"
[[ "${backend_repository}" == "projecthami/hami-webui-be-oss" ]] || \
  release_die "unexpected default backend repository: ${backend_repository}"
[[ "${frontend_tag}" == "v${version}" ]] || release_die "frontend tag must be v${version}"
[[ "${backend_tag}" == "v${version}" ]] || release_die "backend tag must be v${version}"
release_validate_digest "${frontend_digest}"
release_validate_digest "${backend_digest}"
[[ "${frontend_digest}" == "${expected_frontend_digest}" ]] || \
  release_die "frontend default digest does not match the candidate"
[[ "${backend_digest}" == "${expected_backend_digest}" ]] || \
  release_die "backend default digest does not match the candidate"

for component in frontend backend; do
  for registry in dockerhub ghcr; do
    image_repository="$(jq -r ".images.${component}.${registry}.repository" "${candidate_manifest}")"
    image_reference="$(jq -r ".images.${component}.${registry}.ref" "${candidate_manifest}")"
    image_digest="$(jq -r ".images.${component}.${registry}.digest" "${candidate_manifest}")"
    case "${component}/${registry}" in
      frontend/dockerhub) expected_repository="projecthami/hami-webui-fe-oss" ;;
      frontend/ghcr) expected_repository="ghcr.io/project-hami/hami-webui-fe-oss" ;;
      backend/dockerhub) expected_repository="projecthami/hami-webui-be-oss" ;;
      backend/ghcr) expected_repository="ghcr.io/project-hami/hami-webui-be-oss" ;;
    esac
    [[ "${image_repository}" == "${expected_repository}" ]] || \
      release_die "unexpected ${component}/${registry} repository: ${image_repository}"
    [[ "${image_reference}" == "${image_repository}:${candidate_tag}" ]] || \
      release_die "${component}/${registry} does not use the sealed candidate tag"
    release_validate_digest "${image_digest}"
  done
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
