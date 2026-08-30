#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 IMAGE PLATFORM DOCKER_NETWORK KUBECONFIG" >&2
  exit 2
fi

image=$1
platform=$2
network=$3
kubeconfig=$4
expected_arch=${platform##*/}

for command in curl docker node; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "required command is unavailable: ${command}" >&2
    exit 2
  fi
done

if [[ ! -r "${kubeconfig}" ]]; then
  echo "kubeconfig is not readable: ${kubeconfig}" >&2
  exit 2
fi

workdir=$(mktemp -d)
container="hami-webui-unified-smoke-${expected_arch}-${RANDOM}"

cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if docker container inspect "${container}" >/dev/null 2>&1; then
    if [[ ${status} -ne 0 ]]; then
      docker logs "${container}" >&2
    fi
    docker rm --force "${container}" >/dev/null 2>&1
  fi
  rm -rf "${workdir}"
  exit "${status}"
}
trap cleanup EXIT

cat >"${workdir}/config.yaml" <<'EOF'
server:
  http:
    addr: 0.0.0.0:8000
    timeout: 60s
prometheus:
  # No devices match the smoke-test selectors, so the collector must remain
  # healthy without depending on an upstream Prometheus process.
  address: http://127.0.0.1:9
  timeout: 1s
exporter:
  interval: 3600s
  timeout: 1s
node_selectors:
  NVIDIA: hami-webui-smoke-nvidia=true
  Ascend: hami-webui-smoke-ascend=true
  DCU: hami-webui-smoke-dcu=true
  MLU: hami-webui-smoke-mlu=true
  Metax: hami-webui-smoke-metax=true
EOF
cp "${kubeconfig}" "${workdir}/kubeconfig"
chmod 0444 "${workdir}/config.yaml" "${workdir}/kubeconfig"

inspect() {
  docker image inspect --format "$1" "${image}"
}

[[ "$(inspect '{{.Architecture}}')" == "${expected_arch}" ]]
[[ "$(inspect '{{.Config.User}}')" == "65532:65532" ]]
[[ "$(inspect '{{json .Config.Entrypoint}}')" == '["/apps/hami-webui"]' ]]
[[ "$(inspect '{{json .Config.Cmd}}')" == '["--conf","/apps/config/config.yaml"]' ]]
[[ "$(inspect '{{json .Config.Healthcheck.Test}}')" == '["CMD","/apps/hami-webui","--healthcheck"]' ]]

docker run --detach \
  --platform "${platform}" \
  --name "${container}" \
  --network "${network}" \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --env KUBECONFIG=/apps/smoke/kubeconfig \
  --env HAMI_WEBUI_BASE_PATH=/gpu-ui/ \
  --env "HAMI_WEBUI_FRAME_ANCESTORS_JSON=[\"'self'\",\"https://portal.example.com\"]" \
  --mount "type=bind,src=${workdir}/config.yaml,dst=/apps/config/config.yaml,readonly" \
  --mount "type=bind,src=${workdir}/kubeconfig,dst=/apps/smoke/kubeconfig,readonly" \
  --publish 127.0.0.1::3000 \
  --publish 127.0.0.1::8000 \
  "${image}" >/dev/null

published_port() {
  docker port "${container}" "$1/tcp" | sed -E 's/.*:([0-9]+)$/\1/' | head -n 1
}

public_port=$(published_port 3000)
backend_port=$(published_port 8000)
curl_local=(curl --noproxy '*' --connect-timeout 3 --max-time 15)
curl_probe=(curl --noproxy '*' --connect-timeout 1 --max-time 2)

for _ in {1..45}; do
  if "${curl_probe[@]}" --fail --silent --show-error "http://127.0.0.1:${public_port}/health_check" >/dev/null; then
    break
  fi
  if [[ "$(docker inspect --format '{{.State.Running}}' "${container}")" != "true" ]]; then
    echo "unified container exited before becoming ready" >&2
    exit 1
  fi
  sleep 2
done

"${curl_local[@]}" --fail --silent --show-error "http://127.0.0.1:${public_port}/health_check" >/dev/null
"${curl_local[@]}" --fail --silent --show-error \
  --output "${workdir}/index.html" \
  "http://127.0.0.1:${public_port}/gpu-ui/"
grep -Fq '<base data-hami-webui-base href="/gpu-ui/">' "${workdir}/index.html"
"${curl_local[@]}" --silent --show-error --head \
  --output "${workdir}/headers" \
  "http://127.0.0.1:${public_port}/gpu-ui/"
tr -d '\r' <"${workdir}/headers" >"${workdir}/headers.normalized"
grep -Fq "Content-Security-Policy: frame-ancestors 'self' https://portal.example.com" \
  "${workdir}/headers.normalized"

api_response="${workdir}/nodes.json"
api_status=$("${curl_local[@]}" --silent --show-error \
  --header 'Content-Type: application/json' \
  --data '{"filters":{}}' \
  --output "${api_response}" \
  --write-out '%{http_code}' \
  "http://127.0.0.1:${public_port}/gpu-ui/api/vgpu/v1/nodes")
[[ "${api_status}" == "200" ]]
node -e 'JSON.parse(require("node:fs").readFileSync(process.argv[1], "utf8"))' "${api_response}"

for private_path in /gpu-ui/metrics /gpu-ui/readyz; do
  [[ "$("${curl_local[@]}" --silent --output /dev/null --write-out '%{http_code}' \
    "http://127.0.0.1:${public_port}${private_path}")" == "404" ]]
done

"${curl_local[@]}" --fail --silent --show-error \
  --output "${workdir}/readyz" \
  "http://127.0.0.1:${backend_port}/readyz"
grep -Fq 'ok' "${workdir}/readyz"
"${curl_local[@]}" --fail --silent --show-error \
  --output "${workdir}/metrics" \
  "http://127.0.0.1:${backend_port}/metrics"
grep -Fq '# HELP' "${workdir}/metrics"

docker exec "${container}" /apps/hami-webui --healthcheck
mkdir "${workdir}/public"
docker cp "${container}:/etc/ssl/certs/ca-certificates.crt" "${workdir}/ca-certificates.crt" >/dev/null
docker cp "${container}:/usr/share/zoneinfo/Etc/UTC" "${workdir}/UTC" >/dev/null
docker cp "${container}:/apps/public/." "${workdir}/public" >/dev/null
[[ -s "${workdir}/ca-certificates.crt" ]]
[[ -s "${workdir}/UTC" ]]
[[ -s "${workdir}/public/index.html" ]]
find "${workdir}/public/static" -type f -name '*.gz' -print -quit | grep -q .

docker stop --time 15 "${container}" >/dev/null
[[ "$(docker inspect --format '{{.State.ExitCode}}' "${container}")" == "0" ]]
docker rm "${container}" >/dev/null
container=""

echo "Unified runtime smoke passed for ${platform}."
