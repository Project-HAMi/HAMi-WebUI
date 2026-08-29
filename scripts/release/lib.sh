#!/usr/bin/env bash

set -euo pipefail

release_die() {
  echo "release error: $*" >&2
  exit 1
}

release_require_command() {
  command -v "$1" >/dev/null 2>&1 || release_die "required command not found: $1"
}

release_sha256_file() {
  local path="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${path}" | awk '{print $1}'
  else
    shasum -a 256 "${path}" | awk '{print $1}'
  fi
}

release_validate_digest() {
  [[ "$1" =~ ^sha256:[a-f0-9]{64}$ ]] || release_die "invalid sha256 digest: $1"
}

release_validate_commit() {
  [[ "$1" =~ ^[a-f0-9]{40}$ ]] || release_die "expected a full 40-character commit SHA: $1"
}

release_manifest_digest() {
  local reference="$1"
  local raw_file
  raw_file="$(mktemp)"

  if ! docker buildx imagetools inspect "${reference}" --raw >"${raw_file}"; then
    rm -f "${raw_file}"
    return 1
  fi
  printf 'sha256:%s\n' "$(release_sha256_file "${raw_file}")"
  rm -f "${raw_file}"
}

release_is_not_found_error() {
  grep -Eqi 'manifest unknown|name unknown|not found|does not exist|404' "$1"
}
