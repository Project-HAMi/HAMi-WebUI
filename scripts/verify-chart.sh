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

assert_internal_prometheus_address() {
  local release_name="$1"
  local release_namespace="$2"
  shift 2

  local -a render_args=(
    --namespace "${release_namespace}"
    --set 'kube-prometheus-stack.enabled=true'
    --set 'kube-prometheus-stack.prometheus.enabled=true'
    "$@"
  )
  local service_render config_render service_name service_namespace service_port config_address expected

  service_render="$(helm template "${release_name}" "${work_dir}/hami-webui" \
    "${render_args[@]}" \
    --show-only charts/kube-prometheus-stack/templates/prometheus/service.yaml)"
  config_render="$(helm template "${release_name}" "${work_dir}/hami-webui" \
    "${render_args[@]}" \
    --show-only templates/configmap.yaml)"

  service_name="$(awk '/^metadata:$/ { metadata = 1; next } metadata && /^  name:/ { print $2; exit }' <<<"${service_render}")"
  service_namespace="$(awk '/^metadata:$/ { metadata = 1; next } metadata && /^  namespace:/ { print $2; exit }' <<<"${service_render}")"
  service_port="$(awk '/^  ports:$/ { ports = 1; next } ports && /^    port:/ { print $2; exit }' <<<"${service_render}")"
  config_address="$(awk '$1 == "address:" { print $2; exit }' <<<"${config_render}")"
  expected="http://${service_name}.${service_namespace}.svc.cluster.local:${service_port}"

  if [[ -z "${service_name}" || -z "${service_namespace}" || -z "${service_port}" || "${config_address}" != "${expected}" ]]; then
    echo "embedded Prometheus address does not match its rendered Service: ${expected}" >&2
    exit 1
  fi
}

assert_internal_prometheus_address test hami-webui-test
assert_internal_prometheus_address hami-webui-prometheus-address-regression-release-1234 hami-webui-test
assert_internal_prometheus_address test hami-webui-test \
  --set-string 'kube-prometheus-stack.fullnameOverride=metrics-core' \
  --set-string 'kube-prometheus-stack.namespaceOverride=metrics-system' \
  --set 'kube-prometheus-stack.prometheus.service.port=19090'

prometheus_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --set 'kube-prometheus-stack.enabled=true' \
  --show-only charts/kube-prometheus-stack/templates/prometheus/prometheus.yaml)"
kube_state_metrics_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --set 'kube-prometheus-stack.enabled=true' \
  --show-only charts/kube-prometheus-stack/charts/kube-state-metrics/templates/servicemonitor.yaml)"
selector_label='jobRelease: hami-webui-prometheus'
if ! grep -Fq "${selector_label}" <<<"${prometheus_render}" ||
  ! grep -Fq "${selector_label}" <<<"${kube_state_metrics_render}"; then
  echo "Bundled Prometheus does not select the kube-state-metrics ServiceMonitor" >&2
  exit 1
fi

external_address='http://external-prometheus.observability.svc.cluster.local:9090'
external_render="$(helm template test "${work_dir}/hami-webui" \
  --set 'externalPrometheus.enabled=true' \
  --set-string "externalPrometheus.address=${external_address}" \
  --set 'kube-prometheus-stack.enabled=true' \
  --set 'kube-prometheus-stack.prometheus.enabled=true' \
  --show-only templates/configmap.yaml)"
grep -Fq "address: ${external_address}" <<<"${external_render}"

deployment_with_address() {
  helm template test "${work_dir}/hami-webui" \
    --set 'externalPrometheus.enabled=true' \
    --set-string "externalPrometheus.address=$1" \
    --set-string 'podAnnotations.example\.com/team=release' \
    --set-string 'podAnnotations.checksum/config=user-value' \
    --show-only templates/deployment.yaml
}

deployment_a="$(deployment_with_address 'http://prometheus-a.example:9090')"
deployment_b="$(deployment_with_address 'http://prometheus-b.example:9090')"
checksum_a="$(awk '$1 == "checksum/config:" { print $2; exit }' <<<"${deployment_a}")"
checksum_b="$(awk '$1 == "checksum/config:" { print $2; exit }' <<<"${deployment_b}")"
if [[ ! "${checksum_a}" =~ ^[a-f0-9]{64}$ || ! "${checksum_b}" =~ ^[a-f0-9]{64}$ || "${checksum_a}" == "${checksum_b}" ]] ||
  [[ "$(grep -c 'checksum/config:' <<<"${deployment_a}")" -ne 1 ]] ||
  ! grep -Fq 'example.com/team: release' <<<"${deployment_a}"; then
  echo "Deployment config checksum does not change with the Prometheus address" >&2
  exit 1
fi

