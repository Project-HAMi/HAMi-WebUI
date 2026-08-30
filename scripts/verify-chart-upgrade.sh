#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image_repository="${1:-projecthami/hami-webui}"
image_tag="${2:-main}"
cluster_name="${3:-hami-webui-chart-upgrade}"
namespace="hami-webui-upgrade"
release_name="upgrade-test"
deployment_name="${release_name}-hami-webui"
web_service_name="${release_name}-hami-webui"
backend_service_name="${release_name}-hami-webui-backend"
work_dir="$(mktemp -d)"
cluster_created=false

cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if [[ "${cluster_created}" == "true" ]]; then
    kind delete cluster --name "${cluster_name}" >/dev/null 2>&1
  fi
  rm -rf "${work_dir}"
  exit "${status}"
}
trap cleanup EXIT

for command in docker git helm jq kind kubectl tar; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "required command is unavailable: ${command}" >&2
    exit 2
  fi
done

if [[ ! "${image_repository}" =~ ^[a-z0-9._/-]+$ ]] ||
  [[ ! "${image_tag}" =~ ^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$ ]]; then
  echo "invalid test image reference: ${image_repository}:${image_tag}" >&2
  exit 2
fi

legacy_chart="${work_dir}/legacy/charts/hami-webui"
current_chart="${work_dir}/current/hami-webui"
mkdir -p "${work_dir}/legacy" "${work_dir}/current"
git -C "${repo_root}" archive v1.3.0 charts/hami-webui | tar -x -C "${work_dir}/legacy"
cp -R "${repo_root}/charts/hami-webui" "${current_chart}"

export HELM_CONFIG_HOME="${work_dir}/helm/config"
export HELM_CACHE_HOME="${work_dir}/helm/cache"
export HELM_DATA_HOME="${work_dir}/helm/data"
helm repo add nvidia-dcgm https://nvidia.github.io/dcgm-exporter/helm-charts >/dev/null
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm dependency build --skip-refresh "${legacy_chart}" >/dev/null
helm dependency build --skip-refresh "${current_chart}" >/dev/null

legacy_overrides=(
  # The released Chart renders multi-architecture index digests. Pull those
  # immutable references below, retag the selected platform manifests, and load
  # the tags into Kind so kubelet can use the locally imported images.
  --set-string image.frontend.digest=
  --set-string image.backend.digest=
  --set dcgm-exporter.enabled=false
  --set serviceMonitor.enabled=false
  --set hamiServiceMonitor.enabled=false
  --set kube-prometheus-stack.enabled=false
)
legacy_digest_images="$(helm template "${release_name}" "${legacy_chart}" \
  --namespace "${namespace}" \
  --set dcgm-exporter.enabled=false \
  --set serviceMonitor.enabled=false \
  --set hamiServiceMonitor.enabled=false \
  --set kube-prometheus-stack.enabled=false \
  --show-only templates/deployment.yaml | \
  awk '$1 == "image:" { gsub(/"/, "", $2); print $2 }')"
legacy_images="$(helm template "${release_name}" "${legacy_chart}" \
  --namespace "${namespace}" \
  "${legacy_overrides[@]}" \
  --show-only templates/deployment.yaml | \
  awk '$1 == "image:" { gsub(/\"/, "", $2); print $2 }')"
if [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"${legacy_images}")" -ne 2 ]]; then
  echo "expected exactly two Chart 1.3 application images" >&2
  exit 1
fi
if [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"${legacy_digest_images}")" -ne 2 ]]; then
  echo "expected exactly two pinned Chart 1.3 application images" >&2
  exit 1
fi

paste <(printf '%s\n' "${legacy_digest_images}") <(printf '%s\n' "${legacy_images}") |
  while IFS=$'\t' read -r digest_image runtime_image; do
    docker pull "${digest_image}"
    docker tag "${digest_image}" "${runtime_image}"
  done

image_reference="${image_repository}:${image_tag}"
if ! docker image inspect "${image_reference}" >/dev/null 2>&1; then
  docker pull "${image_reference}"
fi
all_images="${legacy_images}"$'\n'"${image_reference}"

if kind get clusters | grep -Fxq "${cluster_name}"; then
  echo "Kind cluster already exists; choose another name: ${cluster_name}" >&2
  exit 2
fi

kind create cluster \
  --name "${cluster_name}" \
  --image kindest/node:v1.30.10@sha256:4de75d0e82481ea846c0ed1de86328d821c1e6a6a91ac37bf804e5313670e507 \
  --wait 120s
cluster_created=true

while IFS= read -r image; do
  [[ -n "${image}" ]] || continue
  if ! docker image inspect "${image}" >/dev/null 2>&1; then
    echo "test image was not prepared locally: ${image}" >&2
    exit 1
  fi
  kind load docker-image --name "${cluster_name}" "${image}"
done <<<"${all_images}"

helm install "${release_name}" "${legacy_chart}" \
  --namespace "${namespace}" \
  --create-namespace \
  --values "${legacy_chart}/values.yaml" \
  "${legacy_overrides[@]}" \
  --wait \
  --timeout 10m

