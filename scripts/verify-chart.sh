#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT

export HELM_CONFIG_HOME="${work_dir}/helm/config"
export HELM_CACHE_HOME="${work_dir}/helm/cache"
export HELM_DATA_HOME="${work_dir}/helm/data"

bash "${repo_root}/scripts/verify-install-docs.sh"

helm repo add nvidia-dcgm https://nvidia.github.io/dcgm-exporter/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

cp -R "${repo_root}/charts/hami-webui" "${work_dir}/hami-webui"
cp "${repo_root}/scripts/chart/tests/render-install-notes.yaml" \
  "${work_dir}/hami-webui/templates/test-install-notes.yaml"
helm dependency build --skip-refresh "${work_dir}/hami-webui"

prometheus_test_address='http://prometheus.monitoring.svc.cluster.local:9090'
if default_prometheus_output="$(helm template test "${work_dir}/hami-webui" 2>&1)"; then
  echo "Chart defaults unexpectedly guessed a Prometheus backend" >&2
  exit 1
fi
if ! grep -Fq 'Prometheus is not configured' <<<"${default_prometheus_output}"; then
  echo "Unconfigured Prometheus mode did not return an actionable error" >&2
  echo "${default_prometheus_output}" >&2
  exit 1
fi

helm lint --strict "${work_dir}/hami-webui" \
  --set 'externalPrometheus.enabled=true' \
  --set-string "externalPrometheus.address=${prometheus_test_address}"

# Most assertions below exercise unrelated Chart contracts. Give the temporary
# test copy one explicit external Prometheus mode while leaving the shipped
# defaults unconfigured and fail-closed.
test_values_tmp="${work_dir}/values.yaml.tmp"
awk -v address="${prometheus_test_address}" '
  $0 == "externalPrometheus:" { in_external = 1; print; next }
  in_external && $0 == "  enabled: false" { print "  enabled: true"; next }
  in_external && $0 ~ /^  address:/ {
    print "  address: \"" address "\""
    in_external = 0
    next
  }
  { print }
' "${work_dir}/hami-webui/values.yaml" >"${test_values_tmp}"
mv "${test_values_tmp}" "${work_dir}/hami-webui/values.yaml"

render_template() {
  local template="$1"
  shift

  helm template test "${work_dir}/hami-webui" \
    --namespace hami-webui-test \
    "$@" \
    --show-only "${template}"
}

service_port_names() {
  awk '
    /^  ports:$/ { in_ports = 1; next }
    /^  selector:$/ { in_ports = 0 }
    in_ports && /^      name:/ { print $2 }
  '
}

web_service_render="$(render_template templates/service.yaml)"
backend_service_render="$(render_template templates/backend-service.yaml)"
service_monitor_render="$(render_template templates/servicemonitor.yaml)"
hami_service_monitor_render="$(render_template templates/hamiservicemonitor.yaml)"
dcgm_service_monitor_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --show-only charts/dcgm-exporter/templates/service-monitor.yaml)"
default_deployment_render="$(render_template templates/deployment.yaml)"
default_config_render="$(render_template templates/configmap.yaml)"
default_install_notes_render="$(render_template templates/test-install-notes.yaml)"

deployment_containers="$(awk '
  /^      containers:$/ { in_containers = 1; next }
  in_containers && /^      [[:alpha:]][[:alnum:]]*:/ { exit }
  in_containers && /^        - name:/ { print $3 }
' <<<"${default_deployment_render}")"
if [[ "$(awk 'NF { count++ } END { print count + 0 }' <<<"${deployment_containers}")" -ne 1 ]]; then
  echo "Chart 2 must render exactly one application container" >&2
  exit 1
fi

application_container="$(awk '
  /^      containers:$/ { in_containers = 1; next }
  in_containers && /^      [[:alpha:]][[:alnum:]]*:/ { exit }
  in_containers { print }
' <<<"${default_deployment_render}")"
if [[ "$(grep -c 'containerPort:' <<<"${application_container}")" -ne 2 ]] ||
  ! grep -B1 -A1 -F 'containerPort: 3000' <<<"${application_container}" | grep -Fq 'name: http' ||
  ! grep -B1 -A1 -F 'containerPort: 8000' <<<"${application_container}" | grep -Fq 'name: metrics'; then
  echo "The unified container must expose named http:3000 and metrics:8000 ports" >&2
  exit 1
fi
if grep -Eq '^[[:space:]]+(command|args):' <<<"${application_container}"; then
  echo "Chart must leave the unified image entrypoint and command untouched" >&2
  exit 1
fi

probe_block() {
  local probe_name="$1"
  awk -v probe="${probe_name}" '
    $0 == "          " probe ":" { in_probe = 1; print; next }
    in_probe && /^          [[:alpha:]][[:alnum:]]*:/ { exit }
    in_probe { print }
  '
}

startup_probe="$(probe_block startupProbe <<<"${application_container}")"
readiness_probe="$(probe_block readinessProbe <<<"${application_container}")"
liveness_probe="$(probe_block livenessProbe <<<"${application_container}")"
if ! grep -Fq 'path: /readyz' <<<"${startup_probe}" ||
  ! grep -Fq 'port: metrics' <<<"${startup_probe}" ||
  ! grep -Fq 'periodSeconds: 5' <<<"${startup_probe}" ||
  ! grep -Fq 'failureThreshold: 60' <<<"${startup_probe}"; then
  echo "Startup probe must allow five minutes for backend informer synchronization on /readyz:8000" >&2
  exit 1
