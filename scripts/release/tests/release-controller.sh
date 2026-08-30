#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
release_dir="$(cd "${script_dir}/.." && pwd)"
fixture_dir="${script_dir}/fixtures"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

require_failure() {
  local description="$1"
  shift
  if "$@" >"${work_dir}/unexpected-success.log" 2>&1; then
    echo "expected failure: ${description}" >&2
    cat "${work_dir}/unexpected-success.log" >&2
    exit 1
  fi
}

verify_contract_in_repo() {
  local repository_dir="$1"
  local release_sha="$2"

  (
    cd "${repository_dir}"
    GITHUB_REPOSITORY=Project-HAMi/HAMi-WebUI \
      bash "${release_dir}/verify-release-contract.sh" candidate.json 42 "${release_sha}"
  )
}

digest_a="sha256:$(printf 'candidate manifest fixture' | sha256sum | awk '{print $1}')"
digest_b="sha256:$(printf 'conflicting stable fixture' | sha256sum | awk '{print $1}')"

# Candidate sealing calls a repository script, so its job must fetch the exact
# source commit with credentials disabled. Actionlint cannot infer this runtime
# dependency.
# shellcheck disable=SC2016
yq -e '
  [.jobs."candidate-manifest".steps[] |
    select(.uses == "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1") |
    select(.with.ref == "${{ needs.candidate-preflight.outputs.source_sha }}") |
    select(.with."persist-credentials" == false)] |
  length == 1
' .github/workflows/release.yaml >/dev/null

# Development and candidate publication must build the same unified target and
# publish only the two canonical registry references.
yq -e '
  [.jobs."publish-development-image".steps[] |
    select(.uses == "docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a") |
    select(.with.context == "." and .with.target == "unified") |
    select((.with.tags | split("\n") | map(select(. != "")) | sort | join(",")) ==
      "ghcr.io/project-hami/hami-webui:main,projecthami/hami-webui:main")] |
  length == 1
' .github/workflows/ci.yaml >/dev/null

# shellcheck disable=SC2016
yq -e '
  [.jobs."candidate-images".steps[] |
    select(.uses == "docker/build-push-action@53b7df96c91f9c12dcc8a07bcb9ccacbed38856a") |
    select(.with.context == "." and .with.target == "unified") |
    select((.with.tags | split("\n") | map(select(. != "")) | sort | join(",")) ==
      "ghcr.io/project-hami/hami-webui:${{ needs.candidate-preflight.outputs.candidate_tag }},projecthami/hami-webui:${{ needs.candidate-preflight.outputs.candidate_tag }}")] |
  length == 1
' .github/workflows/release.yaml >/dev/null

# Build a minimal two-commit repository so the contract test exercises the
# candidate-to-release comparison rather than relying on this checkout's history.
contract_repo="${work_dir}/contract-repo"
mkdir -p "${contract_repo}/charts/hami-webui"
cp "${fixture_dir}/Chart.yaml" "${fixture_dir}/Chart.lock" "${fixture_dir}/values.yaml" \
  "${contract_repo}/charts/hami-webui/"
git -C "${contract_repo}" init -q
git -C "${contract_repo}" config user.name release-test
git -C "${contract_repo}" config user.email release-test@example.invalid
git -C "${contract_repo}" add .
git -C "${contract_repo}" commit -qm candidate
candidate_sha="$(git -C "${contract_repo}" rev-parse HEAD)"

jq -n \
  --arg source_sha "${candidate_sha}" \
  --arg digest "${digest_a}" '
    {
      schemaVersion: 2,
      repository: "Project-HAMi/HAMi-WebUI",
      workflow: ".github/workflows/release.yaml",
      sourceSha: $source_sha,
      candidateTag: ("candidate-" + $source_sha[0:12] + "-42-1"),
      runId: "42",
      runAttempt: "1",
      images: {
        webui: {
          dockerhub: {repository: "projecthami/hami-webui", ref: ("projecthami/hami-webui:candidate-" + $source_sha[0:12] + "-42-1"), digest: $digest},
          ghcr: {repository: "ghcr.io/project-hami/hami-webui", ref: ("ghcr.io/project-hami/hami-webui:candidate-" + $source_sha[0:12] + "-42-1"), digest: $digest}
        }
      }
    }
  ' >"${contract_repo}/candidate.json"

