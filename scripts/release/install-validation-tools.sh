#!/usr/bin/env bash

set -euo pipefail

ACTIONLINT_VERSION="1.7.7"
ACTIONLINT_SHA256="023070a287cd8cccd71515fedc843f1985bf96c436b7effaecce67290e7e0757"
SHELLCHECK_VERSION="0.11.0"
SHELLCHECK_SHA256="b7af85e41cc99489dcc21d66c6d5f3685138f06d34651e6d34b42ec6d54fe6f6"
destination="${1:-${RUNNER_TEMP:-/tmp}/release-validation/bin}"

[[ "$(uname -s)" == "Linux" && "$(uname -m)" == "x86_64" ]] || {
  echo "validation tool installer supports only linux/x86_64" >&2
  exit 1
}

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
mkdir -p "${destination}"

actionlint_archive="${work_dir}/actionlint.tar.gz"
curl --fail --location --silent --show-error \
  "https://github.com/rhysd/actionlint/releases/download/v${ACTIONLINT_VERSION}/actionlint_${ACTIONLINT_VERSION}_linux_amd64.tar.gz" \
  --output "${actionlint_archive}"
echo "${ACTIONLINT_SHA256}  ${actionlint_archive}" | sha256sum --check --status
tar -xzf "${actionlint_archive}" -C "${work_dir}" actionlint
install -m 0755 "${work_dir}/actionlint" "${destination}/actionlint"

shellcheck_archive="${work_dir}/shellcheck.tar.gz"
curl --fail --location --silent --show-error \
  "https://github.com/koalaman/shellcheck/releases/download/v${SHELLCHECK_VERSION}/shellcheck-v${SHELLCHECK_VERSION}.linux.x86_64.tar.gz" \
  --output "${shellcheck_archive}"
echo "${SHELLCHECK_SHA256}  ${shellcheck_archive}" | sha256sum --check --status
tar -xzf "${shellcheck_archive}" -C "${work_dir}" \
  "shellcheck-v${SHELLCHECK_VERSION}/shellcheck"
install -m 0755 \
  "${work_dir}/shellcheck-v${SHELLCHECK_VERSION}/shellcheck" \
  "${destination}/shellcheck"

"${destination}/actionlint" -version
"${destination}/shellcheck" --version