fi
for steady_probe in "${readiness_probe}" "${liveness_probe}"; do
  if ! grep -Fq 'path: /health_check' <<<"${steady_probe}" ||
    ! grep -Fq 'port: http' <<<"${steady_probe}"; then
    echo "Steady-state readiness and liveness must check the public Web entry on /health_check:3000" >&2
    exit 1
  fi
done

if grep -Eq '^[[:space:]]+grpc:|0\.0\.0\.0:9000' <<<"${default_config_render}" ||
  ! grep -Fq 'addr: 0.0.0.0:8000' <<<"${default_config_render}"; then
  echo "Chart 2 configuration must keep HTTP :8000 and remove the unused gRPC listener" >&2
  exit 1
fi

if grep -Fq '      imagePullSecrets:' <<<"${default_deployment_render}"; then
  echo "An empty imagePullSecrets value must not render a Pod field" >&2
  exit 1
fi

private_registry_deployment_render="$(render_template templates/deployment.yaml \
  --set-string 'imagePullSecrets[0].name=private-registry' \
  --set-string 'imagePullSecrets[1].name=backup-registry')"
image_pull_secrets_block="$(awk '
  /^      imagePullSecrets:$/ { in_secrets = 1; print; next }
  in_secrets && /^        - name:/ { print; next }
  in_secrets { exit }
' <<<"${private_registry_deployment_render}")"
expected_image_pull_secrets_block=$'      imagePullSecrets:\n        - name: private-registry\n        - name: backup-registry'
if [[ "${image_pull_secrets_block}" != "${expected_image_pull_secrets_block}" ]]; then
  echo "Configured imagePullSecrets were not rendered on the WebUI Pod" >&2
  exit 1
fi

if [[ "$(service_port_names <<<"${web_service_render}")" != "http" ]] ||
  ! grep -Fq 'port: 3000' <<<"${web_service_render}" ||
  ! grep -Fq 'targetPort: http' <<<"${web_service_render}" ||
  grep -Fq 'targetPort: metrics' <<<"${web_service_render}"; then
  echo "Primary Service must expose only the supported Web entry on port 3000" >&2
  exit 1
fi

if ! grep -Fq 'Visit http://127.0.0.1:3000/ to use HAMi-WebUI' <<<"${default_install_notes_render}" ||
  ! grep -Fq 'kubectl --namespace hami-webui-test port-forward service/test-hami-webui 3000:http' <<<"${default_install_notes_render}" ||
  grep -Fq 'POD_NAME=' <<<"${default_install_notes_render}"; then
  echo "ClusterIP install notes must forward the named Service port and show the effective public URL" >&2
  exit 1
fi
if [[ "$(service_port_names <<<"${backend_service_render}")" != "backend-http" ]] ||
  ! grep -Fq 'name: test-hami-webui-backend' <<<"${backend_service_render}" ||
  ! grep -Fq 'type: ClusterIP' <<<"${backend_service_render}" ||
  ! grep -Fq 'monitoring.hami.io/job: "test-hami-webui"' <<<"${backend_service_render}" ||
  ! grep -Fq 'port: 8000' <<<"${backend_service_render}" ||
  ! grep -Fq 'targetPort: metrics' <<<"${backend_service_render}"; then
  echo "Internal backend Service contract was not rendered" >&2
  exit 1
fi

long_fullname="$(printf '%063d' 0 | tr '0' 'a')"
other_long_fullname="${long_fullname:0:55}bbbbbbbb"
long_name_web_service="$(render_template templates/service.yaml \
  --set-string "fullnameOverride=${long_fullname}")"
long_name_backend_service="$(render_template templates/backend-service.yaml \
  --set-string "fullnameOverride=${long_fullname}")"
other_long_name_backend_service="$(render_template templates/backend-service.yaml \
  --set-string "fullnameOverride=${other_long_fullname}")"
