# HAMi-WebUI

![Version: 1.3.0](https://img.shields.io/badge/Version-1.3.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.3.0](https://img.shields.io/badge/AppVersion-1.3.0-informational?style=flat-square)

## Get Repo Info

```console
helm repo add hami-webui https://project-hami.github.io/HAMi-WebUI
helm repo update
```

_See [helm repo](https://helm.sh/docs/helm/helm_repo/) for command documentation._

## Installing the Chart

Before deploying, ensure that you configure the `values.yaml` file to match your cluster’s requirements. For detailed instructions, refer to the [Configuration Guide for HAMi-WebUI Helm Chart](#configuration-guide-for-hamiwebui-helm-chart)
> _**Important**_: You must adjust the values.yaml before proceeding with the deployment.

Download the `values.yaml` file from the Helm Charts repository:

https://github.com/Project-HAMi/HAMi-WebUI/blob/main/charts/hami-webui/values.yaml


```console
helm install my-hami-webui hami-webui/hami-webui --create-namespace --namespace hami -f values.yaml
```

## Uninstalling the Chart

To uninstall/delete the my-release deployment:

```console
helm delete my-hami-webui
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
| dcgm-exporter.enabled | bool | `true`                                                                             |  |
| dcgm-exporter.nodeSelector.gpu | string | `"on"`                                                                             |  |
| dcgm-exporter.serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| dcgm-exporter.serviceMonitor.enabled | bool | `true`                                                                             |  |
| dcgm-exporter.serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| dcgm-exporter.serviceMonitor.interval | string | `"15s"`                                                                            |  |
| externalPrometheus.address | string | `"http://prometheus-kube-prometheus-prometheus.prometheus.svc.cluster.local:9090"` | Prometheus or VictoriaMetrics HTTP API address. |
| externalPrometheus.enabled | bool | `false`                                                                            | Use an existing metrics backend instead of the bundled address. |
| externalPrometheus.timeout | string | `"1m"` | Timeout sent with each upstream PromQL request. |
| externalPrometheus.tls.insecureSkipVerify | bool | `false` | Disable HTTPS certificate verification. Use only as a temporary break-glass setting. |
| externalPrometheus.tls.serverName | string | `""` | Optional certificate name override for the HTTPS endpoint. |
| externalPrometheus.tls.existingSecret | string | `""` | Existing Secret containing private-CA or mTLS files. The Chart never creates this Secret. |
| externalPrometheus.tls.caKey | string | `""` | Secret data key containing the CA certificate. |
| externalPrometheus.tls.certKey | string | `""` | Secret data key containing the client certificate; configure with `keyKey`. |
| externalPrometheus.tls.keyKey | string | `""` | Secret data key containing the client private key; configure with `certKey`. |
| frontend.basePath | string | `"/"` | Public URL prefix served by the official Go Web entry; use the same non-stripped Ingress path. |
| frontend.frameAncestors | list or null | `null` | CSP framing allowlist. `null` preserves existing behavior, `[]` blocks framing, and a list allows explicit parents. |
| frontend.livenessProbe.enabled | bool | `true` | Enable the Web-entry liveness probe. |
| frontend.livenessProbe.failureThreshold | int | `6` |  |
| frontend.livenessProbe.initialDelaySeconds | int | `5` |  |
| frontend.livenessProbe.periodSeconds | int | `10` |  |
| frontend.livenessProbe.timeoutSeconds | int | `3` |  |
| frontend.proxyTimeout | string | `"65s"` | End-to-end backend proxy timeout; keep this longer than `backend.http.timeout`. |
| frontend.readinessProbe.enabled | bool | `true` | Enable the Web-entry readiness probe. |
| frontend.readinessProbe.failureThreshold | int | `3` |  |
| frontend.readinessProbe.initialDelaySeconds | int | `1` |  |
| frontend.readinessProbe.periodSeconds | int | `5` |  |
| frontend.readinessProbe.timeoutSeconds | int | `3` |  |
| fullnameOverride | string | `""`                                                                               |  |
| hamiServiceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| hamiServiceMonitor.enabled | bool | `true`                                                                             |  |
| hamiServiceMonitor.honorLabels | bool | `true`                                                                            | Keep HAMi's workload namespace/pod/container labels when they collide with Prometheus scrape-target labels. |
| hamiServiceMonitor.interval | string | `"15s"`                                                                            |  |
| hamiServiceMonitor.relabelings | list | `[]`                                                                               |  |
| hamiServiceMonitor.svcNamespace | string | `"kube-system"`                                                                    | Namespace where the HAMi monitor Service is installed. |
| image.backend.digest | string | `"sha256:7057047c7c2f7838cd190b3dc7263d503bbcf5d8e52642bc703227d137bc029d"`                | Immutable manifest digest; takes precedence over `image.backend.tag` when set. |
| image.backend.pullPolicy | string | `"IfNotPresent"`                                                                  |  |
| image.backend.repository | string | `"projecthami/hami-webui-be-oss"`                                                  |  |
| image.backend.tag | string | `"v1.3.0"`                                                                         | Used only when `image.backend.digest` is empty. |
| image.frontend.digest | string | `"sha256:b40bbec2b963932545a8b7ac15efef3ec087c76dce4da0ea4c3659fa2abd695e"`               | Immutable manifest digest; takes precedence over `image.frontend.tag` when set. |
| image.frontend.pullPolicy | string | `"IfNotPresent"`                                                                   |  |
| image.frontend.repository | string | `"projecthami/hami-webui-fe-oss"`                                                  |  |
| image.frontend.tag | string | `"v1.3.0"`                                                                         | Used only when `image.frontend.digest` is empty. |
| imagePullSecrets | list | `[]`                                                                               | Image pull secrets used by both containers in the WebUI Pod. |
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
| kube-prometheus-stack.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.grafana.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.kube-state-metrics.prometheus.monitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          | Allow the bundled Prometheus to scrape kube-state-metrics for cluster CPU and memory totals. |
| kube-prometheus-stack.kubernetesServiceMonitors.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.nodeExporter.enabled | bool | `false`                                                                            |  |
| kube-prometheus-stack.prometheus.prometheusSpec.serviceMonitorSelector.matchLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| kube-prometheus-stack.prometheusOperator.enabled | bool | `false`                                                                            |  |
| nameOverride | string | `""`                                                                               |  |
| namespaceOverride | string | `""`                                                                               |  |
| nodeSelector | object | `{}`                                                                               |  |
| podAnnotations | object | `{}`                                                                               |  |
| podSecurityContext | object | `{}`                                                                               |  |
| replicaCount | int | `1`                                                                                |  |
| resources.backend.limits.cpu | string | `"50m"`                                                                            |  |
| resources.backend.limits.memory | string | `"250Mi"`                                                                          |  |
| resources.backend.requests.cpu | string | `"50m"`                                                                            |  |
| resources.backend.requests.memory | string | `"250Mi"`                                                                          |  |
| resources.frontend.limits.cpu | string | `"200m"`                                                                           |  |
| resources.frontend.limits.memory | string | `"500Mi"`                                                                          |  |
| resources.frontend.requests.cpu | string | `"200m"`                                                                           |  |
| resources.frontend.requests.memory | string | `"500Mi"`                                                                          |  |
| securityContext | object | `{}`                                                                               |  |
| service.legacyBackendPort | bool | `true`                                                                        | Deprecated Chart 1.x compatibility port for direct access to the raw backend on the primary Service. Set to `false` to expose only the supported Web entry; removed in Chart 2.0.0. |
| service.port | int | `3000`                                                                             |  |
| service.type | string | `"ClusterIP"`                                                                      |  |
| serviceAccount.annotations | object | `{}`                                                                               |  |
| serviceAccount.create | bool | `true`                                                                             |  |
| serviceAccount.name | string | `""`                                                                               |  |
| serviceMonitor.additionalLabels.jobRelease | string | `"hami-webui-prometheus"`                                                          |  |
| serviceMonitor.enabled | bool | `true`                                                                             |  |
| serviceMonitor.honorLabels | bool | `false`                                                                            |  |
| serviceMonitor.interval | string | `"15s"`                                                                            |  |
| serviceMonitor.relabelings | list | `[]`                                                                               |  |
| tolerations | list | `[]`                                                                               |  |

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

#### Scenario 1: If an existing Prometheus is available in your cluster

If your cluster already has a working Prometheus instance, you can enable the external Prometheus configuration and provide the correct address:

```yaml
externalPrometheus:
  enabled: true
  address: "<your-prometheus-address>"
```

Here, replace <your-prometheus-address> with the actual domain or internal Ingress address for your Prometheus instance.

HTTPS certificates are verified by default. For an endpoint signed by a private
CA, create a Secret and reference its data key instead of placing PEM material
in values:

```yaml
externalPrometheus:
  enabled: true
  address: "https://prometheus.example.com"
  tls:
    existingSecret: "prometheus-ca"
    caKey: "ca.crt"
```

See the [installation guide](../../docs/installation/helm/index.md#https-external-prometheus)
for Secret creation, mTLS, and the upgrade boundary.

#### Scenario 2: If no Prometheus or Operator is installed in the cluster

If there is no existing Prometheus or Prometheus Operator in your cluster, you can enable the kube-prometheus-stack to deploy Prometheus:

```yaml
kube-prometheus-stack:
  enabled: true
  crds:
    enabled: true
  ...
  prometheusOperator:
    enabled: true
  ...
```

#### Scenario 3: If Prometheus and Operator already exist, but a separate Prometheus instance is needed
If your cluster has Prometheus and Prometheus Operator, but you want to use a separate instance without affecting the existing setup, modify the configuration as follows:

```yaml
kube-prometheus-stack:
  enabled: true
  ...
```
This allows you to reuse the existing Operator and CRDs while deploying a new Prometheus instance.

### 3. About `jobRelease` Labels

If deploying a completely new Prometheus, you can leave the default `jobRelease: hami-webui-prometheus` unchanged.

***However, if you are integrating with an existing Prometheus instance and modifying the `prometheusSpec.serviceMonitorSelector.matchLabels`, ensure that **all** corresponding `...ServiceMonitor.additionalLabels` are updated to reflect the correct label.***

For example, if you modify:

```yaml
prometheus:
  prometheusSpec:
    serviceMonitorSelector:
      matchLabels:
        <jobRelease-label-key>: <jobRelease-label-value>
```

You must also modify all ...ServiceMonitor.additionalLabels in your values.yaml file to match:

```yaml
...ServiceMonitor:
  additionalLabels:
    <jobRelease-label-key>: <jobRelease-label-value>
```

This includes `kube-prometheus-stack.kube-state-metrics.prometheus.monitor.additionalLabels`.

This ensures that Prometheus will correctly discover all the ServiceMonitor configurations based on the updated labels.
