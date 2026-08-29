#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

export HELM_CONFIG_HOME="${work_dir}/helm/config"
export HELM_CACHE_HOME="${work_dir}/helm/cache"
export HELM_DATA_HOME="${work_dir}/helm/data"

helm repo add nvidia-dcgm https://nvidia.github.io/dcgm-exporter/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

cp -R "${repo_root}/charts/hami-webui" "${work_dir}/hami-webui"
helm dependency build --skip-refresh "${work_dir}/hami-webui"
helm lint "${work_dir}/hami-webui"

app_version="$(helm show chart "${work_dir}/hami-webui" | awk -F': *' '$1 == "appVersion" {gsub(/\"/, "", $2); print $2}')"
expected_tag="v${app_version#v}"

tag_render="$(helm template test "${work_dir}/hami-webui")"
grep -Fq "image: \"projecthami/hami-webui-fe-oss:${expected_tag}\"" <<<"${tag_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss:${expected_tag}\"" <<<"${tag_render}"

fallback_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.frontend.tag=' \
  --set-string 'image.backend.tag=')"
grep -Fq "image: \"projecthami/hami-webui-fe-oss:${expected_tag}\"" <<<"${fallback_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss:${expected_tag}\"" <<<"${fallback_render}"

frontend_digest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
backend_digest="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
digest_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string "image.frontend.digest=${frontend_digest}" \
  --set-string "image.backend.digest=${backend_digest}")"
grep -Fq "image: \"projecthami/hami-webui-fe-oss@${frontend_digest}\"" <<<"${digest_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss@${backend_digest}\"" <<<"${digest_render}"

if helm template test "${work_dir}/hami-webui" \
  --set-string 'image.frontend.digest=not-a-digest' >/dev/null 2>&1; then
  echo "invalid image digest was accepted" >&2
  exit 1
fi

echo "Chart lint and image reference checks passed."