rendered_web_service_name="$(awk '$1 == "name:" { print $2; exit }' <<<"${long_name_web_service}")"
rendered_backend_service_name="$(awk '$1 == "name:" { print $2; exit }' <<<"${long_name_backend_service}")"
other_rendered_backend_service_name="$(awk '$1 == "name:" { print $2; exit }' <<<"${other_long_name_backend_service}")"
if [[ ${#rendered_backend_service_name} -gt 63 ]] ||
  [[ "${rendered_backend_service_name}" != *-backend ]] ||
  [[ "${rendered_backend_service_name}" == "${rendered_web_service_name}" ]] ||
  [[ "${rendered_backend_service_name}" == "${other_rendered_backend_service_name}" ]]; then
  echo "Internal backend Service name is not distinct and DNS-label safe" >&2
  exit 1
fi

service_monitor_spec="$(sed -n '/^spec:/,$p' <<<"${service_monitor_render}")"
if ! grep -Fq 'app.kubernetes.io/component: "backend"' <<<"${service_monitor_spec}" ||
  grep -Fq 'app.kubernetes.io/component: "hami-webui"' <<<"${service_monitor_spec}" ||
  ! grep -Fq 'jobLabel: monitoring.hami.io/job' <<<"${service_monitor_spec}" ||
  ! grep -Fq 'port: "backend-http"' <<<"${service_monitor_spec}"; then
  echo "ServiceMonitor must select only the internal backend Service" >&2
  exit 1
fi
if ! grep -Fq 'jobRelease: hami-webui-prometheus' <<<"${service_monitor_render}" ||
  ! grep -Fq 'jobRelease: hami-webui-prometheus' <<<"${hami_service_monitor_render}" ||
  ! grep -Fq 'jobRelease: hami-webui-prometheus' <<<"${dcgm_service_monitor_render}" ||
  ! grep -Fq 'honorLabels: true' <<<"${hami_service_monitor_render}"; then
  echo "Prometheus Operator mode did not render the aligned scrape labels and HAMi label contract" >&2
  exit 1
fi

load_balancer_web_service="$(render_template templates/service.yaml \
  --set 'service.type=LoadBalancer')"
load_balancer_backend_service="$(render_template templates/backend-service.yaml \
  --set 'service.type=LoadBalancer')"
if ! grep -Fq 'type: LoadBalancer' <<<"${load_balancer_web_service}" ||
  ! grep -Fq 'type: ClusterIP' <<<"${load_balancer_backend_service}" ||
  grep -Fq 'type: LoadBalancer' <<<"${load_balancer_backend_service}"; then
  echo "Internal backend Service must not inherit the public Service type" >&2
  exit 1
fi

monitor_disabled_render="$(helm template test "${work_dir}/hami-webui" \
  --set 'serviceMonitor.enabled=false' \
  --set 'hamiServiceMonitor.enabled=false')"
if grep -Fq 'name: test-hami-webui-svc-monitor' <<<"${monitor_disabled_render}" ||
  ! grep -Fq 'name: test-hami-webui-backend' <<<"${monitor_disabled_render}"; then
  echo "Disabling ServiceMonitors must not remove the internal backend Service" >&2
  exit 1
fi

assert_internal_prometheus_address() {
  local release_name="$1"
  local release_namespace="$2"
  shift 2

  local -a render_args=(
    --namespace "${release_namespace}"
    --set 'externalPrometheus.enabled=false'
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
  --set 'externalPrometheus.enabled=false' \
  --set 'kube-prometheus-stack.enabled=true' \
  --show-only charts/kube-prometheus-stack/templates/prometheus/prometheus.yaml)"
kube_state_metrics_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --set 'externalPrometheus.enabled=false' \
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
  --show-only templates/configmap.yaml)"
grep -Fq "address: ${external_address}" <<<"${external_render}"
grep -Fq 'insecure_skip_verify: false' <<<"${external_render}"

assert_prometheus_mode_rejected() {
  local expected_message="$1"
  shift
  local output

  if output="$(helm template test "${work_dir}/hami-webui" "$@" 2>&1)"; then
    echo "Invalid Prometheus mode was accepted: ${expected_message}" >&2
    exit 1
  fi
  if ! grep -Fq "${expected_message}" <<<"${output}"; then
    echo "Prometheus mode rejection did not explain: ${expected_message}" >&2
    echo "${output}" >&2
    exit 1
  fi
}

assert_prometheus_mode_rejected 'Prometheus is not configured' \
  --set 'externalPrometheus.enabled=false' \
  --set 'kube-prometheus-stack.enabled=false'
assert_prometheus_mode_rejected 'mutually exclusive' \
  --set 'externalPrometheus.enabled=true' \
  --set-string "externalPrometheus.address=${external_address}" \
  --set 'kube-prometheus-stack.enabled=true'
assert_prometheus_mode_rejected 'externalPrometheus.address is required' \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address='
assert_prometheus_mode_rejected 'absolute http:// or https:// URL' \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=prometheus.monitoring.svc:9090'
assert_prometheus_mode_rejected 'externalPrometheus.address must include a hostname' \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=http:///api/v1'
assert_prometheus_mode_rejected 'externalPrometheus.address must include a hostname' \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=http://:9090'
assert_prometheus_mode_rejected 'kube-prometheus-stack.prometheus.enabled must remain true' \
  --set 'externalPrometheus.enabled=false' \
  --set 'kube-prometheus-stack.enabled=true' \
  --set 'kube-prometheus-stack.prometheus.enabled=false'

raw_prometheus_render="$(helm template test "${work_dir}/hami-webui" \
  --set 'externalPrometheus.enabled=true' \
  --set-string "externalPrometheus.address=${external_address}" \
  --set 'serviceMonitor.enabled=false' \
  --set 'hamiServiceMonitor.enabled=false' \
  --set 'dcgm-exporter.serviceMonitor.enabled=false')"
if grep -Fq 'kind: ServiceMonitor' <<<"${raw_prometheus_render}" ||
  ! grep -Fq 'kind: DaemonSet' <<<"${raw_prometheus_render}" ||
  ! grep -Fq "address: ${external_address}" <<<"${raw_prometheus_render}"; then
  echo "Manual-scrape mode did not remove only the Operator custom resources" >&2
  exit 1
fi

existing_operator_args=(
  --namespace hami-webui-test
  --include-crds
  --set 'externalPrometheus.enabled=false'
  --set 'kube-prometheus-stack.enabled=true'
)
existing_operator_render="$(helm template test "${work_dir}/hami-webui" \
  "${existing_operator_args[@]}")"
if ! grep -Eq '^kind: Prometheus$' <<<"${existing_operator_render}" ||
  grep -Eq '^kind: CustomResourceDefinition$' <<<"${existing_operator_render}" ||
  helm template test "${work_dir}/hami-webui" \
    "${existing_operator_args[@]}" \
    --show-only charts/kube-prometheus-stack/templates/prometheus-operator/deployment.yaml \
    >/dev/null 2>&1; then
  echo "Existing-Operator mode rendered the wrong Prometheus ownership resources" >&2
  exit 1
fi

self_contained_args=(
  --namespace hami-webui-test
  --include-crds
  --set 'externalPrometheus.enabled=false'
  --set 'kube-prometheus-stack.enabled=true'
  --set 'kube-prometheus-stack.crds.enabled=true'
  --set 'kube-prometheus-stack.prometheusOperator.enabled=true'
)
self_contained_render="$(helm template test "${work_dir}/hami-webui" \
  "${self_contained_args[@]}")"
self_contained_prometheus_render="$(helm template test "${work_dir}/hami-webui" \
  "${self_contained_args[@]}" \
  --show-only charts/kube-prometheus-stack/templates/prometheus/prometheus.yaml)"
self_contained_operator_render="$(helm template test "${work_dir}/hami-webui" \
  "${self_contained_args[@]}" \
  --show-only charts/kube-prometheus-stack/templates/prometheus-operator/deployment.yaml)"
if ! grep -Eq '^kind: CustomResourceDefinition$' <<<"${self_contained_render}" ||
  ! grep -Eq '^kind: Prometheus$' <<<"${self_contained_prometheus_render}" ||
  ! grep -Eq '^kind: Deployment$' <<<"${self_contained_operator_render}" ||
  ! grep -Fq 'app: kube-prometheus-stack-operator' <<<"${self_contained_operator_render}"; then
  echo "Self-contained Prometheus mode omitted CRDs, Operator, or Prometheus" >&2
  exit 1
fi

external_deployment_render="$(render_template templates/deployment.yaml \
  --set 'externalPrometheus.enabled=true' \
  --set-string "externalPrometheus.address=${external_address}")"
if grep -Fq 'prometheus-tls' <<<"${external_deployment_render}"; then
  echo "External Prometheus must not mount TLS material unless a Secret is configured" >&2
  exit 1
fi

prometheus_tls_ca_config="$(render_template templates/configmap.yaml \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=https://prometheus.example.com' \
  --set-string 'externalPrometheus.tls.existingSecret=prometheus-ca' \
  --set-string 'externalPrometheus.tls.caKey=company-ca.pem')"
prometheus_tls_ca_deployment="$(render_template templates/deployment.yaml \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=https://prometheus.example.com' \
  --set-string 'externalPrometheus.tls.existingSecret=prometheus-ca' \
  --set-string 'externalPrometheus.tls.caKey=company-ca.pem')"
if ! grep -Fq 'ca_file: "/apps/prometheus-tls/ca.crt"' <<<"${prometheus_tls_ca_config}" ||
  grep -Eq 'prometheus-ca|company-ca\.pem' <<<"${prometheus_tls_ca_config}" ||
  ! grep -Fq 'secretName: "prometheus-ca"' <<<"${prometheus_tls_ca_deployment}" ||
  ! grep -A1 -F 'key: "company-ca.pem"' <<<"${prometheus_tls_ca_deployment}" | grep -Fq 'path: ca.crt' ||
  [[ "$(grep -c 'name: prometheus-tls' <<<"${prometheus_tls_ca_deployment}")" -ne 2 ]] ||
  ! grep -A2 -F 'mountPath: /apps/prometheus-tls/' <<<"${prometheus_tls_ca_deployment}" | grep -Fq 'readOnly: true'; then
  echo "Private-CA Secret was not rendered as a single unified-container fixed-path mount" >&2
  exit 1
fi

prometheus_mtls_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=https://prometheus.example.com' \
  --set-string 'externalPrometheus.tls.serverName=prometheus.internal' \
  --set-string 'externalPrometheus.tls.existingSecret=prometheus-mtls' \
  --set-string 'externalPrometheus.tls.caKey=root.pem' \
  --set-string 'externalPrometheus.tls.certKey=client.pem' \
  --set-string 'externalPrometheus.tls.keyKey=client-key.pem' \
  --show-only templates/configmap.yaml \
  --show-only templates/deployment.yaml)"
for expected in \
  'server_name: "prometheus.internal"' \
  'ca_file: "/apps/prometheus-tls/ca.crt"' \
  'cert_file: "/apps/prometheus-tls/tls.crt"' \
  'key_file: "/apps/prometheus-tls/tls.key"' \
  'key: "root.pem"' \
  'key: "client.pem"' \
  'key: "client-key.pem"'; do
  if ! grep -Fq "${expected}" <<<"${prometheus_mtls_render}"; then
    echo "mTLS rendering is missing ${expected}" >&2
    exit 1
  fi
done

prometheus_insecure_render="$(render_template templates/configmap.yaml \
  --set 'externalPrometheus.enabled=true' \
  --set-string 'externalPrometheus.address=https://prometheus.example.com' \
  --set 'externalPrometheus.tls.insecureSkipVerify=true')"
grep -Fq 'insecure_skip_verify: true' <<<"${prometheus_insecure_render}"

disabled_tls_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --set 'externalPrometheus.enabled=false' \
  --set-string 'externalPrometheus.tls.existingSecret=unused-tls' \
  --set-string 'externalPrometheus.tls.caKey=ca.pem' \
  --set 'kube-prometheus-stack.enabled=true' \
  --show-only templates/configmap.yaml \
  --show-only templates/deployment.yaml)"
if grep -Eq 'prometheus-tls|insecure_skip_verify|unused-tls|ca\.pem' <<<"${disabled_tls_render}"; then
  echo "Disabled external Prometheus unexpectedly rendered TLS configuration" >&2
  exit 1
fi

for invalid_prometheus_tls in \
  '--set-string=externalPrometheus.tls.insecureSkipVerify=true' \
  '--set-string=externalPrometheus.tls.unknownField=value' \
  '--set=externalPrometheus.enabled=true --set-string=externalPrometheus.tls.certKey=tls.crt' \
  '--set=externalPrometheus.enabled=true --set-string=externalPrometheus.tls.keyKey=tls.key' \
  '--set=externalPrometheus.enabled=true --set-string=externalPrometheus.tls.caKey=ca.crt' \
  '--set=externalPrometheus.enabled=true --set-string=externalPrometheus.tls.existingSecret=empty-secret'; do
  read -r -a invalid_args <<<"${invalid_prometheus_tls}"
  if helm template test "${work_dir}/hami-webui" "${invalid_args[@]}" >/dev/null 2>&1; then
    echo "Invalid external Prometheus TLS values were accepted: ${invalid_prometheus_tls}" >&2
    exit 1
  fi
done

if grep -Fq 'kind: Secret' <<<"${prometheus_mtls_render}"; then
  echo "The Chart must reference, not create, Prometheus credential Secrets" >&2
  exit 1
fi

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

# The development default is deliberately mutable. Stable release automation
# replaces it with the verified candidate tag or digest before publication.
grep -Fq 'image: "projecthami/hami-webui:main"' <<<"${default_deployment_render}"

explicit_tag_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.tag=chart-contract' \
  --set-string 'image.digest=' \
  --show-only templates/deployment.yaml)"
grep -Fq 'image: "projecthami/hami-webui:chart-contract"' <<<"${explicit_tag_render}"

fallback_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.tag=' \
  --set-string 'image.digest=' \
  --show-only templates/deployment.yaml)"
grep -Fq 'image: "projecthami/hami-webui:main"' <<<"${fallback_render}"

assert_release_app_version_fallback() {
  local app_version="$1"
  local expected_tag="$2"
  local release_chart
  release_chart="$(mktemp -d "${work_dir}/release-XXXXXX")/hami-webui"
  local chart_yaml_tmp="${release_chart}/Chart.yaml.tmp"

  cp -R "${work_dir}/hami-webui" "${release_chart}"
  awk -v app_version="${app_version}" '
    $1 == "appVersion:" { print "appVersion: \"" app_version "\""; next }
    { print }
  ' "${release_chart}/Chart.yaml" >"${chart_yaml_tmp}"
  mv "${chart_yaml_tmp}" "${release_chart}/Chart.yaml"

  local release_fallback_render
  release_fallback_render="$(helm template test "${release_chart}" \
    --set-string 'image.tag=' \
    --set-string 'image.digest=' \
    --show-only templates/deployment.yaml)"
  grep -Fq "image: \"projecthami/hami-webui:${expected_tag}\"" <<<"${release_fallback_render}"
}

assert_release_app_version_fallback '2.0.0' 'v2.0.0'
assert_release_app_version_fallback 'v2.0.0' 'v2.0.0'
assert_release_app_version_fallback '2.0.0-rc.1+build.7' 'v2.0.0-rc.1_build.7'

if helm template test "${work_dir}/hami-webui" \
  --set-string 'image.tag=invalid+tag' >/dev/null 2>&1; then
  echo "Invalid OCI image tag was accepted" >&2
  exit 1
fi

image_digest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
digest_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.tag=must-not-win' \
  --set-string "image.digest=${image_digest}" \
  --show-only templates/deployment.yaml)"
grep -Fq "image: \"projecthami/hami-webui@${image_digest}\"" <<<"${digest_render}"
if grep -Fq 'must-not-win' <<<"${digest_render}"; then
  echo "Image tag took precedence over an immutable digest" >&2
  exit 1
fi

if helm template test "${work_dir}/hami-webui" \
  --set-string 'image.digest=not-a-digest' >/dev/null 2>&1; then
  echo "Invalid image digest was accepted" >&2
  exit 1
fi

grep -A1 -F 'name: HAMI_WEBUI_BASE_PATH' <<<"${default_deployment_render}" | grep -Fq 'value: "/"'
if grep -Fq 'name: HAMI_WEBUI_FRAME_ANCESTORS_JSON' <<<"${default_deployment_render}"; then
  echo "Default framing policy must preserve the Chart 1.x no-header behavior" >&2
  exit 1
fi
if grep -Fq 'name: HAMI_WEBUI_PROXY_TIMEOUT' <<<"${default_deployment_render}"; then
  echo "Removed frontend proxy timeout leaked into the single-process Deployment" >&2
  exit 1
fi

base_path_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'frontend.basePath=gpu-ui')"
grep -A1 -F 'name: HAMI_WEBUI_BASE_PATH' <<<"${base_path_render}" | grep -Fq 'value: "/gpu-ui/"'

