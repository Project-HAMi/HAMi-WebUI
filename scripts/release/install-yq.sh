#!/usr/bin/env bash

set -euo pipefail

YQ_VERSION="v4.47.2"
YQ_SHA256="1bb99e1019e23de33c7e6afc23e93dad72aad6cf2cb03c797f068ea79814ddb0"
destination="${1:-${RUNNER_TEMP:-/tmp}/release-tools/yq}"
archive="$(mktemp)"

mkdir -p "$(dirname "${destination}")"
curl --fail --location --silent --show-error \
  "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" \
  --output "${archive}"
echo "${YQ_SHA256}  ${archive}" | sha256sum --check --status
install -m 0755 "${archive}" "${destination}"
rm -f "${archive}"
printf '%s\n' "${destination}"