# Exercise the exact candidate-sealing code used by the workflow. This catches
# jq compile errors and rejects inconsistent image metadata before release use.
candidate_parts="${work_dir}/candidate-parts"
mkdir -p "${candidate_parts}"
jq '{
  component: "webui",
  sourceSha,
  candidateTag,
  registries: .images.webui
}' "${contract_repo}/candidate.json" >"${candidate_parts}/webui.json"
bash "${release_dir}/seal-candidate-manifest.sh" \
  "${candidate_parts}" "${work_dir}/sealed-candidate.json" \
  Project-HAMi/HAMi-WebUI "${candidate_sha}" \
  "candidate-${candidate_sha:0:12}-42-1" 42 1
jq -S . "${contract_repo}/candidate.json" >"${work_dir}/expected-candidate.json"
jq -S . "${work_dir}/sealed-candidate.json" >"${work_dir}/actual-candidate.json"
cmp "${work_dir}/expected-candidate.json" "${work_dir}/actual-candidate.json"

cp -R "${candidate_parts}" "${work_dir}/bad-candidate-parts"
jq '.sourceSha = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"' \
  "${work_dir}/bad-candidate-parts/webui.json" >"${work_dir}/bad-webui.json"
mv "${work_dir}/bad-webui.json" "${work_dir}/bad-candidate-parts/webui.json"
require_failure "inconsistent candidate image metadata" \
  bash "${release_dir}/seal-candidate-manifest.sh" \
    "${work_dir}/bad-candidate-parts" "${work_dir}/bad-candidate.json" \
    Project-HAMi/HAMi-WebUI "${candidate_sha}" \
    "candidate-${candidate_sha:0:12}-42-1" 42 1

VERSION=2.0.1 DIGEST="${digest_a}" yq -i '
  .version = strenv(VERSION) |
  .appVersion = strenv(VERSION)
' "${contract_repo}/charts/hami-webui/Chart.yaml"
VERSION=2.0.1 DIGEST="${digest_a}" yq -i '
  .image.tag = "v" + strenv(VERSION) |
  .image.digest = strenv(DIGEST)
' "${contract_repo}/charts/hami-webui/values.yaml"
git -C "${contract_repo}" add charts
git -C "${contract_repo}" commit -qm 'allowed release metadata'
release_sha="$(git -C "${contract_repo}" rev-parse HEAD)"

(
  cd "${contract_repo}"
  GITHUB_REPOSITORY=Project-HAMi/HAMi-WebUI \
    bash "${release_dir}/verify-release-contract.sh" candidate.json 42 "${release_sha}"
)

# The single-image manifest is a new contract. A legacy schema must fail closed
# instead of being interpreted with missing or ambiguous image identities.
jq '.schemaVersion = 1' "${contract_repo}/candidate.json" \
  >"${contract_repo}/legacy-candidate.json"
(
  cd "${contract_repo}"
  require_failure "legacy candidate manifest schema" \
    env GITHUB_REPOSITORY=Project-HAMi/HAMi-WebUI \
    bash "${release_dir}/verify-release-contract.sh" legacy-candidate.json 42 "${release_sha}"
)

# The controller must not reinterpret a Chart 1.x two-container values shape as
# a partially populated unified image contract.
cp -R "${contract_repo}" "${work_dir}/legacy-chart-values"
yq -i '
  .image = {
    "frontend": {
      "repository": "projecthami/hami-webui-fe-oss",
      "tag": "v1.3.0",
      "digest": .image.digest
    },
    "backend": {
      "repository": "projecthami/hami-webui-be-oss",
      "tag": "v1.3.0",
      "digest": .image.digest
    }
  }
' "${work_dir}/legacy-chart-values/charts/hami-webui/values.yaml"
git -C "${work_dir}/legacy-chart-values" add charts/hami-webui/values.yaml
git -C "${work_dir}/legacy-chart-values" commit -qm 'legacy two-image values'
legacy_chart_sha="$(git -C "${work_dir}/legacy-chart-values" rev-parse HEAD)"
require_failure "Chart 1.x two-image values" \
  verify_contract_in_repo "${work_dir}/legacy-chart-values" "${legacy_chart_sha}"