base_path_notes_render="$(render_template templates/test-install-notes.yaml \
  --set-string 'frontend.basePath=gpu-ui')"
grep -Fq 'Visit http://127.0.0.1:3000/gpu-ui/ to use HAMi-WebUI' <<<"${base_path_notes_render}"

namespace_override_notes_render="$(render_template templates/test-install-notes.yaml \
  --set-string 'namespaceOverride=runtime-ns' \
  --set-string 'frontend.basePath=/gpu-ui/')"
grep -Fq 'kubectl --namespace runtime-ns port-forward service/test-hami-webui 3000:http' <<<"${namespace_override_notes_render}"
if grep -Fq 'kubectl --namespace hami-webui-test' <<<"${namespace_override_notes_render}"; then
  echo "Install notes ignored namespaceOverride" >&2
  exit 1
fi

node_port_notes_render="$(render_template templates/test-install-notes.yaml \
  --set-string 'service.type=NodePort' \
  --set-string 'frontend.basePath=/gpu-ui/')"
grep -Fq 'echo "http://$NODE_IP:$NODE_PORT/gpu-ui/"' <<<"${node_port_notes_render}"

load_balancer_notes_render="$(render_template templates/test-install-notes.yaml \
  --set-string 'service.type=LoadBalancer' \
  --set-string 'frontend.basePath=/gpu-ui/')"