container_count() {
  kubectl --namespace "${namespace}" get deployment "${deployment_name}" \
    --output json | jq '.spec.template.spec.containers | length'
}

service_port_names() {
  local service_name=$1
  kubectl --namespace "${namespace}" get service "${service_name}" \
    --output json | jq -r '.spec.ports[].name' | sort
}

service_proxy_get() {
  local service_name=$1
  local port_name=$2
  local path=$3
  kubectl get --raw \
    "/api/v1/namespaces/${namespace}/services/http:${service_name}:${port_name}/proxy/${path}"
}

wait_for_service_path() {
  local service_name=$1
  local port_name=$2
  local path=$3
  for _ in {1..90}; do
    if service_proxy_get "${service_name}" "${port_name}" "${path}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "service path did not become ready: ${service_name}:${port_name}/${path}" >&2
  return 1
}

assert_equal() {
  local description=$1
  local expected=$2
  local actual=$3
  if [[ "${actual}" != "${expected}" ]]; then
    printf 'unexpected %s: expected %q, got %q\n' \
      "${description}" "${expected}" "${actual}" >&2
    return 1
  fi
}

assert_legacy_contract() {
  assert_equal "Chart 1.3 container count" "2" "$(container_count)"
  assert_equal "Chart 1.3 Web Service ports" $'http\nmetrics' \
    "$(service_port_names "${web_service_name}")"
  if kubectl --namespace "${namespace}" get service "${backend_service_name}" >/dev/null 2>&1; then
    echo "Chart 1.3 unexpectedly contains the Chart 2 backend discovery Service" >&2
    return 1
  fi
  wait_for_service_path "${web_service_name}" http health_check
  service_proxy_get "${web_service_name}" metrics metrics | grep -Fq '# HELP'
}

assert_current_contract() {
  assert_equal "Chart 2 container count" "1" "$(container_count)"
  assert_equal "Chart 2 Web Service ports" "http" \
    "$(service_port_names "${web_service_name}")"
  assert_equal "Chart 2 internal Service ports" "backend-http" \
    "$(service_port_names "${backend_service_name}")"
  wait_for_service_path "${web_service_name}" http health_check
  service_proxy_get "${backend_service_name}" backend-http readyz | grep -Fq 'ok'
  service_proxy_get "${backend_service_name}" backend-http metrics | grep -Fq '# HELP'
  kubectl --namespace "${namespace}" exec deployment/"${deployment_name}" -- \
    /apps/hami-webui --healthcheck
}

assert_legacy_contract

assert_upgrade_rejected() {
  local mode=$1
  shift
  local output
  if output="$(helm upgrade "${release_name}" "${current_chart}" \
    --namespace "${namespace}" "$@" 2>&1)"; then
    echo "Chart 2 unexpectedly accepted Chart 1 values during ${mode}" >&2
    return 1
  fi
  if ! grep -Fq 'HAMi-WebUI Chart 2.0 no longer accepts Chart 1.x split-container values' <<<"${output}"; then
    echo "${mode} did not return the actionable Chart 2 migration error" >&2
    echo "${output}" >&2
    return 1
  fi
  [[ "$(helm history "${release_name}" --namespace "${namespace}" --output json | jq 'length')" == "1" ]]
  assert_legacy_contract
}

assert_upgrade_rejected "ordinary upgrade"
assert_upgrade_rejected "--reuse-values upgrade" --reuse-values

values_v2="${work_dir}/values-v2.yaml"
cat >"${values_v2}" <<EOF
image:
  repository: "${image_repository}"
  pullPolicy: IfNotPresent
  tag: "${image_tag}"
  digest: ""
dcgm-exporter:
  enabled: false
serviceMonitor:
  enabled: false
hamiServiceMonitor:
  enabled: false
kube-prometheus-stack:
  enabled: false
externalPrometheus:
  enabled: true
  address: http://prometheus.invalid
  timeout: 1s
metricsExporter:
  interval: 3600s
  timeout: 1s
EOF

helm upgrade "${release_name}" "${current_chart}" \
  --namespace "${namespace}" \
  --reset-values \
  --values "${values_v2}" \
  --wait \
  --timeout 10m &
upgrade_pid=$!
availability_failures=0
while kill -0 "${upgrade_pid}" >/dev/null 2>&1; do
  if ! service_proxy_get "${web_service_name}" http health_check >/dev/null 2>&1; then
    sleep 0.2
    if ! service_proxy_get "${web_service_name}" http health_check >/dev/null 2>&1; then
      availability_failures=$((availability_failures + 1))
    fi
  fi
  sleep 1
done
wait "${upgrade_pid}"
if [[ ${availability_failures} -ne 0 ]]; then
  echo "Web Service was unavailable during the Chart 2 rolling upgrade" >&2
  exit 1
fi

assert_current_contract

helm rollback "${release_name}" 1 \
  --namespace "${namespace}" \
  --wait \
  --timeout 10m
assert_legacy_contract

helm rollback "${release_name}" 2 \
  --namespace "${namespace}" \
  --wait \
  --timeout 10m
assert_current_contract

echo "Chart 1.3 to Chart 2 upgrade and rollback checks passed."