# A root image object alone is not enough to hide a breaking Chart migration
# under a 1.x version number.
cp -R "${contract_repo}" "${work_dir}/legacy-chart-major"
VERSION=1.9.0 yq -i '
  .version = strenv(VERSION) |
  .appVersion = strenv(VERSION)
' "${work_dir}/legacy-chart-major/charts/hami-webui/Chart.yaml"
git -C "${work_dir}/legacy-chart-major" add charts/hami-webui/Chart.yaml
git -C "${work_dir}/legacy-chart-major" commit -qm 'legacy chart major'
legacy_major_sha="$(git -C "${work_dir}/legacy-chart-major" rev-parse HEAD)"
require_failure "Chart major version below 2" \
  verify_contract_in_repo "${work_dir}/legacy-chart-major" "${legacy_major_sha}"

# A non-release default under an allowed file path must still invalidate the
# already-built candidate.
cp -R "${contract_repo}" "${work_dir}/bad-values"
REPLICAS=2 yq -i '.replicaCount = env(REPLICAS)' \
  "${work_dir}/bad-values/charts/hami-webui/values.yaml"
git -C "${work_dir}/bad-values" add charts/hami-webui/values.yaml
git -C "${work_dir}/bad-values" commit -qm 'forbidden runtime default'
bad_values_sha="$(git -C "${work_dir}/bad-values" rev-parse HEAD)"
require_failure "values.yaml runtime change" \
  verify_contract_in_repo "${work_dir}/bad-values" "${bad_values_sha}"

# Chart dependency or other normalized Chart.yaml content must also fail.
cp -R "${contract_repo}" "${work_dir}/bad-chart"
DESCRIPTION=changed yq -i '.description = env(DESCRIPTION)' \
  "${work_dir}/bad-chart/charts/hami-webui/Chart.yaml"
git -C "${work_dir}/bad-chart" add charts/hami-webui/Chart.yaml
git -C "${work_dir}/bad-chart" commit -qm 'forbidden chart metadata'
bad_chart_sha="$(git -C "${work_dir}/bad-chart" rev-parse HEAD)"
require_failure "Chart.yaml non-version change" \
  verify_contract_in_repo "${work_dir}/bad-chart" "${bad_chart_sha}"

# Verify the canonical bundle accepts exact bytes and rejects tampering.
bundle="${work_dir}/bundle"
mkdir -p "${bundle}"
printf 'canonical chart fixture' >"${bundle}/hami-webui-9.9.9.tgz"
chart_sha="$(sha256sum "${bundle}/hami-webui-9.9.9.tgz" | awk '{print $1}')"
jq -n --arg chart_sha "${chart_sha}" '
  {release: {chart: {file: "hami-webui-9.9.9.tgz", sha256: $chart_sha}}}
' >"${bundle}/release-manifest.json"
(
  cd "${bundle}"
  {
    sha256sum hami-webui-9.9.9.tgz
    sha256sum release-manifest.json
  } >SHA256SUMS
)
bash "${release_dir}/verify-bundle.sh" "${bundle}"
cp -R "${bundle}" "${work_dir}/tampered-bundle"
printf 'tampered' >>"${work_dir}/tampered-bundle/hami-webui-9.9.9.tgz"
require_failure "tampered canonical bundle" \
  bash "${release_dir}/verify-bundle.sh" "${work_dir}/tampered-bundle"

# Exercise the all-destination preflight without any network writes. Command
# fixtures model two available registry manifests and absent stable targets.
preflight_bundle="${work_dir}/preflight-bundle"
pages="${work_dir}/pages"
mock_bin="${work_dir}/mock-bin"
mock_log="${work_dir}/mock.log"
mkdir -p "${preflight_bundle}" "${pages}" "${mock_bin}"
printf 'canonical chart fixture' >"${preflight_bundle}/hami-webui-9.9.9.tgz"
preflight_chart_sha="$(sha256sum "${preflight_bundle}/hami-webui-9.9.9.tgz" | awk '{print $1}')"
printf 'apiVersion: v1\nentries:\n  hami-webui: []\n' >"${pages}/index.yaml"
jq -n \
  --arg release_sha aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  --arg chart_sha "${preflight_chart_sha}" \
  --arg digest "${digest_a}" '
    {
      release: {
        sourceSha: $release_sha,
        version: "9.9.9",
        tag: "v9.9.9",
        chart: {file: "hami-webui-9.9.9.tgz", sha256: $chart_sha}
      },
      images: {
        webui: {
          dockerhub: {repository: "projecthami/hami-webui", digest: $digest},
          ghcr: {repository: "ghcr.io/project-hami/hami-webui", digest: $digest}
        }
      }
    }
  ' >"${preflight_bundle}/release-manifest.json"
