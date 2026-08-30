#!/usr/bin/env bash

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/release/lib.sh
source "${script_dir}/lib.sh"

parts_dir="${1:?candidate parts directory is required}"
output_path="${2:?candidate manifest output path is required}"
repository="${3:?GitHub repository is required}"
source_sha="${4:?source SHA is required}"
candidate_tag="${5:?candidate tag is required}"
run_id="${6:?candidate run ID is required}"
run_attempt="${7:?candidate run attempt is required}"

release_require_command jq
release_validate_commit "${source_sha}"
[[ "${run_id}" =~ ^[0-9]+$ ]] || release_die "candidate run ID must be numeric"
[[ "${run_attempt}" =~ ^[0-9]+$ ]] || release_die "candidate run attempt must be numeric"
expected_tag="candidate-${source_sha:0:12}-${run_id}-${run_attempt}"
[[ "${candidate_tag}" == "${expected_tag}" ]] || \
  release_die "candidate tag does not bind the source, run, and attempt"
[[ -f "${parts_dir}/webui.json" ]] || \
  release_die "unified webui candidate metadata is required"
[[ "$(find "${parts_dir}" -maxdepth 1 -name '*.json' -type f | wc -l | tr -d ' ')" == "1" ]] || \
  release_die "candidate metadata must contain exactly one JSON part"

jq -e -s \
  --arg repository "${repository}" \
  --arg source_sha "${source_sha}" \
  --arg candidate_tag "${candidate_tag}" \
  --arg run_id "${run_id}" \
  --arg run_attempt "${run_attempt}" '
    if length != 1 or
       any(.[]; .sourceSha != $source_sha or .candidateTag != $candidate_tag) or
       [.[].component] != ["webui"] or
       any(.[]; (.registries | keys | sort) != ["dockerhub", "ghcr"])
    then error("candidate image metadata is incomplete or inconsistent")
    else {
      schemaVersion: 2,
      repository: $repository,
      workflow: ".github/workflows/release.yaml",
      sourceSha: $source_sha,
      candidateTag: $candidate_tag,
      runId: $run_id,
      runAttempt: $run_attempt,
      images: (reduce .[] as $image ({};
        .[$image.component] = $image.registries))
    }
    end
  ' "${parts_dir}/webui.json" >"${output_path}"

jq -e . "${output_path}" >/dev/null
echo "Candidate manifest sealed from exactly one image record."
