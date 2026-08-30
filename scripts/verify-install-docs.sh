#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
chart_dir="${1:-${repo_root}/charts/hami-webui}"
install_guide="${2:-${repo_root}/docs/installation/helm/index.md}"
chart_readme="${chart_dir}/README.md"

for required_file in "${chart_dir}/Chart.yaml" "${chart_readme}" "${install_guide}"; do
  if [[ ! -f "${required_file}" ]]; then
    echo "Required installation document is missing: ${required_file}" >&2
    exit 1
  fi
done

chart_version="$(awk '$1 == "version:" { value = $2; gsub(/"/, "", value); print value; exit }' "${chart_dir}/Chart.yaml")"
app_version="$(awk '$1 == "appVersion:" { value = $2; gsub(/"/, "", value); print value; exit }' "${chart_dir}/Chart.yaml")"
badge_version="${chart_version//-/--}"
badge_app_version="${app_version//-/--}"

if ! grep -F "![Version: ${chart_version}]" "${chart_readme}" | \
  grep -Fq "https://img.shields.io/badge/Version-${badge_version}-"; then
  echo "Chart README version badge does not match Chart.yaml: ${chart_version}" >&2
  exit 1
fi
if ! grep -F "![AppVersion: ${app_version}]" "${chart_readme}" | \
  grep -Fq "https://img.shields.io/badge/AppVersion-${badge_app_version}-"; then
  echo "Chart README appVersion badge does not match Chart.yaml: ${app_version}" >&2
  exit 1
fi

for install_doc in "${chart_readme}" "${install_guide}"; do
  if grep -Eq 'github\.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/(README\.md|values\.yaml)|raw\.githubusercontent\.com/Project-HAMi/HAMi-WebUI/main/charts/hami-webui/(README\.md|values\.yaml)' "${install_doc}"; then
    echo "Installation documentation must not pair main Chart files with a released Chart: ${install_doc}" >&2
    exit 1
  fi

  awk -v document="${install_doc}" '
    /^[[:space:]]*```(bash|console|sh)[[:space:]]*$/ {
      in_block = 1
      block = ""
      next
    }
    in_block && /^[[:space:]]*```[[:space:]]*$/ {
      if (index(block, "helm show values hami-webui/hami-webui") > 0) {
        saw_values = 1
        if (index(block, "--version \"${CHART_VERSION}\"") == 0) {
          printf "helm show values must use CHART_VERSION in %s\n", document > "/dev/stderr"
          failed = 1
        }
      }
      if (index(block, "helm install ") > 0 && index(block, "hami-webui/hami-webui") > 0) {
        saw_install = 1
        if (index(block, "--version \"${CHART_VERSION}\"") == 0) {
          printf "helm install must use CHART_VERSION in %s\n", document > "/dev/stderr"
          failed = 1
        }
        if (index(block, "CHART_VERSION=") == 0 && index(block, "${CHART_VERSION:?") == 0) {
          printf "helm install must define or require CHART_VERSION in the same code block: %s\n", document > "/dev/stderr"
          failed = 1
        }
        external_mode = index(block, "--set externalPrometheus.enabled=true") > 0 && \
          index(block, "--set-string externalPrometheus.address=") > 0
        managed_mode = index(block, "--set kube-prometheus-stack.enabled=true") > 0
        if (external_mode == managed_mode) {
          printf "helm install must select exactly one explicit Prometheus mode in %s\n", document > "/dev/stderr"
          failed = 1
        }
      }
      in_block = 0
      next
    }
    in_block {
      block = block "\n" $0
    }
    END {
      if (!saw_values) {
        printf "Installation documentation must retrieve version-matched values: %s\n", document > "/dev/stderr"
        failed = 1
      }
      if (!saw_install) {
        printf "Installation documentation must contain the versioned install command: %s\n", document > "/dev/stderr"
        failed = 1
      }
      exit failed
    }
  ' "${install_doc}"
done

echo "Version-matched Helm installation and Prometheus mode documentation is valid."