(
  cd "${preflight_bundle}"
  {
    sha256sum hami-webui-9.9.9.tgz
    sha256sum release-manifest.json
  } >SHA256SUMS
)
export MOCK_PREFLIGHT_CHART_DIGEST="sha256:${preflight_chart_sha}"
MOCK_PREFLIGHT_SUMS_DIGEST="sha256:$(sha256sum "${preflight_bundle}/SHA256SUMS" | awk '{print $1}')"
MOCK_PREFLIGHT_MANIFEST_DIGEST="sha256:$(sha256sum "${preflight_bundle}/release-manifest.json" | awk '{print $1}')"
export MOCK_PREFLIGHT_SUMS_DIGEST MOCK_PREFLIGHT_MANIFEST_DIGEST

cat >"${mock_bin}/docker" <<'MOCK_DOCKER'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
reference="${*: -2:1}"
if [[ "${reference}" == *@sha256:* ]]; then
  printf 'candidate manifest fixture'
elif [[ "${MOCK_IMAGE_CONFLICT:-false}" == "true" ]]; then
  printf 'conflicting stable fixture'
else
  echo 'manifest not found' >&2
  exit 1
fi
MOCK_DOCKER
cat >"${mock_bin}/curl" <<'MOCK_CURL'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
if [[ "${MOCK_DOCKERHUB_MISSING:-false}" == "true" ]]; then
  echo 'curl: (22) The requested URL returned error: 404' >&2
  exit 22
fi
is_private=false
[[ "${MOCK_DOCKERHUB_PRIVATE:-false}" != "true" ]] || is_private=true
jq -n --argjson is_private "${is_private}" '
  {
    namespace: "projecthami",
    name: "hami-webui",
    is_private: $is_private,
    status: 1
  }