grep -Fq 'echo "http://$SERVICE_HOST:3000/gpu-ui/"' <<<"${load_balancer_notes_render}"

ingress_render="$(render_template templates/ingress.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].host=ui.example.com' \
  --set-string 'ingress.hosts[0].paths[0].path=/gpu-ui' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix')"
grep -Fq 'path: /gpu-ui' <<<"${ingress_render}"
grep -Fq 'pathType: Prefix' <<<"${ingress_render}"

default_ingress_render="$(render_template templates/ingress.yaml \
  --set 'ingress.enabled=true')"
grep -Fq 'path: /' <<<"${default_ingress_render}"
grep -Fq 'pathType: Prefix' <<<"${default_ingress_render}"

ingress_119_render="$(helm template test "${work_dir}/hami-webui" \
  --namespace hami-webui-test \
  --kube-version 1.19.0 \
  --set 'ingress.enabled=true' \
  --show-only templates/ingress.yaml)"
grep -Fq 'apiVersion: networking.k8s.io/v1' <<<"${ingress_119_render}"
grep -Fq 'pathType: Prefix' <<<"${ingress_119_render}"

if helm template test "${work_dir}/hami-webui" \
  --kube-version 1.18.20 >/dev/null 2>&1; then
  echo "Chart rendered below its Kubernetes 1.19 dependency and Ingress contract" >&2
  exit 1
