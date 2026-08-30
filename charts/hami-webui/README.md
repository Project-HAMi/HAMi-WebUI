# HAMi-WebUI

![Version: 2.0.0-rc.1](https://img.shields.io/badge/Version-2.0.0--rc.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: main](https://img.shields.io/badge/AppVersion-main-informational?style=flat-square)

## Get Repo Info

```console
helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
helm repo update
```

_See [helm repo](https://helm.sh/docs/helm/helm_repo/) for command documentation._

## Installing the Chart

Choose a released Chart version first. This README follows the Chart source in
its branch, while the published Helm repository may contain an older release.
Never combine `values.yaml` from `main` with a different Chart version.

```console
helm search repo hami-webui/hami-webui --versions
# Replace X.Y.Z with the version you want to install.
CHART_VERSION="X.Y.Z"
helm show values hami-webui/hami-webui \
  --version "${CHART_VERSION}" > values.yaml
# Review and edit values.yaml for your cluster before installing it.
PROMETHEUS_ADDRESS="http://prometheus.monitoring.svc.cluster.local:9090"
helm install my-hami-webui hami-webui/hami-webui \
  --version "${CHART_VERSION}" \
  --create-namespace \
  --namespace hami \
  --values values.yaml \
  --set externalPrometheus.enabled=true \
  --set-string externalPrometheus.address="${PROMETHEUS_ADDRESS}"
```

Using `helm show values` and `helm install` with the same explicit version keeps
the configuration schema and the installed templates together. For the selected
release, treat the comments in the retrieved values file as authoritative. The
[configuration guide below](#configuration-guide-for-hamiwebui-helm-chart)
describes the Chart version shown at the top of this README.

The command above assumes an existing Prometheus and Prometheus Operator. Its
Prometheus resource must select the ServiceMonitors created by this Chart. See
[Prometheus modes](#2-about-prometheus) before installing into a new cluster or
one that uses raw scrape configuration.

Chart 2 deploys one `projecthami/hami-webui` image and one container. When
upgrading from Chart 1.3, create a fresh Chart 2 values file and use
`--reset-values`; do not reuse the nested frontend/backend values. See the
[upgrade and rollback procedure](../../docs/installation/helm/index.md#upgrade-from-chart-13-to-20).

## Uninstalling the Chart

To uninstall the release:

```console
helm uninstall my-hami-webui --namespace hami
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Requirements

| Repository | Name | Version |
|------------|------|---------|
| https://nvidia.github.io/dcgm-exporter/helm-charts | dcgm-exporter | 3.5.0 |
| https://prometheus-community.github.io/helm-charts | kube-prometheus-stack | 62.6.0 |

## Values

| Key | Type | Default                                                                            | Description |
|-----|------|------------------------------------------------------------------------------------|-------------|
| affinity | object | `{}`                                                                               |  |
| backend.http.timeout | string | `"60s"` | Timeout applied to each incoming API request context. |
| dcgm-exporter.enabled | bool | `true`                                                                             |  |
| dcgm-exporter.nodeSelector.gpu | string | `"on"`                                                                             |  |
| dcgm-exporter.serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| dcgm-exporter.serviceMonitor.enabled | bool | `true`                                                                             | Create a DCGM ServiceMonitor; disable in raw/manual scrape mode. |
| dcgm-exporter.serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| dcgm-exporter.serviceMonitor.interval | string | `"15s"`                                                                            |  |
| env[0].name | string | `"TZ"` | Default environment variable name for the single application container. Replace the list to add other variables. |
| env[0].value | string | `"Asia/Shanghai"` | Default timezone value. |
| externalPrometheus.address | string | `""` | Required Prometheus or VictoriaMetrics HTTP API address when external mode is enabled. |
| externalPrometheus.enabled | bool | `false`                                                                            | Use an existing metrics backend. Exactly one of this value and `kube-prometheus-stack.enabled` must be true. |
| externalPrometheus.timeout | string | `"1m"` | Timeout sent with each upstream PromQL request. |
| externalPrometheus.tls.insecureSkipVerify | bool | `false` | Disable HTTPS certificate verification. Use only as a temporary break-glass setting. |
| externalPrometheus.tls.serverName | string | `""` | Optional certificate name override for the HTTPS endpoint. |
| externalPrometheus.tls.existingSecret | string | `""` | Existing Secret containing private-CA or mTLS files. The Chart never creates this Secret. |
| externalPrometheus.tls.caKey | string | `""` | Secret data key containing the CA certificate. |
| externalPrometheus.tls.certKey | string | `""` | Secret data key containing the client certificate; configure with `keyKey`. |
| externalPrometheus.tls.keyKey | string | `""` | Secret data key containing the client private key; configure with `certKey`. |
| frontend.basePath | string | `"/"` | Public URL prefix served by the official Go Web entry; use the same non-stripped Ingress path. |
| frontend.frameAncestors | list or null | `null` | CSP framing allowlist. `null` preserves existing behavior, `[]` blocks framing, and a list allows explicit parents. |
| fullnameOverride | string | `""`                                                                               |  |
| hamiServiceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| hamiServiceMonitor.enabled | bool | `true`                                                                             | Preferred HAMi device-plugin monitor. Disable only when another selected monitor preserves workload labels with `honorLabels: true`, or in raw/manual mode. |
| hamiServiceMonitor.honorLabels | bool | `true`                                                                            | Keep HAMi's workload namespace/pod/container labels when they collide with Prometheus scrape-target labels. |
| hamiServiceMonitor.interval | string | `"15s"`                                                                            |  |
| hamiServiceMonitor.relabelings | list | `[]`                                                                               |  |
| hamiServiceMonitor.svcNamespace | string | `"kube-system"`                                                                    | Namespace where the HAMi monitor Service is installed. |
| image.digest | string | `""` | Immutable manifest digest; takes precedence over `image.tag` when set. |
| image.pullPolicy | string | `"IfNotPresent"` | Pull policy for the application image. |
| image.repository | string | `"projecthami/hami-webui"` | Unified HAMi-WebUI image repository. |
| image.tag | string | `"main"` | Used only when `image.digest` is empty. Release packaging replaces development defaults with the released version. |
| imagePullSecrets | list | `[]` | Image pull secrets used by the WebUI Pod. |
| ingress.annotations | object | `{}`                                                                               |  |
| ingress.className | string | `""`                                                                               |  |
| ingress.enabled | bool | `false`                                                                            |  |
| ingress.hosts[0].host | string | `"chart-example.local"`                                                            |  |
| ingress.hosts[0].paths[0].path | string | `"/"`                                                                              |  |
| ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"`                                                         |  |
| ingress.tls | list | `[]`                                                                               |  |
| kube-prometheus-stack.alertmanager.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.crds.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.defaultRules.create | bool | `false`                                                                            |  |
| kube-prometheus-stack.enabled | bool | `false`                                                                            | Create a dedicated Prometheus through the dependency Chart. Mutually exclusive with `externalPrometheus.enabled`. |
| kube-prometheus-stack.grafana.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.kube-state-metrics.prometheus.monitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          | Allow the bundled Prometheus to scrape kube-state-metrics for cluster CPU and memory totals. |
| kube-prometheus-stack.kubernetesServiceMonitors.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.nodeExporter.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.prometheus.prometheusSpec.serviceMonitorSelector.matchLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| kube-prometheus-stack.prometheusOperator.enabled | bool | `false`                                                                            |  |
| metricsExporter.interval | string | `"30s"` | Interval between background metric refreshes. |
| metricsExporter.timeout | string | `"60s"` | Hard timeout for one background metric refresh. |
| nameOverride | string | `""`                                                                               |  |
| namespaceOverride | string | `""`                                                                               |  |
| nodeSelector | object | `{}`                                                                               |  |
| podAnnotations | object | `{}`                                                                               |  |
| podSecurityContext | object | `{}`                                                                               |  |
| probes.liveness.enabled | bool | `true` | Probe the public `/health_check` endpoint after startup. |
| probes.liveness.failureThreshold | int | `6` |  |
| probes.liveness.initialDelaySeconds | int | `0` |  |
| probes.liveness.periodSeconds | int | `10` |  |
| probes.liveness.timeoutSeconds | int | `3` |  |
| probes.readiness.enabled | bool | `true` | Probe the public `/health_check` endpoint after startup. |
| probes.readiness.failureThreshold | int | `3` |  |
| probes.readiness.initialDelaySeconds | int | `0` |  |
| probes.readiness.periodSeconds | int | `5` |  |
| probes.readiness.timeoutSeconds | int | `3` |  |
| probes.startup.enabled | bool | `true` | Wait for internal `/readyz` and informer synchronization before other probes begin. |
| probes.startup.failureThreshold | int | `60` | Combined with the five-second period, allows up to five minutes for startup. |
| probes.startup.initialDelaySeconds | int | `0` |  |
| probes.startup.periodSeconds | int | `5` |  |
| probes.startup.timeoutSeconds | int | `3` |  |
| replicaCount | int | `1`                                                                                |  |
| resources.limits.cpu | string | `"250m"` | CPU limit for the single application container. |
| resources.limits.memory | string | `"750Mi"` | Memory limit for the single application container. |
| resources.requests.cpu | string | `"250m"` | CPU request for the single application container. |
| resources.requests.memory | string | `"750Mi"` | Memory request for the single application container. |
| securityContext | object | `{}`                                                                               |  |
| service.port | int | `3000` | Public SPA and browser API Service port. The internal metrics Service keeps port 8000 separate. |
| service.type | string | `"ClusterIP"`                                                                      |  |
| serviceAccount.annotations | object | `{}`                                                                               |  |
| serviceAccount.create | bool | `true`                                                                             |  |
| serviceAccount.name | string | `""`                                                                               |  |
| serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| serviceMonitor.enabled | bool | `true`                                                                             | Create a ServiceMonitor for HAMi-WebUI metrics; requires Prometheus Operator CRDs. |
| serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| serviceMonitor.interval | string | `"15s"`                                                                            |  |
| serviceMonitor.relabelings | list | `[]`                                                                               |  |
| tolerations | list | `[]`                                                                               |  |
| vendorNodeSelectors.Ascend | string | `"ascend=on"` | Node-label selector used for Ascend device discovery. |
| vendorNodeSelectors.DCU | string | `"dcu=on"` | Node-label selector used for DCU device discovery. |
| vendorNodeSelectors.MLU | string | `"mlu=on"` | Node-label selector used for MLU device discovery. |
| vendorNodeSelectors.Metax | string | `"metax-tech.com/gpu.installed=true"` | Node-label selector used for Metax device discovery. |
| vendorNodeSelectors.NVIDIA | string | `"gpu=on"` | Node-label selector used for NVIDIA device discovery. |

## Configuration Guide for HAMi-WebUI Helm Chart

For a non-root URL path and whole-application iframe embedding, see the
[embedding guide](https://github.com/Project-HAMi/HAMi-WebUI/blob/main/docs/installation/embedding.md).
The official Go Web entry requires the Ingress to preserve the configured
`frontend.basePath`.

### 1. About `dcgm-exporter`

If `dcgm-exporter` is already installed in your cluster, you should disable it by modifying the following setting:

```yaml
dcgm-exporter:
  enabled: false
```
This ensures that the existing `dcgm-exporter` instance is used, preventing conflicts.


### 2. About `Prometheus`

HAMi-WebUI does not guess a Prometheus release name or namespace. Select exactly
one mode; rendering fails when neither or both are enabled.

For the recommended existing-Prometheus path, provide its real HTTP API address:

```yaml
externalPrometheus:
  enabled: true
  address: "<your-prometheus-address>"
```

For a self-contained evaluation cluster, enable the dependency together with
its Operator and CRDs:

```yaml
kube-prometheus-stack:
  enabled: true
  crds:
    enabled: true
  prometheusOperator:
    enabled: true
```

The external path requires a running Prometheus Operator and matching
ServiceMonitor label/namespace selectors. The self-contained path creates
cluster-scoped resources. Raw scraping, reuse of an existing Operator, HTTPS
trust, duplicate-target prevention, and verification queries are documented in
the [installation guide](../../docs/installation/helm/index.md#select-a-prometheus-ownership-mode).