'
MOCK_CURL
cat >"${mock_bin}/helm" <<'MOCK_HELM'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
if [[ "${MOCK_OCI_CONFLICT:-false}" == "true" ]]; then
  destination=""
  while (($#)); do
    if [[ "$1" == "--destination" ]]; then
      destination="$2"
      break
    fi
    shift
  done
  [[ -n "${destination}" ]]
  printf 'conflicting OCI chart fixture' >"${destination}/hami-webui-9.9.9.tgz"
  exit 0
fi
echo 'chart not found' >&2
exit 1
MOCK_HELM
cat >"${mock_bin}/gh" <<'MOCK_GH'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
if [[ "$*" == *'/packages/container/hami-webui'* ]]; then
  [[ "${MOCK_WEBUI_PACKAGE_MISSING:-false}" != "true" ]] || {
    echo 'HTTP 404: not found' >&2
    exit 1
  }
  visibility="public"
  repository="Project-HAMi/HAMi-WebUI"
  [[ "${MOCK_WEBUI_PACKAGE_PRIVATE:-false}" != "true" ]] || visibility="private"
  [[ "${MOCK_WEBUI_PACKAGE_UNLINKED:-false}" != "true" ]] || repository=""
  jq -n \
    --arg visibility "${visibility}" \
    --arg repository "${repository}" '
      {
        name: "hami-webui",
        package_type: "container",
        visibility: $visibility,
        repository: {full_name: $repository}
      }
    '
elif [[ "$*" == *'/packages/container/charts%2Fhami-webui'* ]]; then
  visibility="public"
  repository="Project-HAMi/HAMi-WebUI"
  [[ "${MOCK_OCI_PACKAGE_PRIVATE:-false}" != "true" ]] || visibility="private"
  [[ "${MOCK_OCI_PACKAGE_UNLINKED:-false}" != "true" ]] || repository=""
  jq -n \
    --arg visibility "${visibility}" \
    --arg repository "${repository}" '
      {
        name: "charts/hami-webui",
        package_type: "container",
        visibility: $visibility,
        repository: {full_name: $repository}
      }
    '
elif [[ "$*" == *'/pages'* ]]; then
  build_type="workflow"
  [[ "${MOCK_PAGES_LEGACY:-false}" != "true" ]] || build_type="legacy"
  jq -n --arg build_type "${build_type}" '
    {
      build_type: $build_type,
      public: true,
      https_enforced: true,
      html_url: "https://project-hami.github.io/HAMi-WebUI/"
    }
  '
elif [[ "$*" == *'/git/ref/tags/'* ]]; then
  if [[ "${MOCK_RELEASE_STATE:-absent}" == published-* ]]; then
    jq -n '{object: {sha: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", type: "commit"}}'
  else
    echo 'HTTP 404: not found' >&2
    exit 1
  fi
elif [[ "$*" == *'/releases/7/assets'* ]]; then
  if [[ "${MOCK_RELEASE_STATE:-absent}" == "draft-extra" ]]; then
    jq -n '[{name: "unexpected.txt", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]'
  elif [[ "${MOCK_RELEASE_STATE:-absent}" == "draft-incomplete" ]]; then
    jq -n --arg digest "${MOCK_PREFLIGHT_CHART_DIGEST}" \
      '[{name: "hami-webui-9.9.9.tgz", digest: $digest}]'
  else
    jq -n \
      --arg chart "${MOCK_PREFLIGHT_CHART_DIGEST}" \
      --arg sums "${MOCK_PREFLIGHT_SUMS_DIGEST}" \
      --arg manifest "${MOCK_PREFLIGHT_MANIFEST_DIGEST}" '
        [
          {name: "hami-webui-9.9.9.tgz", digest: $chart},
          {name: "SHA256SUMS", digest: $sums},
          {name: "release-manifest.json", digest: $manifest}
        ]
      '
  fi
elif [[ "$*" == *'/releases?per_page=100'* ]]; then
  state="${MOCK_RELEASE_STATE:-absent}"
  if [[ "${state}" == "absent" ]]; then
    printf '[]\n'
  else
    draft=true
    prerelease=false
    immutable=false
    [[ "${state}" != published-* ]] || draft=false
    [[ "${state}" != "published-prerelease" ]] || prerelease=true
    [[ "${state}" != "published-exact" ]] || immutable=true
    jq -n \
      --argjson draft "${draft}" \
      --argjson prerelease "${prerelease}" \
      --argjson immutable "${immutable}" '
        [{
          id: 7,
          tag_name: "v9.9.9",
          target_commitish: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          draft: $draft,
          prerelease: $prerelease,
          immutable: $immutable
        }]
      '
  fi
else
  printf '[]\n'
fi
MOCK_GH
chmod +x "${mock_bin}/docker" "${mock_bin}/curl" "${mock_bin}/helm" "${mock_bin}/gh"

PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

require_failure "missing Docker Hub publication target" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_DOCKERHUB_MISSING=true \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

require_failure "private Docker Hub publication target" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_DOCKERHUB_PRIVATE=true \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

require_failure "missing GHCR publication target" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_WEBUI_PACKAGE_MISSING=true \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

require_failure "private GHCR publication target" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_WEBUI_PACKAGE_PRIVATE=true \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

require_failure "unlinked GHCR publication target" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_WEBUI_PACKAGE_UNLINKED=true \
  bash "${release_dir}/verify-image-publication-targets.sh" \
    Project-HAMi/HAMi-WebUI projecthami/hami-webui hami-webui

PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI
[[ "$(grep -c '@sha256:' "${mock_log}")" == "2" ]]
[[ "$(grep -c ':v9.9.9' "${mock_log}")" == "2" ]]

require_failure "private OCI parent package" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_OCI_PACKAGE_PRIVATE=true \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "OCI parent package linked to another repository" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_OCI_PACKAGE_UNLINKED=true \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "legacy Pages build mode" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_PAGES_LEGACY=true \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_RELEASE_STATE=draft-incomplete \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "draft release contains an unexpected asset" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_RELEASE_STATE=draft-extra \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "published release is mutable during preflight" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_RELEASE_STATE=published-mutable \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "published release is a prerelease during preflight" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_RELEASE_STATE=published-prerelease \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_RELEASE_STATE=published-exact \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

require_failure "conflicting stable image found by unified preflight" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_IMAGE_CONFLICT=true \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

# This is the late-destination regression guard: all image checks pass, then an
# existing OCI version with different bytes must abort before any publication.
require_failure "conflicting OCI chart found after all image checks" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" MOCK_OCI_CONFLICT=true \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

# A generated Pages branch may ignore chart archives globally. The publisher
# owns this validated artifact, so it must still commit the package and index
# together instead of failing at git add.
pages_chart_dir="${work_dir}/pages-chart"
pages_publish_dir="${work_dir}/pages-publish"
pages_remote_dir="${work_dir}/pages-remote.git"
mkdir -p "${pages_chart_dir}" "${pages_publish_dir}"
cat >"${pages_chart_dir}/Chart.yaml" <<'PAGES_CHART'
apiVersion: v2
name: hami-webui
type: application
version: 9.9.9
appVersion: "9.9.9"
PAGES_CHART
helm package "${pages_chart_dir}" --destination "${work_dir}" >/dev/null

git -C "${pages_publish_dir}" init -q -b gh-pages
git -C "${pages_publish_dir}" config user.name release-test
git -C "${pages_publish_dir}" config user.email release-test@example.invalid
printf '*.tgz\n' >"${pages_publish_dir}/.gitignore"
printf 'apiVersion: v1\nentries:\n  hami-webui: []\n' >"${pages_publish_dir}/index.yaml"
git -C "${pages_publish_dir}" add .gitignore index.yaml
git -C "${pages_publish_dir}" commit -qm 'initialize Pages source'
git init -q --bare "${pages_remote_dir}"
git -C "${pages_publish_dir}" remote add origin "${pages_remote_dir}"
git -C "${pages_publish_dir}" push -q -u origin gh-pages

GITHUB_TOKEN=test-token bash "${release_dir}/publish-pages.sh" \
  "${work_dir}/hami-webui-9.9.9.tgz" 9.9.9 "${pages_publish_dir}" \
  https://project-hami.github.io/HAMi-WebUI
git --git-dir="${pages_remote_dir}" show \
  gh-pages:hami-webui-9.9.9.tgz >"${work_dir}/published-pages-chart.tgz"
cmp "${work_dir}/hami-webui-9.9.9.tgz" "${work_dir}/published-pages-chart.tgz"
git --git-dir="${pages_remote_dir}" show gh-pages:index.yaml \
  >"${work_dir}/published-pages-index.yaml"
VERSION=9.9.9 yq -e '
  [.entries."hami-webui"[] | select(.version == strenv(VERSION))] | length == 1
' "${work_dir}/published-pages-index.yaml" >/dev/null
[[ -z "$(git -C "${pages_publish_dir}" status --short)" ]]
pages_publish_commit="$(git --git-dir="${pages_remote_dir}" rev-parse gh-pages)"
GITHUB_TOKEN=test-token bash "${release_dir}/publish-pages.sh" \
  "${work_dir}/hami-webui-9.9.9.tgz" 9.9.9 "${pages_publish_dir}" \
  https://project-hami.github.io/HAMi-WebUI
[[ "$(git --git-dir="${pages_remote_dir}" rev-parse gh-pages)" == "${pages_publish_commit}" ]]

# Pages must never accept only half of the immutable package/index pair.
cp "${preflight_bundle}/hami-webui-9.9.9.tgz" "${pages}/hami-webui-9.9.9.tgz"
require_failure "Pages package without matching index entry" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  bash "${release_dir}/preflight-publication.sh" \
    "${preflight_bundle}" Project-HAMi/HAMi-WebUI "${pages}" \
    https://project-hami.github.io/HAMi-WebUI

# The stable tag helper may create a missing tag once, but can never update a
# conflicting tag. Ruleset failures therefore happen before registry writes.
tag_state="${work_dir}/tag-state"
cat >"${mock_bin}/gh" <<'MOCK_TAG_GH'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
if [[ "$*" == *'/git/ref/tags/'* ]]; then
  if [[ -f "${MOCK_TAG_STATE}" ]]; then
    jq -n --arg sha "$(cat "${MOCK_TAG_STATE}")" \
      '{object: {sha: $sha, type: "commit"}}'
  else
    echo 'HTTP 404: not found' >&2
    exit 1
  fi
elif [[ "$*" == *'--method POST'* && "$*" == *'/git/refs'* ]]; then
  [[ "${MOCK_TAG_CREATE_FAIL:-false}" != "true" ]] || {
    echo 'HTTP 403: tag creation blocked' >&2
    exit 1
  }
  printf '%s\n' "${MOCK_TAG_SHA}" >"${MOCK_TAG_STATE}"
  printf '{}\n'
else
  echo "unexpected gh call: $*" >&2
  exit 1
fi
MOCK_TAG_GH
chmod +x "${mock_bin}/gh"

rm -f "${tag_state}"
: >"${mock_log}"
PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  MOCK_TAG_STATE="${tag_state}" MOCK_TAG_SHA="${release_sha}" \
  bash "${release_dir}/reserve-release-tag.sh" \
    Project-HAMi/HAMi-WebUI v2.0.1 "${release_sha}"
[[ "$(cat "${tag_state}")" == "${release_sha}" ]]
[[ "$(grep -c -- '--method POST' "${mock_log}")" == "1" ]]

: >"${mock_log}"
PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  MOCK_TAG_STATE="${tag_state}" MOCK_TAG_SHA="${release_sha}" \
  bash "${release_dir}/reserve-release-tag.sh" \
    Project-HAMi/HAMi-WebUI v2.0.1 "${release_sha}"
if grep -q -- '--method POST' "${mock_log}"; then
  echo "existing exact release tag was created again" >&2
  exit 1
fi

printf '%s\n' bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb >"${tag_state}"
require_failure "conflicting release tag" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  MOCK_TAG_STATE="${tag_state}" MOCK_TAG_SHA="${release_sha}" \
  bash "${release_dir}/reserve-release-tag.sh" \
    Project-HAMi/HAMi-WebUI v2.0.1 "${release_sha}"

rm -f "${tag_state}"
require_failure "release tag creation rejected by ruleset" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  MOCK_TAG_STATE="${tag_state}" MOCK_TAG_SHA="${release_sha}" \
  MOCK_TAG_CREATE_FAIL=true \
  bash "${release_dir}/reserve-release-tag.sh" \
    Project-HAMi/HAMi-WebUI v2.0.1 "${release_sha}"

# Recheck the tag and exact draft asset set immediately before the final
# publication mutation. A stale target_commitish is not sufficient when a tag
# already exists.
publish_bundle="${work_dir}/publish-bundle"
mkdir -p "${publish_bundle}"
cp "${preflight_bundle}/hami-webui-9.9.9.tgz" \
  "${preflight_bundle}/release-manifest.json" "${publish_bundle}/"
(
  cd "${publish_bundle}"
  {
    sha256sum hami-webui-9.9.9.tgz
    sha256sum release-manifest.json
  } >SHA256SUMS
)
publish_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
chart_digest="sha256:$(sha256sum "${publish_bundle}/hami-webui-9.9.9.tgz" | awk '{print $1}')"
sums_digest="sha256:$(sha256sum "${publish_bundle}/SHA256SUMS" | awk '{print $1}')"
manifest_digest="sha256:$(sha256sum "${publish_bundle}/release-manifest.json" | awk '{print $1}')"
cat >"${mock_bin}/gh" <<'MOCK_RELEASE_GH'
#!/usr/bin/env bash
set -euo pipefail
echo "$*" >>"${MOCK_LOG}"
if [[ "$*" == *'--method PATCH'* && "$*" == *'/releases/7'* ]]; then
  printf '{}\n'
elif [[ "$*" == *'/releases/7/assets'* ]]; then
  extra='[]'
  [[ "${MOCK_EXTRA_ASSET:-false}" != "true" ]] || \
    extra='[{"name":"unexpected","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]'
  chart_digest="${MOCK_CHART_DIGEST}"
  [[ "${MOCK_ASSET_TAMPER:-false}" != "true" ]] || \
    chart_digest=sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
  jq -n \
    --arg chart_digest "${chart_digest}" \
    --arg sums_digest "${MOCK_SUMS_DIGEST}" \
    --arg manifest_digest "${MOCK_MANIFEST_DIGEST}" \
    --argjson extra "${extra}" '
      [
        {name: "hami-webui-9.9.9.tgz", digest: $chart_digest},
        {name: "SHA256SUMS", digest: $sums_digest},
        {name: "release-manifest.json", digest: $manifest_digest}
      ] + $extra
    '
elif [[ "$*" == *'/releases/7'* ]]; then
  draft=true
  immutable=true
  prerelease=false
  [[ "${MOCK_ALREADY_PUBLISHED:-false}" != "true" ]] || draft=false
  [[ "${MOCK_RELEASE_MUTABLE:-false}" != "true" ]] || immutable=false
  [[ "${MOCK_RELEASE_PRERELEASE:-false}" != "true" ]] || prerelease=true
  jq -n \
    --arg sha "${MOCK_RELEASE_TARGET_SHA:-${MOCK_TAG_SHA}}" \
    --argjson draft "${draft}" \
    --argjson immutable "${immutable}" \
    --argjson prerelease "${prerelease}" '
      {
        tag_name: "v9.9.9",
        target_commitish: $sha,
        draft: $draft,
        immutable: $immutable,
        prerelease: $prerelease
      }
    '
elif [[ "$*" == *'/git/ref/tags/v9.9.9'* ]]; then
  jq -n --arg sha "${MOCK_TAG_SHA}" '{object: {sha: $sha, type: "commit"}}'
else
  echo "unexpected gh call: $*" >&2
  exit 1
fi
MOCK_RELEASE_GH
chmod +x "${mock_bin}/gh"

run_publish_check() {
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
    MOCK_TAG_SHA="${publish_sha}" \
    MOCK_RELEASE_TARGET_SHA="${publish_sha}" \
    MOCK_CHART_DIGEST="${chart_digest}" \
    MOCK_SUMS_DIGEST="${sums_digest}" \
    MOCK_MANIFEST_DIGEST="${manifest_digest}" \
    "$@"
}

: >"${mock_log}"
run_publish_check bash "${release_dir}/publish-github-release.sh" \
  Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"
[[ "$(grep -c -- '--method PATCH' "${mock_log}")" == "1" ]]

require_failure "release tag changed before final publication" \
  env PATH="${mock_bin}:${PATH}" MOCK_LOG="${mock_log}" \
  MOCK_TAG_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
  MOCK_RELEASE_TARGET_SHA="${publish_sha}" \
  MOCK_CHART_DIGEST="${chart_digest}" MOCK_SUMS_DIGEST="${sums_digest}" \
  MOCK_MANIFEST_DIGEST="${manifest_digest}" \
  bash "${release_dir}/publish-github-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

require_failure "draft asset changed before final publication" \
  run_publish_check env MOCK_ASSET_TAMPER=true \
  bash "${release_dir}/publish-github-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

require_failure "unexpected draft asset before final publication" \
  run_publish_check env MOCK_EXTRA_ASSET=true \
  bash "${release_dir}/publish-github-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

: >"${mock_log}"
run_publish_check env MOCK_ALREADY_PUBLISHED=true \
  bash "${release_dir}/publish-github-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"
if grep -q -- '--method PATCH' "${mock_log}"; then
  echo "already-published exact release was mutated again" >&2
  exit 1
fi

run_publish_check env MOCK_ALREADY_PUBLISHED=true \
  bash "${release_dir}/verify-published-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

require_failure "published release is mutable" \
  run_publish_check env MOCK_ALREADY_PUBLISHED=true MOCK_RELEASE_MUTABLE=true \
  bash "${release_dir}/verify-published-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

require_failure "published stable version is marked prerelease" \
  run_publish_check env MOCK_ALREADY_PUBLISHED=true MOCK_RELEASE_PRERELEASE=true \
  bash "${release_dir}/verify-published-release.sh" \
    Project-HAMi/HAMi-WebUI 7 v9.9.9 "${publish_sha}" "${publish_bundle}"

# Keep this variable used: the conflict digest documents that the mock emits a
# genuinely different manifest rather than relying only on a command failure.
[[ "${digest_a}" != "${digest_b}" ]]

echo "Release controller contract tests passed."