fi

broader_ingress_render="$(render_template templates/ingress.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].paths[0].path=/' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix')"
grep -Fq 'path: /' <<<"${broader_ingress_render}"

multi_path_ingress_render="$(render_template templates/ingress.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].paths[0].path=/gpu-ui' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix' \
  --set-string 'ingress.hosts[0].paths[1].path=/health_check' \
  --set-string 'ingress.hosts[0].paths[1].pathType=Exact')"
grep -Fq 'path: /health_check' <<<"${multi_path_ingress_render}"
grep -Fq 'pathType: Exact' <<<"${multi_path_ingress_render}"

ingress_notes_render="$(render_template templates/test-install-notes.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].host=ui.example.com' \
  --set-string 'ingress.hosts[0].paths[0].path=/gpu-ui' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix' \
  --set-string 'ingress.hosts[1].host=ui-internal.example.com' \
  --set-string 'ingress.hosts[1].paths[0].path=/' \
  --set-string 'ingress.hosts[1].paths[0].pathType=Prefix' \
  --set-string 'ingress.tls[0].secretName=ui-tls' \
  --set-string 'ingress.tls[0].hosts[0]=ui.example.com')"
grep -Fq 'https://ui.example.com/gpu-ui/' <<<"${ingress_notes_render}"
grep -Fq 'http://ui-internal.example.com/gpu-ui/' <<<"${ingress_notes_render}"

hostless_ingress_notes_render="$(render_template templates/test-install-notes.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].host=' \
  --set-string 'ingress.hosts[0].paths[0].path=/gpu-ui' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix')"
grep -Fq 'export INGRESS_ADDRESS=$(kubectl get ingress --namespace hami-webui-test test-hami-webui' <<<"${hostless_ingress_notes_render}"
grep -Fq 'Open $INGRESS_ADDRESS at path /gpu-ui/' <<<"${hostless_ingress_notes_render}"
if grep -Fq '%!s' <<<"${hostless_ingress_notes_render}"; then
  echo "Hostless Ingress rendered an invalid formatted URL" >&2
  exit 1
fi

wildcard_ingress_notes_render="$(render_template templates/test-install-notes.yaml \
  --set 'ingress.enabled=true' \
  --set-string 'frontend.basePath=/gpu-ui/' \
  --set-string 'ingress.hosts[0].host=*.example.com' \
  --set-string 'ingress.hosts[0].paths[0].path=/gpu-ui' \
  --set-string 'ingress.hosts[0].paths[0].pathType=Prefix' \
  --set-string 'ingress.tls[0].secretName=wildcard-tls' \
  --set-string 'ingress.tls[0].hosts[0]=*.example.com')"
grep -Fq "Choose a concrete hostname matching *.example.com, then visit https://<matching-host>/gpu-ui/" <<<"${wildcard_ingress_notes_render}"