app_version="$(helm show chart "${work_dir}/hami-webui" | awk -F': *' '$1 == "appVersion" {gsub(/\"/, "", $2); print $2}')"
expected_tag="v${app_version#v}"

# Exercise the tag path independently of stable release defaults, which pin a
# digest and therefore render repository@digest instead.
tag_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.frontend.digest=' \
  --set-string 'image.backend.digest=')"
grep -Fq "image: \"projecthami/hami-webui-fe-oss:${expected_tag}\"" <<<"${tag_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss:${expected_tag}\"" <<<"${tag_render}"
frontend_container="$(awk '
  /^        - name: .*fe-oss$/ { in_frontend = 1 }
  in_frontend && /^        - name: .*be-oss$/ { exit }
  in_frontend { print }
' <<<"${tag_render}")"
if [[ -z "${frontend_container}" ]] || grep -Eq '^[[:space:]]+(command|args):' <<<"${frontend_container}"; then
  echo "Chart must leave the frontend image entrypoint and command untouched" >&2
  exit 1
fi
if [[ "$(grep -c 'path: /health_check' <<<"${tag_render}")" -ne 2 ]]; then
  echo "Frontend liveness and readiness probes were not both rendered" >&2
  exit 1
fi
grep -A1 -F 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${tag_render}" | grep -Fq 'value: "65s"'

proxy_timeout_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'frontend.proxyTimeout=125s')"
grep -A1 -F 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${proxy_timeout_render}" | grep -Fq 'value: "125s"'

proxy_timeout_env_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'env.frontend[0].name=HAMI_WEBUI_PROXY_TIMEOUT' \
  --set-string 'env.frontend[0].value=75s')"
if [[ "$(grep -c 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${proxy_timeout_env_render}")" -ne 1 ]]; then
  echo "Explicit frontend proxy-timeout environment override was duplicated" >&2
  exit 1
fi
grep -A1 -F 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${proxy_timeout_env_render}" | grep -Fq 'value: 75s'

for empty_frontend_env in '[]' 'null'; do
  empty_frontend_env_render="$(helm template test "${work_dir}/hami-webui" \
    --set-json "env.frontend=${empty_frontend_env}")"
  if [[ "$(grep -c 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${empty_frontend_env_render}")" -ne 1 ]]; then
    echo "Empty frontend env value did not render exactly one proxy-timeout variable" >&2
    exit 1
  fi
done

probes_disabled_render="$(helm template test "${work_dir}/hami-webui" \
  --set 'frontend.livenessProbe.enabled=false' \
  --set 'frontend.readinessProbe.enabled=false')"
if grep -Fq 'path: /health_check' <<<"${probes_disabled_render}"; then
  echo "Disabled frontend probes were still rendered" >&2
  exit 1
fi

fallback_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.frontend.tag=' \
  --set-string 'image.frontend.digest=' \
  --set-string 'image.backend.tag=' \
  --set-string 'image.backend.digest=')"
grep -Fq "image: \"projecthami/hami-webui-fe-oss:${expected_tag}\"" <<<"${fallback_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss:${expected_tag}\"" <<<"${fallback_render}"

frontend_digest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
backend_digest="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
digest_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string "image.frontend.digest=${frontend_digest}" \
  --set-string "image.backend.digest=${backend_digest}")"
grep -Fq "image: \"projecthami/hami-webui-fe-oss@${frontend_digest}\"" <<<"${digest_render}"
grep -Fq "image: \"projecthami/hami-webui-be-oss@${backend_digest}\"" <<<"${digest_render}"

# Chart 1.x must continue to accept the last Node-based frontend image. That
# image owns `node dist/main` through its OCI entrypoint and command, so the
# Deployment must not inject process-specific command or args.
legacy_frontend_digest='sha256:b40bbec2b963932545a8b7ac15efef3ec087c76dce4da0ea4c3659fa2abd695e'
legacy_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string "image.frontend.digest=${legacy_frontend_digest}" \
  --set-string 'image.backend.digest=')"
grep -Fq "image: \"projecthami/hami-webui-fe-oss@${legacy_frontend_digest}\"" <<<"${legacy_render}"
legacy_frontend_container="$(awk '
  /^        - name: .*fe-oss$/ { in_frontend = 1 }
  in_frontend && /^        - name: .*be-oss$/ { exit }
  in_frontend { print }
' <<<"${legacy_render}")"
if [[ -z "${legacy_frontend_container}" ]] || grep -Eq '^[[:space:]]+(command|args):' <<<"${legacy_frontend_container}"; then
  echo "Legacy frontend image compatibility was broken by a process override" >&2
  exit 1
fi

if helm template test "${work_dir}/hami-webui" \
  --set-string 'image.frontend.digest=not-a-digest' >/dev/null 2>&1; then
  echo "invalid image digest was accepted" >&2
  exit 1
fi

echo "Chart lint and image reference checks passed."