for invalid_ingress_args in \
  '--set ingress.enabled=true --set-string frontend.basePath=/ --set-string ingress.hosts[0].paths[0].path=/gpu-ui --set-string ingress.hosts[0].paths[0].pathType=Prefix' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=/other --set-string ingress.hosts[0].paths[0].pathType=Prefix' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=// --set-string ingress.hosts[0].paths[0].pathType=Prefix' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=/gpu-ui --set-string ingress.hosts[0].paths[0].pathType=Exact' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=/gpu-ui --set-string ingress.hosts[0].paths[0].pathType=ImplementationSpecific' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=/gpu-ui --set-string ingress.hosts[0].paths[0].pathType=Prefix --set-string ingress.hosts[0].paths[1].path=/health_check' \
  '--set ingress.enabled=true --set-string frontend.basePath=/gpu-ui/ --set-string ingress.hosts[0].paths[0].path=/gpu-ui --set-string ingress.hosts[0].paths[0].pathType=Prefix --set-string ingress.hosts[0].paths[1].path=/health_check --set-string ingress.hosts[0].paths[1].pathType=Unknown'; do
  read -r -a invalid_ingress_options <<<"${invalid_ingress_args}"
  if helm template test "${work_dir}/hami-webui" "${invalid_ingress_options[@]}" >/dev/null 2>&1; then
    echo "An Ingress that cannot portably route the configured public base path was accepted" >&2
    exit 1
  fi
done

if helm template test "${work_dir}/hami-webui" \
  --set-json 'frontend.basePath=123' >/dev/null 2>&1; then
  echo "Non-string frontend.basePath was accepted" >&2
  exit 1
fi
if helm template test "${work_dir}/hami-webui" \
  --set-json 'frontend.basePath=false' >/dev/null 2>&1; then
  echo "Boolean frontend.basePath was accepted" >&2
  exit 1
fi
if helm template test "${work_dir}/hami-webui" \
  --set-string 'frontend.basePath=/gpu-ui//nested/' >/dev/null 2>&1; then
  echo "Invalid frontend.basePath segments were accepted" >&2
  exit 1
fi

if helm template test "${work_dir}/hami-webui" \
  --set-string 'env[0].name=HAMI_WEBUI_BASE_PATH' \
  --set-string 'env[0].value=/from-env/' >/dev/null 2>&1; then
  echo "Generic env bypassed the dedicated frontend.basePath contract" >&2
  exit 1
fi

frame_none_render="$(helm template test "${work_dir}/hami-webui" \
  --set-json 'frontend.frameAncestors=[]')"
grep -A1 -F 'name: HAMI_WEBUI_FRAME_ANCESTORS_JSON' <<<"${frame_none_render}" | grep -Fq 'value: "[]"'

frame_allow_render="$(helm template test "${work_dir}/hami-webui" \
  --set-json "frontend.frameAncestors=[\"'self'\",\"https://portal.example.com\"]")"
grep -A1 -F 'name: HAMI_WEBUI_FRAME_ANCESTORS_JSON' <<<"${frame_allow_render}" | \
  grep -Fq 'value: "[\"'"'"'self'"'"'\",\"https://portal.example.com\"]"'

if helm template test "${work_dir}/hami-webui" \
  --set-string 'env[0].name=HAMI_WEBUI_FRAME_ANCESTORS_JSON' \
  --set-string 'env[0].value=["https://portal.internal"]' >/dev/null 2>&1; then
  echo "Generic env bypassed the dedicated frontend.frameAncestors contract" >&2
  exit 1
fi

if helm template test "${work_dir}/hami-webui" \
  --set-string 'frontend.frameAncestors=https://portal.example.com' >/dev/null 2>&1; then
  echo "Non-list frontend.frameAncestors was accepted" >&2
  exit 1
fi
if helm template test "${work_dir}/hami-webui" \
  --set-json 'frontend.frameAncestors=[123]' >/dev/null 2>&1; then
  echo "Non-string frontend.frameAncestors entry was accepted" >&2
  exit 1
fi

empty_env_render="$(helm template test "${work_dir}/hami-webui" \
  --set-json 'env=[]')"
if [[ "$(grep -c 'name: HAMI_WEBUI_BASE_PATH' <<<"${empty_env_render}")" -ne 1 ]]; then
  echo "Empty env value did not render exactly one base-path variable" >&2
  exit 1
fi

probes_disabled_render="$(helm template test "${work_dir}/hami-webui" \
  --set 'probes.startup.enabled=false' \
  --set 'probes.readiness.enabled=false' \
  --set 'probes.liveness.enabled=false' \
  --show-only templates/deployment.yaml)"
if grep -Eq '^[[:space:]]+(startupProbe|readinessProbe|livenessProbe):' <<<"${probes_disabled_render}"; then
  echo "Disabled unified-container probes were still rendered" >&2
  exit 1
fi

clean_migration_render="$(helm template test "${work_dir}/hami-webui" \
  --set-string 'image.repository=registry.example.com/platform/hami-webui' \
  --set-string 'image.tag=migrated' \
  --set-string 'image.digest=' \
  --set-string 'resources.requests.cpu=125m' \
  --set-string 'resources.requests.memory=384Mi' \
  --set-string 'resources.limits.cpu=500m' \
  --set-string 'resources.limits.memory=1Gi' \
  --set-string 'env[0].name=TZ' \
  --set-string 'env[0].value=UTC' \
  --set-string 'frontend.basePath=/embedded/' \
  --set-json "frontend.frameAncestors=[\"'self'\",\"https://portal.example.com\"]" \
  --set-string 'backend.http.timeout=45s' \
  --show-only templates/deployment.yaml \
  --show-only templates/configmap.yaml)"
for expected in \
  'image: "registry.example.com/platform/hami-webui:migrated"' \
  'cpu: 125m' \
  'memory: 384Mi' \
  'cpu: 500m' \
  'memory: 1Gi' \
  'name: TZ' \
  'value: UTC' \
  'value: "/embedded/"' \
  'value: "[\"'"'"'self'"'"'\",\"https://portal.example.com\"]"' \
  'timeout: 45s'; do
  if ! grep -Fq "${expected}" <<<"${clean_migration_render}"; then
    echo "Clean Chart 2 migration values did not render ${expected}" >&2
    exit 1
  fi
done

legacy_values_fixture="${repo_root}/scripts/chart/tests/values-chart1-transition.yaml"
if [[ ! -f "${legacy_values_fixture}" ]]; then
  echo "Missing the complete Chart 1 migration regression fixture" >&2
  exit 1
fi

helm_supports_skip_schema=false
if helm template --help | grep -Fq -- '--skip-schema-validation'; then
  helm_supports_skip_schema=true
fi

assert_legacy_values_rejected() {
  local expected_field="$1"
  shift
  local mode output

  for mode in schema template; do
    if [[ "${mode}" == template ]]; then
      if [[ "${helm_supports_skip_schema}" != true ]]; then
        continue
      fi
      if output="$(helm template test "${work_dir}/hami-webui" \
        --namespace hami-webui-test \
        --skip-schema-validation \
        "$@" 2>&1)"; then
        echo "Chart 1.x value ${expected_field} was accepted in ${mode} validation mode" >&2
        exit 1
      fi
    elif output="$(helm template test "${work_dir}/hami-webui" \
      --namespace hami-webui-test \
      "$@" 2>&1)"; then
      echo "Chart 1.x value ${expected_field} was accepted in ${mode} validation mode" >&2
      exit 1
    fi
    if ! grep -Fq "${expected_field}" <<<"${output}"; then
      echo "Chart 1.x value rejection did not identify ${expected_field} in ${mode} validation mode" >&2
      echo "${output}" >&2
      exit 1
    fi
  done
}

assert_legacy_values_rejected 'image.frontend' \
  --set-json 'image.frontend={"repository":"legacy/frontend"}'
assert_legacy_values_rejected 'image.backend' \
  --set-json 'image.backend={"repository":"legacy/backend"}'
assert_legacy_values_rejected 'resources.frontend' \
  --set-json 'resources.frontend={"requests":{"cpu":"100m"}}'
assert_legacy_values_rejected 'resources.backend' \
  --set-json 'resources.backend={"requests":{"cpu":"100m"}}'
assert_legacy_values_rejected 'env.frontend' \
  --set-json 'env={"frontend":[{"name":"TZ","value":"UTC"}]}'
assert_legacy_values_rejected 'env.backend' \
  --set-json 'env={"backend":[{"name":"TZ","value":"UTC"}]}'
assert_legacy_values_rejected 'frontend.proxyTimeout' \
  --set-string 'frontend.proxyTimeout=65s'
assert_legacy_values_rejected 'frontend.livenessProbe' \
  --set-json 'frontend.livenessProbe={"enabled":true}'
assert_legacy_values_rejected 'frontend.readinessProbe' \
  --set-json 'frontend.readinessProbe={"enabled":true}'
assert_legacy_values_rejected 'backend.grpc' \
  --set-json 'backend.grpc={"timeout":"60s"}'
assert_legacy_values_rejected 'backend.readinessProbe' \
  --set-json 'backend.readinessProbe={"enabled":true}'
assert_legacy_values_rejected 'service.legacyBackendPort' \
  --set 'service.legacyBackendPort=true'

for validation_mode in schema template; do
  if [[ "${validation_mode}" == template ]]; then
    if [[ "${helm_supports_skip_schema}" != true ]]; then
      continue
    fi
    if full_legacy_output="$(helm template test "${work_dir}/hami-webui" \
      --namespace hami-webui-test \
      --skip-schema-validation \
      --values "${legacy_values_fixture}" 2>&1)"; then
      echo "Complete Chart 1.3 values were accepted in ${validation_mode} validation mode" >&2
      exit 1
    fi
  elif full_legacy_output="$(helm template test "${work_dir}/hami-webui" \
    --namespace hami-webui-test \
    --values "${legacy_values_fixture}" 2>&1)"; then
    echo "Complete Chart 1.3 values were accepted in ${validation_mode} validation mode" >&2
    exit 1
  fi

  for legacy_field in \
    'image.frontend' \
    'image.backend' \
    'resources.frontend' \
    'resources.backend' \
    'env.frontend' \
    'env.backend' \
    'frontend.proxyTimeout' \
    'frontend.livenessProbe' \
    'frontend.readinessProbe' \
    'backend.grpc' \
    'backend.readinessProbe' \
    'service.legacyBackendPort'; do
    if ! grep -Fq "${legacy_field}" <<<"${full_legacy_output}"; then
      echo "Complete Chart 1.3 rejection omitted ${legacy_field} in ${validation_mode} validation mode" >&2
      echo "${full_legacy_output}" >&2
      exit 1
    fi
  done
done

echo "Chart lint, single-container, migration, and image reference checks passed."
